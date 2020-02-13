#!/bin/bash

set -o errexit
set -o pipefail

START_TIME=`date +%s`

echo "Creating cluster for Testground..."
echo

NAME=$1
CLUSTER_SPEC=$2
WORKER_NODES=$3

echo "Name: $NAME"
echo "Cluster spec: $CLUSTER_SPEC"
echo "Worker nodes: $WORKER_NODES"
echo

if [[ -z ${ASSETS_ACCESS_KEY} ]]; then
  echo "ASSETS_ACCESS_KEY is not set. Make sure you set credentials and location for S3 outputs bucket."
  exit 1
fi

if [[ -z ${ASSETS_SECRET_KEY} ]]; then
  echo "ASSETS_SECRET_KEY is not set. Make sure you set credentials and location for S3 outputs bucket."
  exit 1
fi

if [[ -z ${ASSETS_BUCKET_NAME} ]]; then
  echo "ASSETS_BUCKET_NAME is not set. Make sure you set credentials and location for S3 outputs bucket."
  exit 1
fi

if [[ -z ${ASSETS_S3_ENDPOINT} ]]; then
  echo "ASSETS_S3_ENDPOINT is not set. Make sure you set credentials and location for S3 outputs bucket."
  exit 1
fi

kops create -f $CLUSTER_SPEC
kops create secret --name $NAME sshpublickey admin -i ~/.ssh/testground_rsa.pub
kops update cluster $NAME --yes

## wait for worker nodes and master to be ready
echo "Wait for Cluster nodes to be Ready..."
echo
READY_NODES=0
while [ "$READY_NODES" -ne $(($WORKER_NODES + 1)) ]; do READY_NODES=$(kubectl get nodes 2>/dev/null | grep -v NotReady | grep Ready | wc -l || true); echo "Got $READY_NODES ready nodes"; sleep 5; done;

echo "Cluster nodes are Ready"
echo

echo "Add secret for S3 bucket"
echo
kubectl create secret generic assets-s3-bucket --from-literal=access-key="$ASSETS_ACCESS_KEY" \
                                               --from-literal=secret-key="$ASSETS_SECRET_KEY" \
                                               --from-literal=s3-endpoint="$ASSETS_S3_ENDPOINT" \
                                               --from-literal=bucket-name="$ASSETS_BUCKET_NAME"


echo "Install Weave, CNI-Genie, s3bucket DaemonSet, Sidecar Daemonset..."
echo
kubectl apply -f ./kops-weave/genie-plugin.yaml \
              -f ./kops-weave/weave.yml \
              -f ./kops-weave/s3bucket.yml \
              -f ./sidecar.yaml

echo "Install Redis..."
echo
helm install redis stable/redis --values ./redis-values.yaml


echo "Wait for Sidecar to be Ready..."
echo
RUNNING_SIDECARS=0
while [ "$RUNNING_SIDECARS" -ne "$WORKER_NODES" ]; do RUNNING_SIDECARS=$(kubectl get pods | grep testground-sidecar | grep Running | wc -l || true); echo "Got $RUNNING_SIDECARS running sidecar pods"; sleep 5; done;

echo "Testground cluster is ready"
echo

END_TIME=`date +%s`
echo "Execution time was `expr $END_TIME - $START_TIME` seconds"
