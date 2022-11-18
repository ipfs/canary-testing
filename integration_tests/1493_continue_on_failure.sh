#!/bin/bash
my_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$my_dir/header.sh"

testground plan import --from ./plans --name testground

pushd $TEMPDIR

# run all instances
testground run composition \
    -f ${my_dir}/../plans/_integrations_runs/_compositions/issue-1493-multiple-runs-obvious-failure.toml \
    --collect                     \
    --result-file=./results.csv   \
    --wait | tee run.out

assert_runs_outcome_are ./run.out "success" "failure" "success"
# assert_runs_instance_count ./run.out 1 2 4 # TODO: IMPLEMENT
assert_runs_results ./results.csv success failure success

# run all instances
testground run composition \
    -f ${my_dir}/../plans/_integrations_runs/_compositions/issue-1493-multiple-runs-failure-with-first-run.toml \
    --collect                     \
    --result-file=./results.csv   \
    --wait | tee run.out

assert_runs_outcome_are ./run.out "failure" "failure" "success"
# assert_runs_instance_count ./run.out 1 2 4 # TODO: IMPLEMENT
assert_runs_results ./results.csv failure failure success

popd

echo "terminating remaining containers"
testground terminate --runner local:docker