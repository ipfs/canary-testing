package sync

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis/v7"
)

func (s *DefaultService) Barrier(ctx context.Context, state string, target int64) (err error) {
	if target == 0 {
		s.log.Warnw("requested a barrier with target zero; satisfying immediately", "state", state)
		return nil
	}

	b := &barrier{
		key:      state,
		target:   target,
		ctx:      ctx,
		doneCh:   make(chan error, 1),
		resultCh: make(chan error),
	}

	s.barrierCh <- b
	err = <-b.resultCh
	if err != nil {
		return err
	}

	err = <-b.doneCh
	return err
}

func (s *DefaultService) SignalEntry(ctx context.Context, state string) (seq int64, err error) {
	s.log.Debugw("signalling entry to state", "key", state)

	// Increment a counter on the state key.
	seq, err = s.rclient.Incr(state).Result()
	if err != nil {
		return 0, err
	}

	s.log.Debugw("new value of state", "key", state, "value", seq)
	return seq, err
}

func (s *DefaultService) SignalEvent(ctx context.Context, key string, event interface{}) (err error) {
	ev, err := json.Marshal(event)
	if err != nil {
		return err
	}

	args := &redis.XAddArgs{
		Stream: key,
		ID:     "*",
		Values: map[string]interface{}{RedisPayloadKey: ev},
	}

	_, err = s.rclient.XAdd(args).Result()
	return err
}
