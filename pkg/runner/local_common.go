package runner

import (
	"context"

	"github.com/docker/go-units"

	"github.com/ipfs/testground/pkg/docker"
	"github.com/ipfs/testground/pkg/healthcheck"
	"github.com/ipfs/testground/pkg/rpc"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

func localCommonHealthcheck(ctx context.Context, hh *healthcheck.Helper, cli *client.Client, ow *rpc.OutputWriter, controlNetworkID string, workdir string) {
	hh.Enlist("local-outputs-dir",
		healthcheck.CheckDirectoryExists(workdir),
		healthcheck.CreateDirectory(workdir),
	)

	// testground-control network
	hh.Enlist("control-network",
		healthcheck.CheckNetwork(ctx, ow, cli, controlNetworkID),
		healthcheck.CreateNetwork(ctx, ow, cli, controlNetworkID, network.IPAMConfig{Subnet: controlSubnet, Gateway: controlGateway}),
	)

	// grafana from downloaded image, with no additional configuration.
	_, exposed, _ := nat.ParsePortSpecs([]string{"3000:3000"})
	hh.Enlist("local-grafana",
		healthcheck.CheckContainerStarted(ctx, ow, cli, "testground-grafana"),
		healthcheck.StartContainer(ctx, ow, cli, &docker.EnsureContainerOpts{
			ContainerName: "testground-grafana",
			ContainerConfig: &container.Config{
				Image: "bitnami/grafana",
			},
			HostConfig: &container.HostConfig{
				PortBindings: exposed,
				NetworkMode:  container.NetworkMode(controlNetworkID),
			},
			ImageStrategy: docker.ImageStrategyPull,
		}),
	)

	// redis, using a downloaded image and no additional configuration.
	_, exposed, _ = nat.ParsePortSpecs([]string{"6379:6379"})
	hh.Enlist("local-redis",
		healthcheck.CheckContainerStarted(ctx, ow, cli, "testground-redis"),
		healthcheck.StartContainer(ctx, ow, cli, &docker.EnsureContainerOpts{
			ContainerName: "testground-redis",
			ContainerConfig: &container.Config{
				Image: "library/redis",
				Cmd:   []string{"--save", "\"\"", "--appendonly", "no", "--maxclients", "120000"},
			},
			HostConfig: &container.HostConfig{
				PortBindings: exposed,
				NetworkMode:  container.NetworkMode(controlNetworkID),
				Resources: container.Resources{
					Ulimits: []*units.Ulimit{
						{Name: "nofile", Hard: InfraMaxFilesUlimit, Soft: InfraMaxFilesUlimit},
					},
				},
				Sysctls: map[string]string{
					"net.core.somaxconn":             "150000",
					"net.netfilter.nf_conntrack_max": "120000",
				},
				RestartPolicy: container.RestartPolicy{
					Name: "unless-stopped",
				},
			},
			ImageStrategy: docker.ImageStrategyPull,
		}),
	)

}
