package cmd_test

import (
	"testing"
)

func TestList(t *testing.T) {
	err := runSingle(t,
		"list",
	)

	if err != nil {
		t.Fatal(err)
	}
}
