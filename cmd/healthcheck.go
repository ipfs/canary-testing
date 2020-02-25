package cmd

import (
	"context"
	"fmt"

	"github.com/ipfs/testground/pkg/client"
	"github.com/urfave/cli"
)

var HealthcheckCommand = cli.Command{
	Name:   "healthcheck",
	Usage:  "checks, and optionally heals, the preconditions for the runner to be able to run properly",
	Action: healthcheckCommand,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "fix",
			Usage: "should try to fix the preconditions",
		},
		cli.StringFlag{
			Name:     "runner",
			Usage:    "specifies the runner to use; values include: 'local:exec', 'local:docker', 'cluster:k8s'",
			Required: true,
		},
	},
}

func healthcheckCommand(c *cli.Context) error {
	ctx, cancel := context.WithCancel(ProcessContext())
	defer cancel()

	var (
		runner = c.String("runner")
		fix    = c.Bool("fix")
	)

	api, err := setupClient(c)
	if err != nil {
		return err
	}

	r, err := api.Healthcheck(ctx, &client.HealthcheckRequest{
		Runner: runner,
		Fix:    fix,
	})
	if err != nil {
		return err
	}
	defer r.Close()

	resp, err := client.ParseHealthcheckResponse(r)
	if err != nil {
		return err
	}

	fmt.Printf("Finished healthchecking runner %s\n", runner)

	if len(resp.Checks) > 0 {
		fmt.Printf("Checks:\n")
		for _, check := range resp.Checks {
			fmt.Printf("- %s: %s; %s\n", check.Name, check.Status, check.Message)
		}
	} else {
		fmt.Println("No checks made.")
	}

	if len(resp.Fixes) > 0 {
		fmt.Printf("Fixes:\n")
		for _, fix := range resp.Fixes {
			fmt.Printf("- %s: %s; %s\n", fix.Name, fix.Status, fix.Message)
		}
	} else {
		fmt.Println("No fixes applied.")
	}

	return nil
}
