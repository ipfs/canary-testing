# InterPlanetary TestGround

![](https://img.shields.io/badge/go-%3E%3D1.13.0-blue.svg?style=flat-square)

> ⚠️ **Heavy WIP.** beware of the Dragons 🐉..

> **This repository is incubating the InterPlanetary Testground. 🐣**

## Description

You may have noticed a few test efforts with similar names underway! Testing at scale is a hard problem. We are indeed exploring and experimenting a lot, until we land on an end-to-end solution that works for us.

-  Interplanetary Testbed (IPTB): https://github.com/ipfs/iptb
  - a simple utility to manage local clusters/aggregates of IPFS instances.
- libp2p testlab: https://github.com/libp2p/testlab
  - a Nomad deployer for libp2p nodes with a DSL for test scenarios.
- And others such as https://github.com/ipfs/interop and https://github.com/ipfs/benchmarks

The Interplanetary Test Ground aims to leverage the learnings and tooling resulting from those efforts to provide a scalable runtime environment for the execution of various types of tests and benchmarks, written in different languages, by different teams, targeting a specific commit of IPFS and/or libp2p, and quantifying its characteristics in terms of performance, resource and network utilisation, stability, interoperability, etc., when compared to other commits.

The Interplanetary Test Ground aims to be tightly integrated with the software engineering practices and tooling the IPFS and libp2p teams rely on.

## Team

The current TestGround Team is composed of:

- @raulk - Lead Architect, Engineer, Developer
- @daviddias - Engineer, Developer, acting as interim PM for the project
- @jimpick - Engineer, Developer, Infrastructure Lead
- you! Yes, you can contribute as well, however, do understand that this is a brand new and fast moving project and so contributing might require extra time to onboard

We run a Weekly Sync at 4pm Tuesdays on [Zoom Room](https://protocol.zoom.us/j/299213319), notes are taken at [hackmd.io test-ground-weekly/edit](https://hackmd.io/@daviddias/test-ground-weekly/edit?both) and stored at [meeting-notes](https://github.com/ipfs/testground/tree/master/_meeting-notes). This weekly is listed on the [IPFS Community Calendar](https://github.com/ipfs/community#community-calendar).

## Architecture

Refer to the [specification](docs/SPEC.md) document.

## Repo Structure

```
├── README.md                       # This file
├── main.go                         # TestGround entrypoint file
├── cmd                             # TestGround CLI comamnds
│   ├── all.go
│   ├── ...
├── manifests                       # Manifests for each test Plan. These exist independent from plans to enable plans to live elsewhere
│   ├── dht.toml
│   └── smlbench.toml
├── plans                           # The Test Plan. Includes Image to be run, Assertions and more
│   ├── dht
│   └── smlbench
├── sdk                             # SDK available to each test plan
│   ├── runtime
│   └── ...
├── docs                            # Documentation of the project
│   ├── SPEC.md
│   ├── ...
├── pkg                             # Internals to TestGround
│   ├── api
│   ├── ...
└── tools                           # ??
    └── src_generate.go
```

## Contributing & Testing

We kindly ask you to read through the SPEC first and give this project a run first in your local machine. It is a fast moving project at the moment and it might require some tinkering and experimentation to compesate the lack of documentation.

### Setup

Ensure that you are running go 1.13 or later (for gomod support)

```sh
> go version
go version go1.13.1 darwin/amd64
```

Then, onto getting the actual Test Ground code. Download the repo and install the dependencies

```sh
> go get github.com/ipfs/testground
# ..fetch and install logs
> cd $GOPATH/src/github.com/ipfs/testground
```

This command may take a couple of minutes to complete. If successful, it will end with no message.

Now test that everything is installed correctly by running

```sh
> TESTGROUND_BASEDIR=`pwd` testground
resolved testground base dir from env variable: /Users/imp/code/go-projects/src/github.com/ipfs/testground
NAME:
   testground - A new cli application

   USAGE:
      testground [global options] command [command options] [arguments...]

   COMMANDS:
      run      (builds and) runs test case with name `testplan/testcase`
      list     list all test plans and test cases
      build    builds a test plan
      help, h  Shows a list of commands or help for one command

   GLOBAL OPTIONS:
      -v          verbose output (equivalent to INFO log level)
      --vv        super verbose output (equivalent to DEBUG log level)
     --help, -h  show help
```

### Running the tests locally with TestGround

To run a test locally, you can use the `testground run` command. Check what Plans and Tests are available by running the `list` command:

```
> TESTGROUND_BASEDIR=`pwd` testground list
resolved testground base dir from env variable: /Users/imp/code/go-projects/src/github.com/ipfs/testground
dht/lookup-peers
dht/lookup-providers
dht/store-get-value
smlbench/lookup-peers
smlbench/lookup-providers
smlbench/store-get-value
```

This next command is your first test! It runs the lookup-peers test from the DHT plan, using the builder (which sets up the environment + compilation) named docker:go (which compiles go inside docker) and runs it using the runner local:docker (which runs on your local machine).

```
> TESTGROUND_BASEDIR=`pwd` testground -vv run dht/lookup-peers --builder=docker:go --runner=local:docker --build-cfg bypass_cache=true
...
```

You should see a bunch of logs that describe the steps of the test, from:

* Setting up the container
* Compilation of the test case inside the container
* Starting the containers (total of 50 as 50 is the default number of nodes for this test)
* You will see the logs that describe each node connecting to the others and executing a kademlia find-peers action.

### Running a test outside of TestGround orchestrator

You must have a redis instance running locally. Install it for your runtime follow instruction at https://redis.io/download.

Then run it locally with

```
> redis server
# ...
93801:M 03 Oct 2019 14:42:52.430 * Ready to accept connections
```

Then move into the folder that has the plan and test you want to run locally. Execute it by sessting the TEST_CASE & TEST_CASE_SEQ env variables

```
> cd plans/dht
> TEST_CASE="lookup-peers" TEST_CASE_SEQ="0" go run main.go
# ... test output
```

### Running a Test Plan on the TestGround Cloud Infrastructure

`To be Written once such infrastructure exists..soon™`

## Contributing

[![](https://cdn.rawgit.com/jbenet/contribute-ipfs-gif/master/img/contribute.gif)](https://github.com/ipfs/community/blob/master/CONTRIBUTING.md)

This repository falls under the IPFS [Code of Conduct](https://github.com/ipfs/community/blob/master/code-of-conduct.md).

You can contact us on the freenode #ipfs-dev channel or attend one of our [weekly calls](https://github.com/ipfs/team-mgmt/issues/674).

## License

Dual-licensed: [MIT](./LICENSE-MIT), [Apache Software License v2](./LICENSE-APACHE), by way of the [Permissive License Stack](https://protocol.ai/blog/announcing-the-permissive-license-stack/).
