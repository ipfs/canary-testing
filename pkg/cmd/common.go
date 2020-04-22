package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/urfave/cli"

	"github.com/testground/testground/pkg/api"
	"github.com/testground/testground/pkg/client"
	"github.com/testground/testground/pkg/config"
	"github.com/testground/testground/pkg/conv"
)

func setupClient(c *cli.Context) (*client.Client, *config.EnvConfig, error) {
	cfg := &config.EnvConfig{}
	if err := cfg.Load(); err != nil {
		return nil, nil, err
	}

	endpoint := c.GlobalString("endpoint")
	if endpoint != "" {
		cfg.Client.Endpoint = endpoint
	}

	cl := client.New(cfg)
	return cl, cfg, nil
}

// createSingletonComposition parses a single-style command line build/run, and
// produces a synthetic composition to submit to the server.
func createSingletonComposition(c *cli.Context) (*api.Composition, error) {
	var (
		testcase = c.Args().First()

		builder   = c.String("builder")
		runner    = c.String("runner")
		instances = c.Uint("instances")
		artifact  = c.String("use-build")

		buildcfg     = c.StringSlice("build-cfg")
		dependencies = c.StringSlice("dep")

		runcfg     = c.StringSlice("run-cfg")
		testparams = c.StringSlice("test-param")
	)

	comp := &api.Composition{
		Global: api.Global{
			Builder:        builder,
			Runner:         runner,
			TotalInstances: instances,
		},
		Groups: []api.Group{
			{
				ID: "single",
				Instances: api.Instances{
					Count: instances,
				},
				Run: api.Run{
					Artifact: artifact,
				},
			},
		},
	}

	// Translate CLI params to the composition format.
	switch ss := strings.Split(testcase, ":"); len(ss) {
	case 0:
		return nil, errors.New("wrong format for test case name, should be: `<path to testplan>:testcase`, where `<path to testplan> is relative to $TESTGROUND_HOME")
	case 2:
		comp.Global.Case = ss[1]
		fallthrough
	case 1:
		comp.Global.Plan = ss[0]
	default:
		return nil, errors.New("wrong format for test case name, should be: `<path to testplan>:testcase`, where `<path to testplan> is relative to $TESTGROUND_HOME")
	}

	// Build configuration.
	config, err := conv.ParseKeyValues(buildcfg)
	if err != nil {
		return nil, fmt.Errorf("failed while parsing build config: %w", err)
	}
	comp.Global.BuildConfig = conv.InferTypedMap(config)

	// Run configuration.
	config, err = conv.ParseKeyValues(runcfg)
	if err != nil {
		return nil, fmt.Errorf("failed while parsing run config: %w", err)
	}
	comp.Global.RunConfig = conv.InferTypedMap(config)

	// Test parameters.
	parameters, err := conv.ParseKeyValues(testparams)
	if err != nil {
		return nil, fmt.Errorf("failed while parsing test paremters: %w", err)
	}
	comp.Groups[0].Run.TestParams = parameters

	deps, err := conv.ParseKeyValues(dependencies)
	if err != nil {
		return nil, err
	}
	comp.Groups[0].Build.Dependencies = make([]api.Dependency, 0, len(dependencies))

	for name, ver := range deps {
		dep := api.Dependency{
			Module:  name,
			Version: ver,
		}
		comp.Groups[0].Build.Dependencies = append(comp.Groups[0].Build.Dependencies, dep)
	}

	// Validate the composition before returning it.
	switch c := strings.Fields(c.Command.FullName()); c[0] {
	case "build":
		err = comp.ValidateForBuild()
	case "run":
		err = comp.ValidateForRun()
	default:
		err = errors.New("unexpected command")
	}

	return comp, err
}

// resolveTestPlan resolves a test plan, returning its root directory and its
// parsed manifest.
func resolveTestPlan(cfg *config.EnvConfig, name string) (string, *api.TestPlanManifest, error) {
	baseDir := cfg.Dirs().Plans()

	// Resolve the test plan directory.
	path := filepath.Join(baseDir, filepath.FromSlash(name))
	if !isDirectory(path) {
		return "", nil, fmt.Errorf("failed to locate plan in directory: %s", path)
	}

	manifest := filepath.Join(path, "manifest.toml")
	switch fi, err := os.Stat(manifest); {
	case err != nil:
		return "", nil, fmt.Errorf("failed to access plan manifest at %s: %w", manifest, err)
	case fi.IsDir():
		return "", nil, fmt.Errorf("failed to access plan manifest at %s: not a file", manifest)
	}

	plan := new(api.TestPlanManifest)
	if _, err := toml.DecodeFile(manifest, plan); err != nil {
		return "", nil, fmt.Errorf("failed to parse manifest file at %s: %w", manifest, err)
	}

	return path, plan, nil
}

// resolveSDK resolves the root directory of an SDK.
func resolveSDK(cfg *config.EnvConfig, path string) (string, error) {
	baseDir := cfg.Dirs().SDKs()

	var try []string

	if filepath.IsAbs(path) {
		// the user supplied an absolute path.
		try = []string{path}
	} else {
		// the user supplied something that wasn't an absolute path.
		// we interpret it as a relative path to PWD first, to $TESTGROUND_HOME/sdks second.
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("could not determine current working dir: %w", err)
		}

		try = append(try, filepath.Join(wd, path), filepath.Join(baseDir, path))
	}

	for _, d := range try {
		if isDirectory(d) {
			return d, nil
		}
	}

	return "", fmt.Errorf("no matching paths; tried: %v", try)
}

func isDirectory(path string) bool {
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return false
	}
	return true
}
