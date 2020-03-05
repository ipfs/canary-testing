package runtime

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Invoke runs the passed test-case and reports the result.
func Invoke(tc func(*RunEnv) error) {
	runenv := CurrentRunEnv()
	start := time.Now()
	durationGuage := NewGauge(runenv, "plan_duration", "Run time (Seconds)")

	// The prometheus pushgateway has a customized scrape interval, which is used to hint to the
	// prometheus operator at which interval the it should be scraped. This is currently set to 5s.
	// To provide an updated metric in every scrape, jobs will push to the pushgateway at the same
	// interval. When this "push_interval" is changed, you may want to change the scrape interval
	// on the pushgateway
	push_interval := 5
	go func() {
		for range time.Tick(time.Duration(push_interval) * time.Second) {
			err := runenv.MetricsPusher.Add()
			if err != nil {
				runenv.RecordMessage("error during periodic metric push: %w", err)
			}
		}
	}()

	// Push metrics one last time, including the duration for the whole run.
	defer func() {
		durationGuage.Set(time.Since(start).Seconds())
		err := runenv.MetricsPusher.Add()
		if err != nil {
			runenv.RecordMessage("error during end metric push: %w", err)
		}
	}()

	defer runenv.Close()

	runenv.RecordStart()

	errfile, err := runenv.CreateRawAsset("run.err")
	if err != nil {
		runenv.RecordCrash(err)
		return
	}

	rd, wr, err := os.Pipe()
	if err != nil {
		runenv.RecordCrash(err)
		return
	}

	w := io.MultiWriter(errfile, os.Stderr)
	os.Stderr = wr

	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)

		_, err := io.Copy(w, rd)
		if err != nil && !strings.Contains(err.Error(), "file already closed") {
			runenv.RecordCrash(fmt.Errorf("stderr copy failed: %w", err))
			return
		}

		if err = errfile.Sync(); err != nil {
			runenv.RecordCrash(fmt.Errorf("stderr file tee sync failed failed: %w", err))
			return
		}
	}()

	// Prepare the event.
	defer func() {
		if err := recover(); err != nil {
			// Handle panics.
			runenv.RecordCrash(err)
		}
	}()

	err = tc(runenv)
	switch err {
	case nil:
		runenv.RecordSuccess()
	default:
		runenv.RecordFailure(err)
	}

	_ = rd.Close()
	<-doneCh
}
