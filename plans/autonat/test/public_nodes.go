package test

import (
	"context"
	"fmt"
	"time"

	"github.com/ipfs/testground/sdk/runtime"
	"github.com/ipfs/testground/sdk/sync"
	autonat "github.com/libp2p/go-libp2p-autonat"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/network"
)

func PublicNodes(runenv *runtime.RunEnv) error {
	opts := &SetupOpts{
		Timeout:    time.Duration(runenv.IntParam("timeout_secs")) * time.Second,
		NBootstrap: runenv.IntParam("n_bootstrap"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	watcher, writer := sync.MustWatcherWriter(ctx, runenv)
	defer watcher.Close()
	defer writer.Close()

	node, peers, seq, err := Setup(ctx, runenv, watcher, writer, opts)
	if err != nil {
		return err
	}

	// Listen to the node's autonat status.
	sub, _ := node.EventBus().Subscribe(new(event.EvtLocalReachabilityChanged))
	defer sub.Close()

	var statuses []network.Reachability
	go func() {
		for {
			select {
			case ev, ok := <-sub.Out():
				if !ok {
					return
				}
				evt, ok := ev.(event.EvtLocalReachabilityChanged)
				if !ok {
					continue
				}
				if evt.Reachability == network.ReachabilityPublic {
					runenv.RecordMessage("node believes it is publicly reachable")
				} else {
					runenv.RecordMessage("node believes it is a private node")
				}
				statuses = append(statuses, evt.Reachability)
			case <-ctx.Done():
				return
			}
		}
	}()

	defer Teardown(ctx, runenv, watcher, writer)

	// Bring the network into a nice, stable, bootstrapped state.
	bootstraps, err := Bootstrap(ctx, runenv, watcher, writer, node, opts, peers, seq)
	if err != nil {
		return err
	}

	// give some time for background autonat to do its thing.
	time.Sleep(autonat.AutoNATBootDelay)

	client := autonat.NewAutoNATClient(node, nil)
	// expect all of the bootstrappers to be running AutoNAT-svc.
	// expect other nodes to not be running the service.
	isBootstrap := false
	bootstrapDialed := false
	for _, p := range bootstraps {
		if p.ID == node.ID() {
			isBootstrap = true
			break
		}
	}
	for _, p := range bootstraps {
		if p.ID == node.ID() {
			if isBootstrap {
				break
			}
			continue
		}
		t := time.Now()

		ectx, cancel := context.WithCancel(ctx)

		addr, err := client.DialBack(ectx, p.ID)
		cancel()

		runenv.RecordMetric(&runtime.MetricDefinition{
			Name:           "time-to-dial",
			Unit:           "ns",
			ImprovementDir: -1,
		}, float64(time.Since(t).Nanoseconds()))

		if isBootstrap {
			bootstrapDialed = true
			if err != nil {
				runenv.RecordMessage("Dialing to bootstrap at %v didn't work.", p)
				runenv.RecordFailure(err)
			}
		} else if !isBootstrap && err == nil {
			runenv.RecordFailure(fmt.Errorf("Autonat dialing unexpectedly yielded %v", addr))
		}
	}

	// wait for in-flight network requests to resolve.
	time.Sleep(autonat.AutoNATRetryInterval)

	if len(statuses) > 3 {
		runenv.RecordFailure(fmt.Errorf("Nat status shouldn't flap, but %d statuses emitted", len(statuses)))
	} else if len(statuses) == 0 {
		if !isBootstrap || bootstrapDialed {
			runenv.RecordFailure(fmt.Errorf("Nat should have settled on public or private, but no status emitted"))
		}
		return nil
	}

	lastStatus := statuses[len(statuses)-1]
	if isBootstrap && lastStatus != network.ReachabilityPublic {
		if bootstrapDialed {
			runenv.RecordFailure(fmt.Errorf("Bootstrap node believed it had autonat status %#v", lastStatus))
		}
	} else if !isBootstrap && lastStatus != network.ReachabilityPrivate {
		runenv.RecordFailure(fmt.Errorf("Non bootstrap node believed it had autonat status %#v", lastStatus))
	}

	return nil
}
