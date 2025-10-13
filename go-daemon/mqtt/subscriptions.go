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
	onUpdate(TopicUpdate)
	onDelete(TopicUpdate)
}

type Result struct {
	// Indicates whether a topic update should be requeued for processing.
	RequeueAfter time.Duration
}

type Handlers struct {
	OnUpdate func(TopicUpdate)
	OnDelete func(TopicUpdate)
}

func (s *Handlers) onUpdate(update TopicUpdate) {
	if s.OnUpdate != nil {
		s.OnUpdate(update)
	}
}
func (s *Handlers) onDelete(update TopicUpdate) {
	if s.OnDelete != nil {
		s.OnDelete(update)
	}
}

type ApplicationDefinitionSubscription struct {
	Handlers
	AppID string
}

func (s *ApplicationDefinitionSubscription) Topic() Topic {
	return Topic(fmt.Sprintf("apps/%s/definition", s.AppID))
}

type ApplicationServicesSubscription struct {
	Handlers
	AppID string
}

func (s *ApplicationServicesSubscription) Topic() Topic {
	return Topic(fmt.Sprintf("apps/%s/services", s.AppID))
}

type VNFDefinitionSubscription struct {
	Handlers
	VNFID string
}

func (s *VNFDefinitionSubscription) Topic() Topic {
	return Topic(fmt.Sprintf("nfs/%s/definition", s.VNFID))
}

type VnfGroupsSubscription struct {
	Handlers
	NfID string
}
func (s *VnfGroupsSubscription) Topic() Topic {
	return Topic(fmt.Sprintf("nfs/%s/nodes/+/groups/+", s.NfID))
}

type AppGroupsSubscription struct {
	Handlers
	AppID string
}
func (s *AppGroupsSubscription) Topic() Topic {
	return Topic(fmt.Sprintf("apps/%s/nodes/+/groups/+", s.AppID))
}

type ServiceChainDefinitionSubscription struct {
	Handlers
	ChainID string
}
func (s *ServiceChainDefinitionSubscription) Topic() Topic {
	return Topic(fmt.Sprintf("chains/%s/definition", s.ChainID))
}

type NodeDefinitionSubscription struct {
	Handlers
	NodeID string
}
func (s *NodeDefinitionSubscription) Topic() Topic {
	return Topic(fmt.Sprintf("nodes/%s/definition", s.NodeID))
}