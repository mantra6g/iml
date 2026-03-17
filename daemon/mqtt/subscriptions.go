package mqtt

import (
	"fmt"
	"time"
)

// Subscription holds information about a subscription to a topic pattern.
//
// For example, if you subscribe to "apps/+/definition", there will be a single Subscription
// instance for this pattern, but multiple TopicData instances for each exact topic.
type Subscription interface {
	Topic() Topic
}

type Result struct {
	// Indicates whether a topic update should be requeued for processing.
	RequeueAfter time.Duration
}

type ApplicationDefinitionSubscription struct {
	AppID string
}

func (s *ApplicationDefinitionSubscription) Topic() Topic {
	return Topic(fmt.Sprintf("apps/%s/definition", s.AppID))
}

type ApplicationServicesSubscription struct {
	AppID string
}

func (s *ApplicationServicesSubscription) Topic() Topic {
	return Topic(fmt.Sprintf("apps/%s/services", s.AppID))
}

type VNFDefinitionSubscription struct {
	VNFID string
}

func (s *VNFDefinitionSubscription) Topic() Topic {
	return Topic(fmt.Sprintf("nfs/%s/definition", s.VNFID))
}

type VnfGroupsSubscription struct {
	NfID string
}

func (s *VnfGroupsSubscription) Topic() Topic {
	return Topic(fmt.Sprintf("nfs/%s/nodes/+/groups/+", s.NfID))
}

type AppGroupsSubscription struct {
	AppID string
}

func (s *AppGroupsSubscription) Topic() Topic {
	return Topic(fmt.Sprintf("apps/%s/nodes/+/groups/+", s.AppID))
}

type ServiceChainDefinitionSubscription struct {
	ChainID string
}

func (s *ServiceChainDefinitionSubscription) Topic() Topic {
	return Topic(fmt.Sprintf("chains/%s/definition", s.ChainID))
}

type NodeDefinitionSubscription struct {
	NodeID string
}

func (s *NodeDefinitionSubscription) Topic() Topic {
	return Topic(fmt.Sprintf("nodes/%s/definition", s.NodeID))
}
