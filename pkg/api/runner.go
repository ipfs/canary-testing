package api

import (
	"context"
	"io"
	"reflect"

	"github.com/ipfs/testground/pkg/config"
)

// Runner is the interface to be implemented by all runners. A runner takes a
// test plan in executable form and schedules a run of a particular test case
// within it.
//
// TODO cardinality: do we want to be able to run multiple test cases within a
// test plan in a single call?
type Runner interface {
	// ID returns the canonical identifier for this runner.
	ID() string

	// Run runs a test case.
	Run(ctx context.Context, job *RunInput, outputWriter io.Writer) (*RunOutput, error)

	// ConfigType returns the configuration type of this runner.
	ConfigType() reflect.Type

	// CompatibleBuilders returns the IDs of the builders whose artifacts this
	// runner can work with.
	CompatibleBuilders() []string
}

// RunInput encapsulates the input options for running a test plan.
type RunInput struct {
	// RunID is the run id assigned to this job by the Engine.
	RunID string
	// EnvConfig is the env configuration of the engine. Not a pointer to force
	// a copy.
	EnvConfig config.EnvConfig
	// Directories providers accessors to directories managed by the runtime.
	Directories Directories
	// TestPlan is the definition of the test plan containing the test case to
	// run.
	TestPlan *TestPlanDefinition
	// Instances is the number of instances to run.
	Instances int
	// ArtifactPath can be a docker image ID or an executable path; it's
	// runner-dependent.
	ArtifactPath string
	// Seq is the test case seq number to run.
	Seq int
	// Parameters are the runtime parameters to the test case.
	Parameters map[string]string
	// RunnerConfig is the configuration of the runner sourced from the test
	// plan manifest, coalesced with any user-provided overrides.
	RunnerConfig interface{}
	// BuilderID is the ID of the builder used.
	BuilderID string
}

type RunOutput struct {
	// TODO.
	// RunnerID is the ID of the runner used.
	RunnerID string `json:"runnerId"`
}
