module github.com/ipfs/testground/sdk/sync

go 1.13

require (
	github.com/go-redis/redis v6.15.5+incompatible
	github.com/hashicorp/go-multierror v1.0.0
	github.com/ipfs/testground/sdk/runtime v0.0.0-00010101000000-000000000000
	github.com/libp2p/go-libp2p-core v0.2.2
)

replace github.com/ipfs/testground/sdk/runtime => ../runtime
