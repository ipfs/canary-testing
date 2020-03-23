package cmd_test

import (
	"testing"
)

func TestSidecar(t *testing.T) {
	t.Skip("Skipping flaky test")

	err := runSingle(t,
		"run",
		"single",
		"network/ping-pong",
		"--builder",
		"docker:go",
		"--runner",
		"local:docker",
		"--instances",
		"2",
	)

	if err != nil {
		t.Fail()
	}
}
