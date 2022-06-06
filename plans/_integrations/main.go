package main

import (
	"errors"

	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/testground/plans/integrations/shim"
)

var testcases = map[string]interface{}{
	"issue-1337-override-builder-configuration": run.InitializedTestCaseFn(overrideBuilderConfiguration),
	"issue-1349-silent-failure":                 silentFailure,
}

func main() {
	run.InvokeMap(testcases)
}

func overrideBuilderConfiguration(runenv *runtime.RunEnv, initCtx *run.InitContext) error {
	version := shim.Version()
	expectedVersion := runenv.StringParam("expected_version")
	runenv.RecordMessage("Builder Configuration run with version: %s, expected version: %s", version, expectedVersion)

	if expectedVersion != version {
		return errors.New("expected version does not match")
	}

	return nil
}

func silentFailure(runenv *runtime.RunEnv) error {
	runenv.RecordMessage("This fails by NOT returning an error and NOT sending a test success status.")
	return nil
}
