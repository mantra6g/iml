package k8s

import (
	"context"
	"fmt"
	"iml-controller/logger"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Client struct {
	kubeClient    *kubernetes.Clientset
	dynamicClient *dynamic.DynamicClient
}

func NewClient() (*Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &Client{
		kubeClient:    kubeClient,
		dynamicClient: dynamicClient,
	}, nil
}

func (c *Client) Example() {
	c.dynamicClient.Resource(schema.GroupVersionResource{
		Group:    "desire6g.eu",
		Version:  "v1",
		Resource: "apps",
	})
	c.dynamicClient.Resource(schema.GroupVersionResource{
		Group:    "desire6g.eu",
		Version:  "v1",
		Resource: "vnfs",
	})
	c.dynamicClient.Resource(schema.GroupVersionResource{
		Group:    "desire6g.eu",
		Version:  "v1",
		Resource: "chains",
	})
}

func (*Client) Shutdown(ctx context.Context) error {
	// Implement the shutdown logic for the Kubernetes client
	// This is a placeholder implementation
	logger.InfoLogger().Println("Shutting down K8sClient")
	return nil
}