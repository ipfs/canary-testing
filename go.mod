module github.com/ipfs/testground

go 1.13

replace (
	github.com/ipfs/testground/sdk/iptb => ./sdk/iptb
	github.com/ipfs/testground/sdk/runtime => ./sdk/runtime
	github.com/ipfs/testground/sdk/sync => ./sdk/sync
	github.com/miekg/dns => github.com/miekg/dns v1.0.14
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78 // indirect
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/Microsoft/hcsshim v0.8.6 // indirect
	github.com/aws/aws-sdk-go v1.25.32
	github.com/containerd/containerd v1.3.0 // indirect
	github.com/containerd/continuity v0.0.0-20190827140505-75bee3e2ccb6 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v1.4.2-0.20191112174331-c83188248e9c
	github.com/docker/go-connections v0.4.1-0.20190612165340-fd1b1942c4d5 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/google/go-cmp v0.3.1 // indirect
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.3 // indirect
	github.com/hashicorp/go-getter v1.4.0
	github.com/hashicorp/go-multierror v1.0.0
	github.com/imdario/mergo v0.3.8
	github.com/ipfs/testground/sdk/runtime v0.0.0-00010101000000-000000000000
	github.com/logrusorgru/aurora v0.0.0-20191017060258-dc85c304c434
	github.com/mitchellh/go-wordwrap v1.0.0
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/otiai10/copy v1.0.2
	github.com/sirupsen/logrus v1.4.2 // indirect
	github.com/urfave/cli v1.22.1
	go.uber.org/zap v1.12.0
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20191110163157-d32e6e3b99c4 // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	google.golang.org/grpc v1.25.1 // indirect
	gotest.tools v2.2.0+incompatible // indirect
)
