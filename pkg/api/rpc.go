package api

import (
	"bytes"
)

// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
// ~~~~~~ Request payloads ~~~~~~
// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

// DescribeRequest is the request struct for the `describe` function.
type DescribeRequest struct {
	Term string `json:"term"`
}

// BuildRequest is the request struct for the `build` function.
type BuildRequest struct {
	Composition Composition `json:"composition"`
}

// RunRequest is the request struct for the `run` function.
type RunRequest struct {
	Composition Composition `json:"composition"`
}

type OutputsRequest struct {
	Runner string `json:"runner"`
	RunID  string `json:"run_id"`
}

type TerminateRequest struct {
	Runner string `json:"runner"`
}

type HealthcheckRequest struct {
	Runner string `json:"runner"`
	Fix    bool   `json:"fix"`
}

// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
// ~~~~~~ Response payloads ~~~~~~
// ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

// BuildResponse is the response struct for the `build` function.
type BuildResponse = []BuildOutput

type RunResponse = RunOutput

type CollectResponse struct {
	File   bytes.Buffer
	Exists bool
}

type HealthcheckResponse = HealthcheckReport
