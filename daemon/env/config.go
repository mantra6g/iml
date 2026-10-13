package env

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	netutils "github.com/mantra6g/iml/daemon/pkg/utils/net"

	infrav1alpha1 "github.com/mantra6g/iml/api/infra/v1alpha1"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Timeout of each individual call to the Kubernetes API.
	KubernetesAPICallTimeout = 20 * time.Second
	// How often to check whether the operator has assigned CIDRs to this node's LoomNode.
	CIDRPollInterval = 2 * time.Second

	// Default path to read the configmap from
	DefaultIMLConfigMapPath = "/etc/iml/config"
)

// CIDRWaitTimeout is how long to wait for the operator to assign CIDRs to this node's LoomNode.
// On a fresh install the operator may still be starting up, so this needs to be generous.
var CIDRWaitTimeout = 5 * time.Minute

// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.loom.io,resources=loomnodes/status,verbs=get;update;patch

type IMLConfigMap struct {
	ClusterCIDR netutils.DualStackNetwork
	ServiceCIDR netutils.DualStackNetwork
}

type GlobalConfig struct {
	IMLConfigMap
	PodCIDR    netutils.DualStackNetwork
	TunnelCIDR netutils.DualStackNetwork
	NodeID     types.UID
	NodeName   string
}

// Singleton instance of GlobalConfig
var globalConfig *GlobalConfig

func SetUpNode(logger logr.Logger, k8sClient client.Client) (*GlobalConfig, error) {
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
	loomNode, err := ensureLoomNodeCIDRs(context.Background(), logger, k8sClient, hostname)
	if err != nil {
		return nil, err
	}
	podCIDR, err := netutils.ParseDualStackNetworkFromStrings(loomNode.Spec.NodeCIDRs)
	if err != nil {
		return nil, fmt.Errorf("error parsing podCIDRs: %w", err)
	}
	tunnelCIDR, err := netutils.ParseDualStackNetworkFromStrings(loomNode.Spec.TunnelCIDRs)
	if err != nil {
		return nil, fmt.Errorf("error parsing tunnelCIDRs: %w", err)
	}
	if podCIDR.IPv6Net == nil || tunnelCIDR.IPv6Net == nil {
		return nil, fmt.Errorf("loomNode %s has no IPv6 node or tunnel CIDR: nodeCIDRs=%v tunnelCIDRs=%v",
			hostname, loomNode.Spec.NodeCIDRs, loomNode.Spec.TunnelCIDRs)
	}
	globalConfig = &GlobalConfig{
		IMLConfigMap: *configMap,
		PodCIDR:      podCIDR,
		TunnelCIDR:   tunnelCIDR,
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

	serviceCidrStr, err := get("service-cidr")
	if err != nil {
		return nil, err
	}
	_, serviceCidr, err := net.ParseCIDR(serviceCidrStr)
	if err != nil {
		return nil, err
	}
	var serviceCIDR netutils.DualStackNetwork
	if serviceCidr.IP.To4() != nil {
		serviceCIDR.IPv4Net = serviceCidr
	} else {
		serviceCIDR.IPv6Net = serviceCidr
	}

	return &IMLConfigMap{
		ClusterCIDR: netutils.DualStackNetwork{
			IPv4Net: ipv4Cidr,
			IPv6Net: ipv6Cidr,
		},
		ServiceCIDR: serviceCIDR,
	}, nil
}

// ensureLoomNodeCIDRs returns this node's LoomNode once the operator has assigned its CIDRs, creating the
// LoomNode if it does not exist yet. On a fresh install the operator (and its CRDs) may still be starting up,
// so lookup and creation errors are retried until CIDRWaitTimeout expires.
func ensureLoomNodeCIDRs(ctx context.Context, logger logr.Logger, k8sClient client.Client, nodeName string) (*infrav1alpha1.LoomNode, error) {
	var loomNode *infrav1alpha1.LoomNode
	waitingLogged := false
	err := wait.PollUntilContextTimeout(ctx, CIDRPollInterval, CIDRWaitTimeout, true, func(ctx context.Context) (bool, error) {
		node, err := getLoomNode(ctx, k8sClient, nodeName)
		if errors.IsNotFound(err) {
			// AlreadyExists means it was created in the meantime, so it is safe to carry on waiting for it.
			if err = createLoomNode(ctx, k8sClient, nodeName); err != nil && !errors.IsAlreadyExists(err) {
				logger.Info("Failed to create LoomNode, retrying", "loomNode", nodeName, "error", err.Error())
			}
			return false, nil
		}
		if meta.IsNoMatchError(err) {
			logger.V(1).Info("LoomNode CRD not installed yet, retrying", "loomNode", nodeName)
			return false, nil
		}
		if err != nil {
			logger.Info("Failed to get LoomNode, retrying", "loomNode", nodeName, "error", err.Error())
			return false, nil
		}
		if !hasCIDRs(node) {
			if !waitingLogged {
				logger.Info("Waiting for the operator to assign CIDRs to the LoomNode",
					"loomNode", nodeName, "timeout", CIDRWaitTimeout)
				waitingLogged = true
			}
			return false, nil
		}
		loomNode = node
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("error waiting for CIDRs to be assigned to loomNode %s: %w", nodeName, err)
	}
	return loomNode, nil
}

func getLoomNode(ctx context.Context, k8sClient client.Client, nodeName string) (*infrav1alpha1.LoomNode, error) {
	ctx, cancel := context.WithTimeout(ctx, KubernetesAPICallTimeout)
	defer cancel()

	loomNode := &infrav1alpha1.LoomNode{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: nodeName}, loomNode); err != nil {
		return nil, err
	}
	return loomNode, nil
}

func createLoomNode(ctx context.Context, k8sClient client.Client, nodeName string) error {
	ctx, cancel := context.WithTimeout(ctx, KubernetesAPICallTimeout)
	defer cancel()

	loomNode := &infrav1alpha1.LoomNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
		Spec: infrav1alpha1.LoomNodeSpec{
			NodeCIDRs: make([]string, 0),
		},
	}
	return k8sClient.Create(ctx, loomNode)
}

func hasCIDRs(loomNode *infrav1alpha1.LoomNode) bool {
	return len(loomNode.Spec.NodeCIDRs) > 0 && len(loomNode.Spec.TunnelCIDRs) > 0
}
