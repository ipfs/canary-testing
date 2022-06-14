#!/bin/bash
my_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$my_dir/header.sh"

testground plan import --from ./plans --name testground

pushd $TEMPDIR

testground build single \
    --plan testground/placebo \
    --builder docker:go \
    --wait | tee build.out
export ARTIFACT=$(awk -F\" '/generated build artifact/ {print $8}' build.out)
docker tag $ARTIFACT testplan:placebo

testground healthcheck --runner local:docker --fix

testground run single \
    --plan=testground/placebo \
    --testcase=ok \
    --builder=docker:go \
    --use-build=testplan:placebo \
    --runner=local:docker \
    --instances=1 \
    --collect \
    --wait | tee run.out

assert_run_output_is_correct run.out

popd

echo "terminating remaining containers"
testground terminate --runner local:docker
testground terminate --builder docker:go
