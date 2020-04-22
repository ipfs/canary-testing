package runner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/testground/testground/pkg/api"
	"github.com/testground/testground/pkg/conv"
	hc "github.com/testground/testground/pkg/healthcheck"
	"github.com/testground/testground/pkg/logging"
	"github.com/testground/testground/pkg/rpc"
	"github.com/testground/sdk-go/runtime"

	v1 "k8s.io/api/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/kubernetes/client-go/tools/remotecommand"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	_    api.Runner        = (*ClusterK8sRunner)(nil)
	_    api.Terminatable  = (*ClusterK8sRunner)(nil)
	_    api.Healthchecker = (*ClusterK8sRunner)(nil)
	once                   = sync.Once{}
)

const (
	defaultK8sNetworkAnnotation = "flannel"
	// collect-outputs pod is used to compress outputs at the end of a testplan run
	// as well as to copy archives from it, since it has EFS attached to it
	collectOutputsPodName = "collect-outputs"

	// number of CPUs allocated to each Sidecar. should be same as what is set in sidecar.yaml
	sidecarCPUs = 0.2

	// utilisation is how many CPUs from the remainder shall we allocate to Testground
	// note that there are other services running on the Kubernetes cluster such as
	// api proxy, node_exporter, dummy, etc.
	utilisation = 0.85

	// magic values that we monitor on the Testground runner side to detect when Testground
	// testplan instances are initialised and at the stage of actually running a test
	// check sdk/sync for more information
	NetworkInitialisationSuccessful = "network initialisation successful"
	NetworkInitialisationFailed     = "network initialisation failed"
)

var (
	testplanSysctls = []v1.Sysctl{{Name: "net.core.somaxconn", Value: "10000"}}

	// resource requests and limits for the `collect-outputs` pod
	collectOutputsResourceCPU    = resource.MustParse("2000m")
	collectOutputsResourceMemory = resource.MustParse("1024Mi")
)

var k8sSubnetIdx uint64 = 0

func init() {
	// Avoid collisions in picking up subnets
	rand.Seed(time.Now().UnixNano())
	k8sSubnetIdx = rand.Uint64() % 4096
}

func nextK8sSubnet() (*net.IPNet, error) {
	subnet, _, err := nextDataNetwork(int(atomic.AddUint64(&k8sSubnetIdx, 1) % 4096))
	if err != nil {
		return nil, err
	}
	return subnet, err
}

func homeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

// ClusterK8sRunnerConfig is the configuration object of this runner. Boolean
// values are expressed in a way that zero value (false) is the default setting.
type ClusterK8sRunnerConfig struct {
	// LogLevel sets the log level in the test containers (default: not set).
	LogLevel string `toml:"log_level"`

	KeepService bool `toml:"keep_service"`

	// Resources requested for each pod from the Kubernetes cluster
	PodResourceMemory string `toml:"pod_resource_memory"`
	PodResourceCPU    string `toml:"pod_resource_cpu"`
}

// ClusterK8sRunner is a runner that creates a Docker service to launch as
// many replicated instances of a container as the run job indicates.
type ClusterK8sRunner struct {
	config KubernetesConfig
	pool   *pool
}

type KubernetesConfig struct {
	// KubeConfigPath is the path to your kubernetes configuration path
	KubeConfigPath string `json:"kubeConfigPath"`
	// Namespace is the kubernetes namespaces where the pods should be running
	Namespace string `json:"namespace"`
}

// defaultKubernetesConfig uses the default ~/.kube/config
// to discover the kubernetes clusters. It also uses the "default" namespace.
func defaultKubernetesConfig() KubernetesConfig {
	kubeconfig := filepath.Join(homeDir(), ".kube", "config")
	if _, err := os.Stat(kubeconfig); os.IsNotExist(err) {
		kubeconfig = ""
	}
	return KubernetesConfig{
		KubeConfigPath: kubeconfig,
		Namespace:      "default",
	}
}

func (c *ClusterK8sRunner) Run(ctx context.Context, input *api.RunInput, ow *rpc.OutputWriter) (*api.RunOutput, error) {
	c.initPool()

	ow = ow.With("runner", "cluster:k8s", "run_id", input.RunID)

	cfg := *input.RunnerConfig.(*ClusterK8sRunnerConfig)

	podResourceCPU := resource.MustParse(cfg.PodResourceCPU)
	podResourceMemory := resource.MustParse(cfg.PodResourceMemory)

	template := runtime.RunParams{
		TestPlan:          input.TestPlan,
		TestCase:          input.TestCase,
		TestRun:           input.RunID,
		TestInstanceCount: input.TotalInstances,
		TestSidecar:       true,
		TestOutputsPath:   "/outputs",
		TestStartTime:     time.Now(),
	}

	// currently weave is not releaasing IP addresses upon container deletion - we get errors back when trying to
	// use an already used IP address, even if the container has been removed
	// this functionality should be refactored asap, when we understand how weave releases IPs (or why it doesn't release
	// them when a container is removed/ and as soon as we decide how to manage `networks in-use` so that there are no
	// collisions in concurrent testplan runs
	subnet, err := nextK8sSubnet()
	if err != nil {
		return nil, err
	}

	template.TestSubnet = &runtime.IPNet{IPNet: *subnet}

	enoughResources, err := c.checkClusterResources(ow, input.Groups, podResourceMemory, podResourceCPU)
	if err != nil {
		return nil, fmt.Errorf("couldn't check cluster resources: %v", err)
	}

	if !enoughResources {
		return nil, errors.New("too many test instances requested, resize cluster if you need more capacity")
	}

	jobName := fmt.Sprintf("tg-%s", input.TestPlan)

	ow.Infow("deploying testground testplan run on k8s", "job-name", jobName)

	var eg errgroup.Group

	// atomic counter which records how many networks have been initialised.
	// it should equal the number of all testplan instances for the given run eventually.
	var initialisedNetworks uint64

	eg.Go(func() error {
		return c.monitorTestplanRunState(ctx, ow, input, &initialisedNetworks)
	})

	sem := make(chan struct{}, 30) // limit the number of concurrent k8s api calls

	for _, g := range input.Groups {
		runenv := template
		runenv.TestGroupID = g.ID
		runenv.TestGroupInstanceCount = g.Instances
		runenv.TestInstanceParams = g.Parameters

		env := conv.ToEnvVar(runenv.ToEnvVars())
		env = append(env, v1.EnvVar{
			Name:  "REDIS_HOST",
			Value: "testground-infra-redis-headless",
		})

		// Set the log level if provided in cfg.
		if cfg.LogLevel != "" {
			env = append(env, v1.EnvVar{
				Name:  "LOG_LEVEL",
				Value: cfg.LogLevel,
			})
		}

		env = append(env, v1.EnvVar{
			Name: "POD_IP",
			ValueFrom: &v1.EnvVarSource{
				FieldRef: &v1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				}},
		})

		for i := 0; i < g.Instances; i++ {
			i := i
			g := g
			sem <- struct{}{}

			podName := fmt.Sprintf("%s-%s-%s-%d", jobName, input.RunID, g.ID, i)

			defer func() {
				if cfg.KeepService {
					return
				}
				client := c.pool.Acquire()
				defer c.pool.Release(client)
				err = client.CoreV1().Pods(c.config.Namespace).Delete(podName, &metav1.DeleteOptions{})
				if err != nil {
					ow.Errorw("couldn't remove pod", "pod", podName, "err", err)
				}
			}()

			eg.Go(func() error {
				defer func() { <-sem }()

				currentEnv := make([]v1.EnvVar, len(env))
				copy(currentEnv, env)

				currentEnv = append(currentEnv, v1.EnvVar{
					Name:  "TEST_OUTPUTS_PATH",
					Value: fmt.Sprintf("/outputs/%s/%s/%d", input.RunID, g.ID, i),
				})

				return c.createTestplanPod(ctx, podName, input, runenv, currentEnv, g, i, podResourceMemory, podResourceCPU)
			})
		}
	}

	err = eg.Wait()
	if err != nil {
		return nil, err
	}

	if input.TotalInstances <= 500 {
		var gg errgroup.Group

		for _, g := range input.Groups {
			for i := 0; i < g.Instances; i++ {
				i := i
				g := g
				sem <- struct{}{}

				gg.Go(func() error {
					defer func() { <-sem }()

					podName := fmt.Sprintf("%s-%s-%s-%d", jobName, input.RunID, g.ID, i)

					logs, err := c.getPodLogs(ow, podName)
					if err != nil {
						return err
					}

					fmt.Print(logs)
					return nil
				})
			}
		}

		err = gg.Wait()
		if err != nil {
			return nil, err
		}
	}

	return &api.RunOutput{RunID: input.RunID}, nil
}

func (*ClusterK8sRunner) ID() string {
	return "cluster:k8s"
}

func (c *ClusterK8sRunner) Healthcheck(ctx context.Context, engine api.Engine, ow *rpc.OutputWriter, fix bool) (*api.HealthcheckReport, error) {

	c.initPool()
	client := c.pool.Acquire()

	// How many plan worker nodes are there?
	res, err := client.CoreV1().Nodes().List(metav1.ListOptions{
		LabelSelector: "testground.nodetype=plan",
	})
	if err != nil {
		return nil, err
	}
	planNodes := res.Items

	hh := &hc.Helper{}

	hh.Enlist("kops validate",
		hc.CheckCommandStatus(ctx, "kops", "validate", "cluster"),
		hc.NotImplemented(),
	)

	hh.Enlist("efs pod",
		hc.CheckK8sPods(ctx, client, "app=efs-provisioner", c.config.Namespace, 1),
		hc.NotImplemented(),
	)

	hh.Enlist("redis pod",
		hc.CheckK8sPods(ctx, client, "app=redis", c.config.Namespace, 1),
		hc.NotImplemented(),
	)

	hh.Enlist("prometheus pod",
		hc.CheckK8sPods(ctx, client, "app=prometheus", c.config.Namespace, 1),
		hc.NotImplemented(),
	)

	hh.Enlist("grafana pod",
		hc.CheckK8sPods(ctx, client, "app.kubernetes.io/name=grafana", c.config.Namespace, 1),
		hc.NotImplemented(),
	)

	hh.Enlist("sidecar pods",
		hc.CheckK8sPods(ctx, client, "name=testground-sidecar", c.config.Namespace, len(planNodes)),
		hc.NotImplemented(),
	)

	return hh.RunChecks(ctx, fix)

}

func (*ClusterK8sRunner) ConfigType() reflect.Type {
	return reflect.TypeOf(ClusterK8sRunnerConfig{})
}

func (*ClusterK8sRunner) CompatibleBuilders() []string {
	return []string{"docker:go"}
}

func (c *ClusterK8sRunner) initPool() {
	once.Do(func() {
		log := logging.S().With("runner", "cluster:k8s")

		c.config = defaultKubernetesConfig()

		var err error
		workers := 20
		c.pool, err = newPool(workers, c.config)
		if err != nil {
			log.Fatal(err)
		}
	})
}

func (c *ClusterK8sRunner) CollectOutputs(ctx context.Context, input *api.CollectionInput, ow *rpc.OutputWriter) error {
	c.initPool()

	log := ow.With("runner", "cluster:k8s", "run_id", input.RunID)
	err := c.ensureCollectOutputsPod(ctx)
	if err != nil {
		return err
	}

	client := c.pool.Acquire()
	defer c.pool.Release(client)

	// This is the same line found in client_pool.go...
	// I need the restCfg, for remotecommand.
	// TODO: Reorganize not to repeat ourselves.
	k8sCfg, err := clientcmd.BuildConfigFromFlags("", c.config.KubeConfigPath)
	if err != nil {
		return err
	}

	// This request is sent to the collect-outputs pod
	// tar, compress, and write to stdout.
	// stdout will remain connected so we can read it later.

	log.Info("collecting outputs")

	req := client.
		CoreV1().
		RESTClient().
		Post().
		Resource("pods").
		Name("collect-outputs").
		Namespace("default").
		SubResource("exec").
		Param("container", "collect-outputs").
		VersionedParams(&v1.PodExecOptions{
			Container: "collect-outputs",
			Command: []string{
				"tar",
				"-C",
				"/outputs",
				"-czf",
				"-",
				input.RunID,
			},
			Stdin:  false,
			Stderr: false,
			Stdout: true,
		}, scheme.ParameterCodec)

	log.Debug("sending command to remote server: ", req.URL())
	exec, err := remotecommand.NewSPDYExecutor(k8sCfg, "POST", req.URL())
	if err != nil {
		log.Warnf("failed to send remote collection command: %v", err)
		return err
	}

	// Connect stdout of the above command to the output file
	// Connect stderr to a buffer which we can read from to display any errors to the user.
	outbuf := bufio.NewWriter(ow.BinaryWriter())
	defer outbuf.Flush()
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: outbuf,
	})
	if err != nil {
		log.Warnf("failed to collect results from remote collection command: %v", err)
		return err
	}
	return nil
}

// waitForPod waits until a given pod reaches the desired `phase` or the context is canceled
func (c *ClusterK8sRunner) waitForPod(ctx context.Context, podName string, phase string) error {
	client := c.pool.Acquire()
	defer c.pool.Release(client)

	var p string

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if p == phase {
				return nil
			}
			res, err := client.CoreV1().Pods(c.config.Namespace).List(metav1.ListOptions{
				FieldSelector: "metadata.name=" + podName,
			})
			if err != nil {
				return err
			}
			if len(res.Items) != 1 {
				continue
			}

			pod := res.Items[0]
			p = string(pod.Status.Phase)

			time.Sleep(1 * time.Second)
		}
	}
}

// ensureCollectOutputsPod ensures that we have a collect-outputs pod running
func (c *ClusterK8sRunner) ensureCollectOutputsPod(ctx context.Context) error {
	client := c.pool.Acquire()
	defer c.pool.Release(client)

	res, err := client.CoreV1().Pods(c.config.Namespace).List(metav1.ListOptions{
		FieldSelector: "metadata.name=" + collectOutputsPodName,
	})
	if err != nil {
		return err
	}
	if len(res.Items) == 0 {
		err = c.createCollectOutputsPod(ctx)
		if err != nil {
			return err
		}
		err = c.waitForPod(ctx, collectOutputsPodName, "Running")
		if err != nil {
			return err
		}
	} else if len(res.Items) > 1 {
		return errors.New("unexpected number of pods for outputs collection")
	}

	return nil
}

func (c *ClusterK8sRunner) getPodLogs(ow *rpc.OutputWriter, podName string) (string, error) {
	client := c.pool.Acquire()
	defer c.pool.Release(client)

	podLogOpts := v1.PodLogOptions{
		TailLines: int64Ptr(2),
	}

	var podLogs io.ReadCloser
	var err error
	err = retry(5, 5*time.Second, func() error {
		req := client.CoreV1().Pods(c.config.Namespace).GetLogs(podName, &podLogOpts)
		podLogs, err = req.Stream()
		if err != nil {
			ow.Warnw("got error when trying to fetch pod logs", "err", err.Error())
		}
		return err
	})
	if err != nil {
		return "", fmt.Errorf("error in opening stream: %v", err)
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", fmt.Errorf("error in copy information from podLogs to buf: %v", err)
	}

	return buf.String(), nil
}

func (c *ClusterK8sRunner) waitNetworksInitialised(ctx context.Context, ow *rpc.OutputWriter, runID string, initialisedNetworks *uint64) error {
	client := c.pool.Acquire()
	res, err := client.CoreV1().Pods(c.config.Namespace).List(metav1.ListOptions{
		LabelSelector: fmt.Sprintf("testground.run_id=%s", runID),
	})
	c.pool.Release(client)
	if err != nil {
		return err
	}

	var eg errgroup.Group

	for _, pod := range res.Items {
		podName := pod.Name

		eg.Go(func() error {
			err := c.waitNetworkInitialised(ctx, ow, podName)
			if err != nil {
				return err
			}

			atomic.AddUint64(initialisedNetworks, 1)

			return nil
		})
	}

	return eg.Wait()
}

func (c *ClusterK8sRunner) waitNetworkInitialised(ctx context.Context, ow *rpc.OutputWriter, podName string) error {
	podLogOpts := v1.PodLogOptions{
		SinceSeconds: int64Ptr(1000),
		Follow:       true,
	}

	var podLogs io.ReadCloser
	var err error
	err = retry(5, 5*time.Second, func() error {
		client := c.pool.Acquire()
		req := client.CoreV1().Pods(c.config.Namespace).GetLogs(podName, &podLogOpts)
		c.pool.Release(client)
		podLogs, err = req.Stream()
		if err != nil {
			ow.Warnw("got error when trying to fetch pod logs", "err", err.Error())
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("error in opening stream: %v", err)
	}
	defer podLogs.Close()

	scanner := bufio.NewScanner(podLogs)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			line := scanner.Text()
			if strings.Contains(line, NetworkInitialisationSuccessful) {
				return nil
			}
		}
	}

	return errors.New("network initialisation successful log line not detected")
}

func (c *ClusterK8sRunner) monitorTestplanRunState(ctx context.Context, ow *rpc.OutputWriter, input *api.RunInput, initialisedNetworks *uint64) error {
	client := c.pool.Acquire()
	defer c.pool.Release(client)

	start := time.Now()
	allRunningStage := false
	allNetworksStage := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if time.Since(start) > 100*time.Minute {
			return errors.New("global timeout")
		}
		time.Sleep(2000 * time.Millisecond)

		countPodsByState := func(state string) int {
			fieldSelector := fmt.Sprintf("status.phase=%s", state)
			opts := metav1.ListOptions{
				LabelSelector: fmt.Sprintf("testground.run_id=%s", input.RunID),
				FieldSelector: fieldSelector,
			}
			res, err := client.CoreV1().Pods(c.config.Namespace).List(opts)
			if err != nil {
				ow.Warnw("k8s client pods list error", "err", err.Error())
				return -1
			}
			return len(res.Items)
		}

		counters := map[string]int{}
		var countersMu sync.Mutex
		states := []string{"Pending", "Running", "Succeeded", "Failed", "Unknown"}

		var wg sync.WaitGroup
		wg.Add(len(states))
		for _, state := range states {
			state := state
			go func() {
				defer wg.Done()

				n := countPodsByState(state)

				countersMu.Lock()
				counters[state] = n
				countersMu.Unlock()
			}()
		}
		wg.Wait()

		initNets := int(atomic.LoadUint64(initialisedNetworks))
		ow.Debugw("testplan pods state", "running_for", time.Since(start), "succeeded", counters["Succeeded"], "running", counters["Running"], "pending", counters["Pending"], "failed", counters["Failed"], "unknown", counters["Unknown"])

		if counters["Running"] == input.TotalInstances && !allRunningStage {
			allRunningStage = true
			ow.Infow("all testplan instances in `Running` state", "took", time.Since(start))

			go func() {
				_ = c.waitNetworksInitialised(ctx, ow, input.RunID, initialisedNetworks)
			}()
		}

		if initNets == input.TotalInstances && !allNetworksStage {
			allNetworksStage = true
			ow.Infow("all testplan instances networks initialised", "took", time.Since(start))
		}

		if counters["Succeeded"] == input.TotalInstances {
			ow.Infow("all testplan instances in `Succeeded` state", "took", time.Since(start))
			return nil
		}

		if (counters["Succeeded"] + counters["Failed"]) == input.TotalInstances {
			ow.Warnw("all testplan instances in `Succeeded` or `Failed` state", "took", time.Since(start))
			return nil
		}

	}
}

func (c *ClusterK8sRunner) createTestplanPod(ctx context.Context, podName string, input *api.RunInput, runenv runtime.RunParams, env []v1.EnvVar, g api.RunGroup, i int, podResourceMemory resource.Quantity, podResourceCPU resource.Quantity) error {
	client := c.pool.Acquire()
	defer c.pool.Release(client)

	mountPropagationMode := v1.MountPropagationHostToContainer
	sharedVolumeName := "efs-shared"

	podRequest := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
			Labels: map[string]string{
				"testground.plan":     input.TestPlan,
				"testground.testcase": runenv.TestCase,
				"testground.run_id":   input.RunID,
				"testground.groupid":  g.ID,
				"testground.purpose":  "plan",
			},
			Annotations: map[string]string{"cni": defaultK8sNetworkAnnotation},
		},
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: sharedVolumeName,
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "efs",
						},
					},
				},
			},
			SecurityContext: &v1.PodSecurityContext{
				Sysctls: testplanSysctls,
			},
			RestartPolicy: v1.RestartPolicyNever,
			InitContainers: []v1.Container{
				{
					Name:    "mkdir-outputs",
					Image:   "busybox",
					Args:    []string{"-c", "mkdir -p $TEST_OUTPUTS_PATH"},
					Command: []string{"sh"},
					Env:     env,
					VolumeMounts: []v1.VolumeMount{
						{
							Name:             sharedVolumeName,
							MountPath:        "/outputs",
							MountPropagation: &mountPropagationMode,
						},
					},
				},
			},
			Containers: []v1.Container{
				{
					Name:  podName,
					Image: g.ArtifactPath,
					Args:  []string{},
					Env:   env,
					Ports: []v1.ContainerPort{
						{
							Name:          "pprof",
							ContainerPort: 6060,
						},
					},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:             sharedVolumeName,
							MountPath:        "/outputs",
							MountPropagation: &mountPropagationMode,
						},
					},
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceMemory: podResourceMemory,
							v1.ResourceCPU:    podResourceCPU,
						},
					},
				},
			},
			NodeSelector: map[string]string{"testground.nodetype": "plan"},
		},
	}

	_, err := client.CoreV1().Pods(c.config.Namespace).Create(podRequest)
	return err
}

func int64Ptr(i int64) *int64 { return &i }

type FakeWriterAt struct {
	w io.Writer
}

func (fw FakeWriterAt) WriteAt(p []byte, offset int64) (n int, err error) {
	// ignore 'offset' because we forced sequential downloads
	return fw.w.Write(p)
}

// checkClusterResources returns whether we can fit the input groups in the current cluster
func (c *ClusterK8sRunner) checkClusterResources(ow *rpc.OutputWriter, groups []api.RunGroup, fallbackMemory resource.Quantity, fallbackCPU resource.Quantity) (bool, error) {
	neededCPUs := 0.0

	defaultPodCPU, err := strconv.ParseFloat(fallbackCPU.AsDec().String(), 64)
	if err != nil {
		return false, err
	}

	client := c.pool.Acquire()
	defer c.pool.Release(client)

	res, err := client.CoreV1().Nodes().List(metav1.ListOptions{
		LabelSelector: "testground.nodetype=plan",
	})
	if err != nil {
		return false, err
	}

	nodes := len(res.Items)

	// all worker nodes are the same, so just take allocatable CPU from the first
	item := res.Items[0].Status.Allocatable["cpu"]
	nodeCPUs, _ := item.AsInt64()

	totalCPUs := nodes * int(nodeCPUs)
	availableCPUs := float64(totalCPUs) - float64(nodes)*sidecarCPUs

	for _, g := range groups {
		var podCPU float64
		if g.Resources.CPU != "" {
			cpu, err := resource.ParseQuantity(g.Resources.CPU)
			if err != nil {
				return false, err
			}
			podCPU, err = strconv.ParseFloat(cpu.AsDec().String(), 64)
			if err != nil {
				return false, err
			}
		} else {
			podCPU = defaultPodCPU
		}

		neededCPUs += podCPU * float64(g.Instances)
	}

	if (availableCPUs * utilisation) > neededCPUs {
		return true, nil
	}

	ow.Warnw("not enough resources on cluster", "available_cpus", availableCPUs, "needed_cpus", neededCPUs, "utilisation", utilisation)
	return false, nil
}

// Terminates all pods for with the label testground.purpose: plan
// This command will remove all plan pods in the cluster.
func (c *ClusterK8sRunner) TerminateAll(_ context.Context, ow *rpc.OutputWriter) error {
	c.initPool()

	client := c.pool.Acquire()
	defer c.pool.Release(client)

	planPods := metav1.ListOptions{
		LabelSelector: "testground.purpose=plan",
	}
	err := client.CoreV1().Pods("default").DeleteCollection(&metav1.DeleteOptions{}, planPods)
	if err != nil {
		ow.Errorw("could not terminate all pods", "err", err)
		return err
	}
	return nil
}

func (c *ClusterK8sRunner) createCollectOutputsPod(ctx context.Context) error {
	client := c.pool.Acquire()
	defer c.pool.Release(client)

	mountPropagationMode := v1.MountPropagationHostToContainer
	sharedVolumeName := "efs-shared"

	podRequest := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: collectOutputsPodName,
			Labels: map[string]string{
				"testground.purpose": "outputs",
			},
			Annotations: map[string]string{"cni": defaultK8sNetworkAnnotation},
		},
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: sharedVolumeName,
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "efs",
						},
					},
				},
			},
			SecurityContext: &v1.PodSecurityContext{
				Sysctls: testplanSysctls,
			},
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{
				{
					Name:    "collect-outputs",
					Image:   "busybox",
					Args:    []string{"-c", "sleep 999999999"},
					Command: []string{"sh"},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:             sharedVolumeName,
							MountPath:        "/outputs",
							MountPropagation: &mountPropagationMode,
						},
					},
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceCPU:    collectOutputsResourceCPU,
							v1.ResourceMemory: collectOutputsResourceMemory,
						},
					},
				},
			},
		},
	}

	_, err := client.CoreV1().Pods(c.config.Namespace).Create(podRequest)
	return err
}
