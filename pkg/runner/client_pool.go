package runner

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
)

type pool struct {
	availableC chan *kubernetes.Clientset
}

// newPool returns a pool of Kubernetes clientset connections
func newPool(workers int, config KubernetesConfig) (*pool, error) {
	k8scfg, err := clientcmd.BuildConfigFromFlags("", config.KubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("could not start k8s client from config: %v", err)
	}

	pool := &pool{
		availableC: make(chan *kubernetes.Clientset, workers),
	}

	for i := 0; i < workers; i++ {
		k8sClientset, err := kubernetes.NewForConfig(k8scfg)
		if err != nil {
			return nil, fmt.Errorf("could not create k8s clientset: %v", err)
		}

		pool.availableC <- k8sClientset
	}

	return pool, nil
}

func (p *pool) Acquire() *kubernetes.Clientset {
	return <-p.availableC
}

func (p *pool) Release(cs *kubernetes.Clientset) {
	p.availableC <- cs
}
