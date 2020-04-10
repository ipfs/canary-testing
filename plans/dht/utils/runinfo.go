package utils

import (
	"github.com/ipfs/testground/sdk/runtime"
	"github.com/ipfs/testground/sdk/sync"
)

type RunInfo struct {
	RunEnv *runtime.RunEnv
	Client *sync.Client

	Groups          []string
	GroupProperties map[string]*GroupInfo
}
