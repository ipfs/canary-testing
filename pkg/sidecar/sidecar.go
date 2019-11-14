package sidecar

import (
	"context"
	"fmt"
	"io"

	"github.com/ipfs/testground/sdk/sync"
)

var runners = map[string]func() (InstanceManager, error){
	"docker": NewDockerManager,
	// TODO: local
}

type InstanceManager interface {
	io.Closer
	Manage(context.Context, func(context.Context, *Instance) error) error
}

func Run(runnerName string) error {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	runner, ok := runners[runnerName]
	if !ok {
		return fmt.Errorf("sidecar runner %s not found", runnerName)
	}

	manager, err := runner()
	if err != nil {
		return err
	}

	defer manager.Close()

	return manager.Manage(ctx, func(ctx context.Context, instance *Instance) error {
		defer func() {
			if err := instance.Close(); err != nil {
				instance.S().Warnf("failed to close instance: %s", err)
			}
		}()

		/* TODO: Initialize all networks to the "down" state.
		for _, n := range instance.Network.ListActive() {
			instance.Network.ConfigureNetwork(&sync.NetworkConfig{
				Network: n,
				Enable:  false,
			})
		}
		*/

		// Wait for all the sidecars to enter the "network-initialized" state.
		const netInitState = "network-initialized"
		if _, err = instance.Writer.SignalEntry(netInitState); err != nil {
			return err
		}
		instance.S().Infof("waiting for all networks to be ready")
		if err := <-instance.Watcher.Barrier(
			ctx,
			netInitState,
			int64(instance.RunEnv.TestInstanceCount),
		); err != nil {
			return err
		}
		instance.S().Infof("all networks ready")

		// Now let the test case tell us how to configure the network.
		subtree := sync.NetworkSubtree(instance.Hostname)
		networkChanges := make(chan *sync.NetworkConfig, 16)
		closeSub, err := instance.Watcher.Subscribe(subtree, networkChanges)
		if err != nil {
			return err
		}
		defer func() {
			if err := closeSub(); err != nil {
				instance.S().Warnf("failed to close sub: %s", err)
			}
		}()

		for cfg := range networkChanges {
			instance.S().Infow("applying network change", "network", cfg)
			if err := instance.Network.ConfigureNetwork(ctx, cfg); err != nil {
				return fmt.Errorf("failed to update network %s: %w", cfg.Network, err)
			}
			_, err := instance.Writer.SignalEntry(cfg.State)
			if err != nil {
				return err
			}
		}

		return nil
	})
}
