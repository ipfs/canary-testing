//+build linux

package sidecar

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/hashicorp/go-multierror"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"

	"github.com/testground/testground/pkg/docker"
	"github.com/testground/testground/pkg/logging"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

// PublicAddr points to an IP address in the public range. It helps us discover
// the IP address of the gateway (i.e. the Docker host) on the control network
// (the learned route will be via the control network because, at this point,
// the only network that's attached to the container is the control network).
//
// Sidecar doesn't whitelist traffic to public addresses, but it special-cases
// traffic between the container and the host, so that pprof, metrics and other
// ports can be exposed to the Docker host.
var PublicAddr = net.ParseIP("1.1.1.1")

type DockerReactor struct {
	client  *sync.Client
	routes  []net.IP
	manager *docker.Manager
}

func NewDockerReactor() (Reactor, error) {
	// TODO: Generalize this to a list of services.
	wantedRoutes := []string{
		os.Getenv(EnvRedisHost),
	}

	var resolvedRoutes []net.IP
	for _, route := range wantedRoutes {
		ip, err := net.ResolveIPAddr("ip4", route)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve host %s: %w", route, err)
		}
		resolvedRoutes = append(resolvedRoutes, ip.IP)
	}

	docker, err := docker.NewManager()
	if err != nil {
		return nil, err
	}

	client, err := sync.NewGenericClient(context.Background(), logging.S())
	if err != nil {
		return nil, err
	}

	// sidecar nodes perform Redis GC.
	client.EnableBackgroundGC(nil)

	return &DockerReactor{
		client:  client,
		routes:  resolvedRoutes,
		manager: docker,
	}, nil
}

func (d *DockerReactor) Handle(globalctx context.Context, handler InstanceHandler) error {
	return d.manager.Watch(globalctx, func(ctx context.Context, container *docker.ContainerRef) error {
		logging.S().Debugw("got container", "container", container.ID)
		inst, err := d.handleContainer(ctx, container)
		if err != nil {
			return fmt.Errorf("failed to initialise the container: %w", err)
		}
		if inst == nil {
			logging.S().Debugw("ignoring container", "container", container.ID)
			return nil
		}

		err = handler(ctx, inst)
		if err != nil {
			return fmt.Errorf("container worker failed: %w", err)
		}
		return nil
	}, "testground.run_id")
}

func (d *DockerReactor) Close() error {
	var err *multierror.Error
	err = multierror.Append(err, d.manager.Close())
	err = multierror.Append(err, d.client.Close())
	return err.ErrorOrNil()
}

func (d *DockerReactor) handleContainer(ctx context.Context, container *docker.ContainerRef) (inst *Instance, err error) {
	// Get the state/config of the cluster
	info, err := container.Inspect(ctx)
	if err != nil {
		return nil, fmt.Errorf("inspect failed: %w", err)
	}

	if !info.State.Running {
		return nil, fmt.Errorf("not running")
	}

	// Construct the runtime environment
	params, err := runtime.ParseRunParams(info.Config.Env)
	if err != nil {
		return nil, fmt.Errorf("failed to parse run environment: %w", err)
	}

	// Not using the sidecar, ignore this container.
	if !params.TestSidecar {
		return nil, nil
	}

	// Remove the TestOutputsPath. We can't store anything from the sidecar.
	params.TestOutputsPath = ""
	runenv := runtime.NewRunEnv(*params)

	//////////////////
	//  NETWORKING  //
	//////////////////

	// TODO: cache this?
	networks, err := container.Manager.NetworkList(ctx, types.NetworkListOptions{
		Filters: filters.NewArgs(
			filters.Arg(
				"label",
				"testground.run_id="+info.Config.Labels["testground.run_id"],
			),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	// Get a netlink handle.
	nshandle, err := netns.GetFromPid(info.State.Pid)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup the net namespace: %s", err)
	}
	defer nshandle.Close()

	netlinkHandle, err := netlink.NewHandleAt(nshandle)
	if err != nil {
		return nil, fmt.Errorf("failed to get handle to network namespace: %w", err)
	}

	defer func() {
		if err != nil {
			netlinkHandle.Delete()
		}
	}()

	// Map _current_ networks to links.
	links, err := dockerLinks(netlinkHandle, info.NetworkSettings)
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate links: %w", err)
	}

	// Finally, construct the network manager.
	network := &DockerNetwork{
		container:      container,
		activeLinks:    make(map[string]*dockerLink, len(info.NetworkSettings.Networks)),
		availableLinks: make(map[string]string, len(networks)),
		nl:             netlinkHandle,
	}

	for _, n := range networks {
		name := n.Labels["testground.name"]
		id := n.ID
		network.availableLinks[name] = id
	}

	reverseIndex := make(map[string]string, len(network.availableLinks))
	for name, id := range network.availableLinks {
		reverseIndex[id] = name
	}

	// TODO: Some of this code could be factored out into helpers.

	var controlRoutes []netlink.Route
	for _, route := range d.routes {
		nlroutes, err := netlinkHandle.RouteGet(route)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve route: %w", err)
		}
		controlRoutes = append(controlRoutes, nlroutes...)
	}

	// Get the route to a public address. We will NOT be whitelisting traffic to
	// public IPs, but this helps us discover the IP address of the Docker host
	// on the control network. See the godoc on the PublicAddr var for more
	// info.
	pub, err := netlinkHandle.RouteGet(PublicAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve route for %s: %w", PublicAddr, err)
	}

	switch {
	case len(pub) == 0:
		logging.S().Warnw("failed to discover gateway/host address; no routes to public IPs", "container_id", container.ID)
	case pub[0].Gw == nil:
		logging.S().Warnw("failed to discover gateway/host address; gateway is nil", "route", pub[0], "container_id", container.ID)
	default:
		hostRoutes, err := netlinkHandle.RouteGet(pub[0].Gw)
		if err != nil {
			logging.S().Warnw("failed to add route for gateway/host address", "error", err, "route", pub[0], "container_id", container.ID)
			break
		}
		logging.S().Infow("successfully resolved route to host", "container_id", container.ID)
		controlRoutes = append(controlRoutes, hostRoutes...)
	}

	for id, link := range links {
		if name, ok := reverseIndex[id]; ok {
			// manage this network
			handle, err := NewNetlinkLink(netlinkHandle, link.Link)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to initialize link %s (%s): %w",
					name,
					link.Attrs().Name,
					err,
				)
			}
			network.activeLinks[name] = &dockerLink{
				NetlinkLink: handle,
				IPv4:        link.IPv4,
				IPv6:        link.IPv6,
			}
			continue
		}

		// We've found a control network (or some other network).

		// Get the current routes.
		linkRoutes, err := netlinkHandle.RouteList(link, netlink.FAMILY_ALL)
		if err != nil {
			return nil, fmt.Errorf("failed to list routes for link %s", link.Attrs().Name)
		}

		// Add learned routes plan containers so they can reach  the testground infra on the control network.
		for _, route := range controlRoutes {
			if route.LinkIndex != link.Attrs().Index {
				continue
			}
			if err := netlinkHandle.RouteAdd(&route); err != nil {
				return nil, fmt.Errorf("failed to add new route: %w", err)
			}
		}

		// Remove the original routes
		for _, route := range linkRoutes {
			if err := netlinkHandle.RouteDel(&route); err != nil {
				return nil, fmt.Errorf("failed to remove existing route: %w", err)
			}
		}
	}
	return NewInstance(d.client, runenv, info.Config.Hostname, network)
}
