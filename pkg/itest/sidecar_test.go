//go:build integration && local_docker
// +build integration,local_docker

package cmd_test

import (
	"testing"
)

func TestSidecar(t *testing.T) {
	t.Skip("Skipping flaky test")

	err := runSingle(t, nil,
		"run",
		"single",
		"--builder",
		"docker:go",
		"--runner",
		"local:docker",
		"--instances",
		"2",
		"--plan",
		"network",
		"--testcase",
		"ping-pong",
	)

	if err != nil {
		t.Fail()
	}
}
