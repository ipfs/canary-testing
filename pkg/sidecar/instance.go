//+build linux

package sidecar

import (
	"context"
	"io"

	"github.com/hashicorp/go-multierror"

	"github.com/ipfs/testground/pkg/logging"
	"github.com/ipfs/testground/sdk/runtime"
	"github.com/ipfs/testground/sdk/sync"
)

type InstanceHandler func(context.Context, *Instance) error

type Reactor interface {
	io.Closer

	Handle(context.Context, InstanceHandler) error
}

// Instance is a test instance as seen by the sidecar.
type Instance struct {
	logging.Logging

	Hostname string
	Client   *sync.Client
	RunEnv   *runtime.RunEnv
	Network  Network
}

// Network is a test instance's network, as seen by the sidecar.
//
// Sidecar runners must implement this interface.
type Network interface {
	io.Closer

	ConfigureNetwork(ctx context.Context, cfg *sync.NetworkConfig) error
	ListActive() []string
}

// NewInstance constructs a new test instance handle.
func NewInstance(client *sync.Client, runenv *runtime.RunEnv, hostname string, network Network) (*Instance, error) {
	return &Instance{
		Logging:  logging.NewLogging(logging.S().With("sidecar", true, "run_id", runenv.TestRun).Desugar()),
		Hostname: hostname,
		RunEnv:   runenv,
		Network:  network,
		Client:   client,
	}, nil
}

// Close closes the instance. It should not be used after closing.
func (inst *Instance) Close() error {
	var err *multierror.Error
	err = multierror.Append(err, inst.Network.Close())
	return err.ErrorOrNil()
}
