# Setting up a self-managed Kubernetes cluster with kops on AWS for Testground

In this directory, you will find:

```
» tree
.
├── README.md
└── kops-weave                     # Kubernetes resources for setting up networking with Weave and Flannel
```

## Introduction

Kubernetes Operations (kops) is a tool which helps to create, destroy, upgrade and maintain production-grade Kubernetes clusters from the command line. We use it to create a k8s cluster on AWS.

We use CoreOS Flannel for networking on Kubernetes - for the default Kubernetes network, which in Testground terms is called the `control` network.

We use Weave for the `data` plane on Testground - a secondary overlay network that we attach containers to on-demand.

`kops` uses 100.96.0.0/11 for pod CIDR range, so this is what we use for the `control` network.

Weave by default uses 10.32.0.0/11 as CIDR, so this is the CIDR for the Testground `data` network. The `sidecar` is responsible for setting up the `data` network for every testplan instance.

In order to have two different networks attached to pods in Kubernetes, we run the [CNI-Genie CNI](https://github.com/cni-genie/CNI-Genie).


## Requirements

1. [kops](https://github.com/kubernetes/kops/releases). >= 1.17.0-alpha.1
2. [AWS CLI](https://aws.amazon.com/cli)
3. [helm](https://github.com/helm/helm)

## Set up infrastructure with kops

1. [Configure your AWS credentials](https://docs.aws.amazon.com/cli/)

2. Create a bucket for `kops` state. This is similar to Terraform state bucket.

```
aws s3api create-bucket \
    --bucket kops-backend-bucket \
    --region eu-central-1 --create-bucket-configuration LocationConstraint=eu-central-1
```

3. Pick up
- a cluster name,
- set AWS zone
- set `kops` state store bucket
- set number of worker nodes
- set location for cluster spec to be generated
- set location of your SSH public key

You might want to add them to your `rc` file (`.zshrc`, `.bashrc`, etc.)

```
export NAME=my-first-cluster-kops.k8s.local
export ZONES=eu-central-1a
export KOPS_STATE_STORE=s3://kops-backend-bucket
export WORKER_NODES=4
export CLUSTER_SPEC=~/cluster.yaml
export PUBKEY=~/.ssh/id_rsa.pub
```

4. Generate the cluster spec. You could reuse it next time you create a cluster.

```
kops create cluster \
  --zones $ZONES \
  --master-zones $ZONES \
  --master-size c5.2xlarge \
  --node-size c5.2xlarge \
  --node-count $WORKER_NODES \
  --networking flannel \
  --name $NAME \
  --dry-run \
  -o yaml > $CLUSTER_SPEC
```

5. Update `kubelet` section in spec with:
```
  kubelet:
    anonymousAuth: false
    maxPods: 200
    allowedUnsafeSysctls:
    - net.core.somaxconn
```

6. Set up Helm and add the `stable` Helm Charts repository

```
helm repo add stable https://kubernetes-charts.storage.googleapis.com/
helm repo update
```

7. Apply cluster setup and install Testground dependencies
```
./install.sh $NAME $CLUSTER_SPEC $PUBKEY $WORKER_NODES
```


## Destroy the cluster when you're done working on it

```
kops delete cluster $NAME --yes
```


## Configure and run your Testground daemon

```
testground --vv daemon
```


## Run a Testground testplan

Use compositions: [/docs/COMPOSITIONS.md](../../docs/COMPOSITIONS.md).

or

```
testground --vv run single network/ping-pong \
    --builder=docker:go \
    --runner=cluster:k8s \
    --build-cfg bypass_cache=true \
    --build-cfg push_registry=true \
    --build-cfg registry_type=aws \
    --run-cfg keep_service=true \
    --instances=2
```

or

```
testground --vv run single dht/find-peers \
    --builder=docker:go \
    --runner=cluster:k8s \
    --build-cfg push_registry=true \
    --build-cfg registry_type=aws \
    --run-cfg keep_service=true \
    --instances=16
```

## Resizing the cluster

1. Edit the cluster state and change number of nodes.

```
kops edit ig nodes
```

2. Apply the new configuration
```
kops update cluster $NAME --yes
```

3. Wait for nodes to come up and for DaemonSets to be Running on all new nodes
```
watch 'kubectl get pods'
```

## Destroying the cluster

Do not forget to delete the cluster once you are done running test plans.


## Cleanup after Testground and other useful commands

Testground is still in very early stage of development. It is possible that it crashes, or doesn't properly clean-up after a testplan run. Here are a few commands that could be helpful for you to inspect the state of your Kubernetes cluster and clean up after Testground.

1. Delete all pods that have the `testground.plan=dht` label (in case you used the `--run-cfg keep_service=true` setting on Testground.
```
kubectl delete pods -l testground.plan=dht --grace-period=0 --force
```

2. Restart the `sidecar` daemon which manages networks for all testplans
```
kubectl delete pods -l name=testground-sidecar --grace-period=0 --force
```

3. Review all running pods
```
kubectl get pods -o wide
```

4. Get logs from a given pod
```
kubectl logs <pod-id, e.g. tg-dht-c95b5>
```


## Known issues and future improvements

- [ ] 1. Kubernetes cluster creation - we intend to automate this, so that it is one command in the future, most probably with `terraform`.

- [ ] 2. Testground dependencies - we intend to automate this, so that all dependencies for Testground are installed with one command, or as a follow-up provisioner on `terraform` - such as `redis`, `sidecar`, etc.

- [ ] 3. Alerts (and maybe auto-scaling down) for idle clusters, so that we don't incur costs.

- [X] 4. We need to decide where Testground is going to publish built docker images - DockerHub? or? This might incur a lot of costs if you build a large image and download it from 100 VMs repeatedly.
Resolution: For now we are using AWS ECR, as clusters are also on AWS.
