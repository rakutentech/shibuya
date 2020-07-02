package config

import (
	"fmt"
	"log"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/flowcontrol"
	metricsc "k8s.io/metrics/pkg/client/clientset/versioned"
)

func newRateLimiter() flowcontrol.RateLimiter {
	return flowcontrol.NewTokenBucketRateLimiter(200.0, 200)
}

// getConfig returns a Kubernetes client config for a given context.
func getConfig() clientcmd.ClientConfig {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
}

// configForContext creates a Kubernetes REST client configuration for a given kubeconfig context.
func configForContext() (*rest.Config, error) {
	var config *rest.Config
	var err error
	if SC.ExecutorConfig.InCluster {
		log.Print("Using in cluster config")
		config, err = rest.InClusterConfig()
	} else {
		log.Print("Using out of cluster config")
		config, err = getConfig().ClientConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("could not get Kubernetes config- %s", err)
	}
	config.RateLimiter = newRateLimiter()
	return config, nil
}

// GetKubeClient creates a Kubernetes config and client for a given kubeconfig context.
func GetKubeClient() (*kubernetes.Clientset, error) {
	config, err := configForContext()
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("could not get Kubernetes client: %s", err)
	}
	return client, nil
}

func GetMetricsClient() (*metricsc.Clientset, error) {
	config, err := configForContext()
	if err != nil {
		return nil, err
	}
	clientset, err := metricsc.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}
