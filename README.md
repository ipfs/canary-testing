# Testground

![Testground logo](https://raw.githubusercontent.com/testground/pm/master/logo/TG_Banner_GitHub.jpg)

[![](https://img.shields.io/badge/made%20by-Protocol%20Labs-blue.svg?style=flat-square)](http://protocol.ai)
![](https://img.shields.io/badge/go-%3E%3D1.14.0-blue.svg)
[![CircleCI](https://circleci.com/gh/testground/testground.svg?style=svg)](https://circleci.com/gh/testground/testground)

Testground is a platform for testing, benchmarking, and simulating distributed and p2p
systems at scale. It's designed to be multi-lingual and runtime-agnostic, scaling gracefully
from 2 to 10k instances, only when needed.

![Testground demo](https://github.com/testground/pm/blob/master/img/testground-demo.gif?raw=true)

## Table of Contents

<!-- INSTRUCTIONS: regenerate with [doctoc](https://github.com/thlorenz/doctoc) -->
<!-- Command: doctoc README.md --maxlevel=2 -->

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


- [Getting started](#getting-started)
- [How does it work?](#how-does-it-work)
- [Features](#features)
- [Documentation](#documentation)
- [Where to find test plans?](#where-to-find-test-plans)
- [Developing test plans](#developing-test-plans)
- [Scaling out](#scaling-out)
- [Contributing](#contributing)
- [Team](#team)
- [License](#license)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Getting started

Currently, we don't distribute binaries, so you will have to build from source.

***Prerequisites: Go 1.14+, Docker daemon running.***

```shell script
$ git clone https://github.com/testground/testground.git
$ cd testground
$ make install       # builds testground and the Docker image, used by the local:docker runner.
$ testground daemon  # will start the daemon listening on localhost:8042 by default.
$ ###### WIP, clone a test plan into $TESTGROUND_HOME/plans, and run it locally. ###### 
``` 

**`$TESTGROUND_HOME` is an important directory.** If not explicitly set, testground uses `$HOME/testground` as a default.

The layout of **`$TESTGROUND_HOME`** is as follows:

```
$TESTGROUND_HOME
 |
 |__ plans              >>> [c] contains test plans, can be git checkouts, symlinks to local dirs, or the source itself
 |    |__ suite-a       >>> test plans can be grouped in suites (which in turn can be nested); this enables you to host many test plans in a single repo / directory.
 |    |    |__ plan-1   >>> source of a test plan identified by suite-a/plan-1 (relative to $TESTGROUND_HOME/plans) 
 |    |    |__ plan-2
 |    |__ plan-3        >>> source of a test plan identified by plan-3 (relative to $TESTGROUND_HOME/plans)
 |
 |__ sdks               >>> [c] hosts the test development SDKs that the client knows about, so they can be used with the --link-sdk option.
 |    |__ sdk-go
 |
 |__ data               >>> [d] data directory  
      |__ outputs
      |__ work
 
[c] = used client-side // [d] = used mostly daemon-side.
``` 

**Follow the [Quick Start Tutorial](https://protocol-labs.gitbook.io/testground/test-plan-sdk/quickstart) to get up and running with your first testplan! 🚀**

## How does it work?

1. **You develop distributed test plans as if you were writing unit tests against local APIs.**
    - No puppeteering necessary.
    - No need to package and ship the system or component under test as a separate daemon.
    - No need to expose every internal setting over an external API, just for the sake of testing.
    
2. **Your test plan calls out to the coordination API to:**
    - communicate out-of-band information (such as endpoint addresses, peer ids, etc.)
    - leverage synchronization and ordering primitives such as signals and barriers to model a
       distributed state machine.
    - programmatically apply network traffic shaping policies, which you can alter during the
       execution of a test to simulate various network conditions.
       
3. **There is no special "conductor" node telling instances what to do when.** The choreography and
   sequencing emerges from within the test plan itself.
   
4. **You decide what versions of the upstream software you want to exercise your test against.**
     - Benchmark, simulate, experiment, run attacks, etc. against versions v1.1 and v1.2 of the
       components under test in order to compare results, or test compatibility.
     - Assemble hybrid test runs mixing various versions of the dependency graph.
     
5. **Inside your test plan:**
     - You record observations, metrics, success/failure statuses.
     - You emit structured or unstructured assets you want collected, such as event logs,
       dumps, snapshots, binary files, etc.
        
6. **Via a TOML-based _composition_ file, you instruct Testground to:**
     - Assemble a test run comprising groups of 2, 200, or 10000 instances, each with different
       test parameters, or built against different depencency sets.
     - Schedule them for run locally (executable or Docker), or in a cluster (Kubernetes).
     
7. **You collect the outputs of the test plan with a single command,** and use data processing scripts and
   platforms (such as the upcoming Jupyter notebooks integration) to draw conclusions.

## Features

### Supports (or aims to support) a variety of testing workflows 💡

> (🌕 = fully supported // 🌑 = planned)

  * Experimental/iterative development 🌖 (The team at Protocol Labs has used Testground extensively to evaluate
    protocol changes in large networks, simulate attacks, measure algorithmic improvements across network boundaries,
    etc.) 
  * Debugging 🌗
  * Comparative testing 🌖
  * Backwards/forward-compatibility testing 🌖 
  * Interoperability testing 🌑
  * Continuous integration 🌑
  * Stakeholder/acceptance testing 🌑

### Simple, normalized, formal runtime environment for tests 📄

A test plan is a blackbox with a formal contract. Testground promises to inject a set of env variables, and the test
plan promises to emit events on stdout, and assets on the output directory.
  * As such, a test plan can be any kind of program, written in Go, JavaScript, C, or shell.
  * At present, we offer builders for Go, with TypeScript (node and browser) being in the works.  

### Modular builders and runners 🛠

For running test plans written in different languages, targeted for different runtimes, and levels of scale:
  * `exec:go` and `docker:go` builders: compile test plans written in Go into executables or containers.
  * `local:exec`, `local:docker`, `cluster:k8s` runners: run executables or containers locally
    (suitable for 2-300 instances), or in a Kubernetes cloud environment (300-10k instances).

> Got some spare cycles and would like to add support for writing test plans Rust, Python or X? It's easy! Open an
> issue, and the community will guide you!

### Distributed coordination API 👯‍♀️

Redis-backed lightweight API offering synchronisation primitives to coordinate and choreograph distributed test
workloads across a fleet of nodes.

### Network traffic shaping ☎️

Test instances are able to set connectedness, latency, jitter, bandwidth, duplication, packet corruption, etc. to
simulate a variety of network conditions.

### Quickstart k8s cluster setup on AWS ☁️

Create a k8s cluster ready to run Testground jobs on AWS by following the instructions at
[`testground/infra`](https://github.com/testground/infra).

### Upstream dependency selection 🧩

Compiling test plans against specific versions of upstream dependencies (e.g. moduleX v0.3, or commit 1a2b3c).

### Dealing with upstream API changes 🌱

So that a single test plan can work with a range of versions of the components under test, as these evolve over time.

### Results and diagnostics, raw and aggregated data points 📈

**Diagnostics:** Automatic diagnostics via pprof (for Go test plans), with metrics emitted to InfluxDB in real-time. Metrics can be raw data points or aggregated measurements, such as histograms, counters, gauges, moving averages, etc.

**Results:** When the test plan concludes, all results are pushed in batch to InfluxDB for later exploration, analysis, and visualization.

### Declarative jobs, we call them _compositions_ 🎼

Create tailored test runs by composing scenarios declaratively, with different groups, cohorts, upstream deps, test
params, etc. 

### Emit and collect test outputs 💾

Emit and collect/export/download test outputs (logs, assets, event trails, run events, etc.) from all participants
in a run. 


## Documentation

\<Full documentation site at WIP>

## Where to find test plans?

There are some basic, project-agnostic Testground test plans in the [`plans`](./plans) directory.
We use these plans to validate the functionality of Testground itself.

To link them under `$TESTGROUND_HOME/plans`, if you're using default paths, these commands should do the trick
(assuming you have already run the Testground daemon once, and therefore the `$TESTGROUND_HOME` layout has been created
for you):

```shell script
$ # from the root of this repo, run the following; it will symlink all test plans under $TESTGROUND_HOME/plans
$ ln -s $PWD/plans/* $HOME/testground/plans
$ testground run single --plan network --testcase ping-pong --builder=docker:go --runner=local:docker --instances=2
```

For project-specific test plans, check out these repos:

* https://github.com/libp2p/test-plans
* https://github.com/ipfs/test-plans

To use them, import them into `$TESTGROUND_HOME/plans` using the following testground commands:

```shell script
$ testground plan import --git --source https://github.com/libp2p/test-plans.git --name libp2p
$ testground plan import --git --source https://github.com/ipfs/test-plans.git --name ipfs
$ # to run the find-peers test case from the libp2p/dht test plan (this is not a complete command!)
$ testground run single --plan libp2p/dht --testcase find-peers --builder docker:go --runner local:docker <options>
``` 


## Developing test plans

\<WIP>

## Scaling out 

<\WIP>

## Contributing

Please read our [CONTRIBUTING Guidelines](./CONTRIBUTING.md) before making a contribution.

## Team

### Core team 💪

* [@raulk](https://github.com/raulk) 🎈 _(founder and tech lead)_
* [@nonsense](https://github.com/nonsense) ⛷ _(core engineer)_
* [@coryschwartz](https://github.com/coryschwartz) 🦉 _(core engineer)_
* [@robmat05](https://github.com/robmat05) 🍝 _(technical project manager)_

### Collaborators ❤

[@daviddias](https://github.com/daviddias), [@stebalien](https://github.com/stebalien), [@hacdias](https://github.com/hacdias), [@jimpick](https://github.com/jimpick), [@aschmahmann](https://github.com/aschmahmann), [@dirkmc](https://github.com/dirkmc), [@yusefnapora](https://github.com/yusefnapora).

### Meeting notes

You can find notes from the Testground team meetings at [github.com/testground/pm](https://github.com/testground/pm/tree/master/_meeting-notes)

## License

Dual-licensed: [MIT](./LICENSE-MIT), [Apache Software License v2](./LICENSE-APACHE), by way of the
[Permissive License Stack](https://protocol.ai/blog/announcing-the-permissive-license-stack/).
