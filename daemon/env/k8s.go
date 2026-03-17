package env

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func K8sGetNodeID() (string, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return "", fmt.Errorf("not running in a k8s cluster: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", fmt.Errorf("error creating k8s client: %w", err)
	}

	// Step 3: Get pod name and namespace from env vars
	podName := os.Getenv("POD_NAME") // Kubernetes sets this to the pod name by default
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		// If you didn’t set POD_NAMESPACE explicitly, use the default service account namespace
		data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			return "", fmt.Errorf("failed to read namespace file: %v", err)
		}
		namespace = string(data)
	}

	// Step 4: Get the Pod object
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %v", err)
	}

	nodeName := pod.Spec.NodeName

	// Step 5: Get the Node object
	node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get node: %v", err)
	}

	return string(node.UID), nil
}
