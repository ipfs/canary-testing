//+build linux

package sidecar

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/containernetworking/cni/libcni"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"

	"github.com/ipfs/testground/pkg/dockermanager"
	"github.com/ipfs/testground/pkg/logging"
	"github.com/ipfs/testground/sdk/runtime"
	"github.com/ipfs/testground/sdk/sync"
)

type K8sInstanceManager struct {
	redis   net.IP
	manager *dockermanager.Manager
}

func NewK8sManager() (InstanceManager, error) {
	redisHost := os.Getenv(EnvRedisHost)

	redisIp, err := net.ResolveIPAddr("ip4", redisHost)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve redis: %w", err)
	}

	docker, err := dockermanager.NewManager()
	if err != nil {
		return nil, err
	}

	return &K8sInstanceManager{
		manager: docker,
		redis:   redisIp.IP,
	}, nil
}

func (d *K8sInstanceManager) Manage(
	ctx context.Context,
	worker func(ctx context.Context, inst *Instance) error,
) error {
	return d.manager.Manage(ctx, func(ctx context.Context, container *dockermanager.Container) error {
		inst, err := d.manageContainer(ctx, container)
		if err != nil {
			return fmt.Errorf("failed to initialise the container: %w", err)
		}
		if inst == nil {
			return nil
		}
		err = worker(ctx, inst)
		if err != nil {
			return fmt.Errorf("container worker failed: %w", err)
		}
		return nil
	})
}

func (d *K8sInstanceManager) Close() error {
	return d.manager.Close()
}

func (d *K8sInstanceManager) manageContainer(ctx context.Context, container *dockermanager.Container) (inst *Instance, err error) {
	// Get the state/config of the cluster
	info, err := container.Inspect(ctx)
	if err != nil {
		return nil, fmt.Errorf("inspect failed: %w", err)
	}

	if !info.State.Running {
		return nil, fmt.Errorf("not running")
	}

	// Construct the runtime environment
	runenv, err := runtime.ParseRunEnv(info.Config.Env)
	if err != nil {
		return nil, fmt.Errorf("failed to parse run environment: %w", err)
	}

	if runenv.TestSidecar == false {
		return nil, nil
	}

	// Initialise CNI config
	cninet := libcni.NewCNIConfig(filepath.SplitList("/host/opt/cni/bin"), nil)

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

	// Finally, construct the network manager.
	network := &K8sNetwork{
		netnsPath:   fmt.Sprintf("/proc/%d/ns/net", info.State.Pid),
		cninet:      cninet,
		container:   container,
		subnet:      runenv.TestSubnet.String(),
		nl:          netlinkHandle,
		activeLinks: make(map[string]*k8sLink),
	}

	return NewInstance(runenv, info.Config.Hostname, network)

	// TODO(anteva): remove all routes but redis and the data subnet

	//reverseIndex := make(map[string]string, len(network.availableLinks))
	//for name, id := range network.availableLinks {
	//reverseIndex[id] = name
	//}

	//// TODO: Some of this code could be factored out into helpers.

	//// Get the routes to redis. We need to keep these.
	//redisRoutes, err := netlinkHandle.RouteGet(d.redis)
	//if err != nil {
	//return nil, fmt.Errorf("failed to resolve route to redis: %w", err)
	//}

	//for id, link := range links {
	//if name, ok := reverseIndex[id]; ok {
	//// manage this network
	//handle, err := NewNetlinkLink(netlinkHandle, link.Link)
	//if err != nil {
	//return nil, fmt.Errorf(
	//"failed to initialize link %s (%s): %w",
	//name,
	//link.Attrs().Name,
	//err,
	//)
	//}
	//network.activeLinks[name] = &dockerLink{
	//NetlinkLink: handle,
	//IPv4:        link.IPv4,
	//IPv6:        link.IPv6,
	//}
	//continue
	//}

	//// We've found a control network (or some other network).

	//// Get the current routes.
	//linkRoutes, err := netlinkHandle.RouteList(link, netlink.FAMILY_ALL)
	//if err != nil {
	//return nil, fmt.Errorf("failed to list routes for link %s", link.Attrs().Name)
	//}

	//// Add specific routes to redis if redis uses this link.
	//for _, route := range redisRoutes {
	//if route.LinkIndex != link.Attrs().Index {
	//continue
	//}
	//if err := netlinkHandle.RouteAdd(&route); err != nil {
	//return nil, fmt.Errorf("failed to add new route: %w", err)
	//}
	//break
	//}

	//// Remove the original routes
	//for _, route := range linkRoutes {
	//if err := netlinkHandle.RouteDel(&route); err != nil {
	//return nil, fmt.Errorf("failed to remove existing route: %w", err)
	//}
	//}
	//}
}

type k8sLink struct {
	*NetlinkLink
	IPv4, IPv6 *net.IPNet

	rt      *libcni.RuntimeConf
	netconf *libcni.NetworkConfigList
}

type K8sNetwork struct {
	container   *dockermanager.Container
	activeLinks map[string]*k8sLink
	nl          *netlink.Handle
	cninet      *libcni.CNIConfig
	subnet      string
	netnsPath   string
}

func (n *K8sNetwork) Close() error {
	return nil
}

func (n *K8sNetwork) ConfigureNetwork(ctx context.Context, cfg *sync.NetworkConfig) error {
	if cfg.Network != "default" {
		return errors.New("configured network is not default")
	}

	link, online := n.activeLinks[cfg.Network]

	// Are we _disabling_ the network?
	if !cfg.Enable {
		// Yes, is it already disabled?
		if online {
			// No. Disconnect.
			if err := n.cninet.DelNetworkList(ctx, link.netconf, link.rt); err != nil {
				return fmt.Errorf("when 6: %w", err)
			}
			delete(n.activeLinks, cfg.Network)
		}
		return nil
	}

	if online && ((cfg.IPv6 != nil && !link.IPv6.IP.Equal(cfg.IPv6.IP)) ||
		(cfg.IPv4 != nil && !link.IPv4.IP.Equal(cfg.IPv4.IP))) {
		// Disconnect and reconnect to change the IP addresses.
		logging.S().Debug("disconnect and reconnect to change the IP addr", "cfg.IPv4", cfg.IPv4, "link.IPv4", link.IPv4.String(), "container", n.container.ID)
		//
		// NOTE: We probably don't need to do this on local docker.
		// However, we probably do with swarm.
		online = false
		if err := n.cninet.DelNetworkList(ctx, link.netconf, link.rt); err != nil {
			return fmt.Errorf("when 5: %w", err)
		}
		delete(n.activeLinks, cfg.Network)
	}

	// Are we _connected_ to the network.
	if !online {
		// No, we're not.
		// Connect.
		if cfg.IPv6 != nil {
			return errors.New("ipv6 not supported")
		}

		var (
			netconf *libcni.NetworkConfigList
			err     error
		)
		if cfg.IPv4 == nil {
			netconf, err = newNetworkConfigList("net", n.subnet)
		} else {
			netconf, err = newNetworkConfigList("ip", cfg.IPv4.String())
		}
		if err != nil {
			return fmt.Errorf("failed to generate new network config list: %w", err)
		}

		cniArgs := [][2]string{}                   // empty
		capabilityArgs := map[string]interface{}{} // empty
		ifName := "eth1"

		rt := &libcni.RuntimeConf{
			ContainerID:    n.container.ID,
			NetNS:          n.netnsPath,
			IfName:         ifName,
			Args:           cniArgs,
			CapabilityArgs: capabilityArgs,
		}

		_, err = n.cninet.AddNetworkList(ctx, netconf, rt)
		if err != nil {
			return fmt.Errorf("failed to add network through cni plugin: %w", err)
		}

		netlinkByName, err := n.nl.LinkByName(ifName)
		if err != nil {
			return fmt.Errorf("failed to get link by name: %w", err)
		}

		// Register an active link.
		handle, err := NewNetlinkLink(n.nl, netlinkByName)
		if err != nil {
			return fmt.Errorf("failed to register new netlink: %w", err)
		}
		v4addrs, err := handle.ListV4()
		if err != nil {
			return fmt.Errorf("failed to list v4 addrs: %w", err)
		}
		if len(v4addrs) != 1 {
			return fmt.Errorf("expected 1 v4addrs, but received %d", len(v4addrs))
		}

		link = &k8sLink{
			NetlinkLink: handle,
			IPv4:        v4addrs[0],
			IPv6:        nil,
			rt:          rt,
			netconf:     netconf,
		}

		logging.S().Debugw("successfully adding an active link", "ipv4", link.IPv4, "container", n.container.ID)

		n.activeLinks[cfg.Network] = link
	}

	// We don't yet support applying per-subnet rules.
	if len(cfg.Rules) != 0 {
		return fmt.Errorf("TODO: per-subnet bandwidth rules not supported")
	}

	if err := link.Shape(cfg.Default); err != nil {
		return fmt.Errorf("failed to shape link: %w", err)
	}
	return nil
}

func (n *K8sNetwork) ListActive() []string {
	panic("not implemented yet")
	return []string{}
}

func (n *K8sNetwork) ListAvailable() []string {
	panic("not implemented yet")
	return []string{}
}

//func attach(addr string, netns string, containerID string, ifName string) error {
//// <addr>     = [ip:]<cidr> | net:<cidr> | net:default

//// addr := "net:10.36.79.0/24"
//// ip := "10.36.79.10/24"
//// netns := "" // abs path
//// containerID := ""
//// ifName := "net1"

//// we assume that this is mapped as a volume
//cniBinPath := filepath.SplitList("/host/opt/cni/bin")

//bytes := []byte(`
//{
//"cniVersion": "0.3.0",
//"name": "weave",
//"plugins": [
//{
//"name": "weave",
//"type": "weave-net",
//"ipam": {
//"subnet": "` + addr + `"
//},
//"hairpinMode": true
//}
//]
//}
//`)

//netconf, err := libcni.ConfListFromBytes(bytes)
//if err != nil {
//return err
//}

//cniArgs := [][2]string{}                   // empty
//capabilityArgs := map[string]interface{}{} // empty

//cninet := libcni.NewCNIConfig(cniBinPath, nil)

//rt := &libcni.RuntimeConf{
//ContainerID:    containerID,
//NetNS:          netns,
//IfName:         ifName,
//Args:           cniArgs,
//CapabilityArgs: capabilityArgs,
//}

//_, err = cninet.AddNetworkList(context.TODO(), netconf, rt)
//if err != nil {
//return err
//}

//return nil
//}

func newNetworkConfigList(t string, addr string) (*libcni.NetworkConfigList, error) {
	if t == "net" {
		bytes := []byte(`
{
		"cniVersion": "0.3.0",
		"name": "weave",
		"plugins": [
				{
						"name": "weave",
						"type": "weave-net",
						"ipam": {
								"subnet": "` + addr + `"
						},
						"hairpinMode": true
				}
		]
}
`)
		return libcni.ConfListFromBytes(bytes)
	}

	if t == "ip" {
		bytes := []byte(`
{
		"cniVersion": "0.3.0",
		"name": "weave",
		"plugins": [
				{
						"name": "weave",
						"type": "weave-net",
						"ipam": {
								"ips": [
								  {
									  "version": "4",
										"address": "` + addr + `"
								  }
								]
						},
						"hairpinMode": true
				}
		]
}
`)
		return libcni.ConfListFromBytes(bytes)
	}

	return nil, errors.New("unknown type")
}
