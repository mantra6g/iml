package env

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	infrav1alpha1 "iml-daemon/api/infra/v1alpha1"
	netutils "iml-daemon/pkg/utils/net"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	KubernetesAPICallTimeout = 20 * time.Second
	RetryTimes               = 5
	TotalRetryTimeout        = RetryTimes * KubernetesAPICallTimeout

	// Default path to read the configmap from
	DefaultIMLConfigMapPath = "/etc/iml/config"
)

const P4_CONTROLLER_ADDR = "iml-p4-controller.loom-system.svc.cluster.local"
const IML_ADDR = "iml-updates-service.loom-system.svc.cluster.local"
const API_PORT = "1810"
const MQTT_PORT = "1816"
const API_URL = "http://" + IML_ADDR + ":" + API_PORT
const MQTT_URL = "mqtt://" + IML_ADDR + ":" + MQTT_PORT
const P4_CONTROLLER_API_URL = "http://" + P4_CONTROLLER_ADDR

// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes/status,verbs=get;update;patch

type IMLConfigMap struct {
	ClusterCIDR netutils.DualStackNetwork
}

type GlobalConfig struct {
	IMLConfigMap
	PodCIDR  netutils.DualStackNetwork
	TunCIDR  netutils.DualStackNetwork
	DecapSID *net.IPNet
	NodeID   types.UID
	NodeName string
}

// Singleton instance of GlobalConfig
var globalConfig *GlobalConfig

func SetUpNode(k8sClient client.Client) (*GlobalConfig, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("error getting hostname: %w", err)
	}
	if hostname == "" {
		return nil, fmt.Errorf("hostname is empty")
	}
	configMap, err := readIMLConfigMap()
	if err != nil {
		return nil, fmt.Errorf("error reading configmap: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), KubernetesAPICallTimeout)
	defer cancel()

	loomNode := &infrav1alpha1.LoomNode{}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: hostname}, loomNode)
	// Discard every other error except NotFound
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("error getting loomNode: %w", err)
	}
	// If the error is NotFound, then we need to create the LoomNode resource
	if errors.IsNotFound(err) {
		loomNode, err = createLoomNode(ctx, k8sClient, hostname)
		if err != nil {
			return nil, fmt.Errorf("error creating loomNode: %w", err)
		}
	}
	// If the loomNode exists but no CIDRs were assigned yet, wait until they are assigned
	if len(loomNode.Spec.PodCIDRs) == 0 {
		err = waitForCIDRs(ctx, k8sClient, hostname)
		if err != nil {
			return nil, fmt.Errorf("error waiting for CIDRs to be created: %w", err)
		}
	}
	podCIDR, err := netutils.ParseDualStackNetworkFromStrings(loomNode.Spec.PodCIDRs)
	if err != nil {
		return nil, fmt.Errorf("error parsing podCIDRs: %w", err)
	}
	tunCIDR, err := netutils.ParseDualStackNetworkFromStrings(loomNode.Spec.TunnelCIDRs)
	if err != nil {
		return nil, fmt.Errorf("error parsing tunCIDRs: %w", err)
	}
	globalConfig = &GlobalConfig{
		IMLConfigMap: *configMap,
		PodCIDR:      podCIDR,
		TunCIDR:      tunCIDR,
		NodeID:       loomNode.UID,
		NodeName:     hostname,
	}
	return globalConfig, nil
}

func readIMLConfigMap() (*IMLConfigMap, error) {
	get := func(key string) (string, error) {
		data, err := os.ReadFile(filepath.Join(DefaultIMLConfigMapPath, key))
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}

	ipv4CidrStr, err := get("cluster-pool-ipv4-cidr")
	if err != nil {
		return nil, err
	}
	_, ipv4Cidr, err := net.ParseCIDR(ipv4CidrStr)
	if err != nil {
		return nil, err
	}

	ipv6CidrStr, err := get("cluster-pool-ipv6-cidr")
	if err != nil {
		return nil, err
	}
	_, ipv6Cidr, err := net.ParseCIDR(ipv6CidrStr)
	if err != nil {
		return nil, err
	}

	return &IMLConfigMap{
		ClusterCIDR: netutils.DualStackNetwork{
			IPv4Net: ipv4Cidr,
			IPv6Net: ipv6Cidr,
		},
	}, nil
}

func createLoomNode(ctx context.Context, k8sClient client.Client, nodeName string) (*infrav1alpha1.LoomNode, error) {
	loomNode := &infrav1alpha1.LoomNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
		Spec: infrav1alpha1.LoomNodeSpec{
			PodCIDRs:    make([]string, 0),
			TunnelCIDRs: make([]string, 0),
		},
	}
	err := k8sClient.Create(ctx, loomNode)
	if err != nil {
		return nil, err
	}
	return loomNode, nil
}

func waitForCIDRs(ctx context.Context, k8sClient client.Client, nodeName string) error {
	subCtx, cancel := context.WithTimeout(ctx, TotalRetryTimeout)
	defer cancel()
	return wait.PollUntilContextCancel(subCtx, KubernetesAPICallTimeout, true, func(ctx context.Context) (bool, error) {
		loomNode := &infrav1alpha1.LoomNode{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: nodeName}, loomNode)
		if errors.IsNotFound(err) {
			return false, err // Resource was deleted, stop retrying and return error
		}
		return true, nil // stop retrying
	})
}
