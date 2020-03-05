module github.com/ipfs/testground/sdk/sync

go 1.13

require (
	github.com/go-redis/redis/v7 v7.0.0-beta.4
	github.com/ipfs/testground v0.1.0
	github.com/ipfs/testground/sdk/runtime v0.1.0
	github.com/libp2p/go-libp2p-core v0.2.3
	github.com/libp2p/go-openssl v0.0.4 // indirect
	github.com/multiformats/go-multiaddr v0.1.1
	golang.org/x/lint v0.0.0-20200130185559-910be7a94367 // indirect
	golang.org/x/tools v0.0.0-20200227222343-706bc42d1f0d // indirect
)

replace github.com/ipfs/testground/sdk/runtime => ../runtime
