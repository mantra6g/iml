package mqtt

import (
	"fmt"
	"iml-daemon/logger"
	"regexp"
)

const (
	UUID_REGEX_STR = "[0-9a-fA-F]{8}\\-[0-9a-fA-F]{4}\\-[0-9a-fA-F]{4}\\-[0-9a-fA-F]{4}\\-[0-9a-fA-F]{12}"
	APP_DEFINITION_TOPIC_STR   = "apps/(" + UUID_REGEX_STR + ")/definition"
	APP_SERVICES_TOPIC_STR     = "apps/(" + UUID_REGEX_STR + ")/services"
	APP_INSTANCES_TOPIC_STR    = "apps/(" + UUID_REGEX_STR + ")/nodes/(" + UUID_REGEX_STR + ")/groups/(" + UUID_REGEX_STR + ")"
	VNF_DEFINITION_TOPIC_STR   = "nfs/(" + UUID_REGEX_STR + ")/definition"
	VNF_INSTANCES_TOPIC_STR    = "nfs/(" + UUID_REGEX_STR + ")/nodes/(" + UUID_REGEX_STR + ")/groups/(" + UUID_REGEX_STR + ")"
	CHAIN_DEFINITION_TOPIC_STR = "chains/(" + UUID_REGEX_STR + ")/definition"
)

var (
	appDefinitionTopicRegex = regexp.MustCompilePOSIX(APP_DEFINITION_TOPIC_STR)
	appServicesTopicRegex   = regexp.MustCompilePOSIX(APP_SERVICES_TOPIC_STR)
	appInstancesTopicRegex  = regexp.MustCompilePOSIX(APP_INSTANCES_TOPIC_STR)
	vnfDefinitionTopicRegex = regexp.MustCompilePOSIX(VNF_DEFINITION_TOPIC_STR)
	vnfInstancesTopicRegex  = regexp.MustCompilePOSIX(VNF_INSTANCES_TOPIC_STR)
	chainDefinitionTopicRegex = regexp.MustCompilePOSIX(CHAIN_DEFINITION_TOPIC_STR)
)

type TopicObject interface {
	DataTopic() Topic
	SubscriptionTopic() Topic
}

type ApplicationDefinitionTopic struct {
	AppID string
}
func ParseApplicationDefinitionTopic(s string) (*ApplicationDefinitionTopic, error) {
	var topic ApplicationDefinitionTopic
	matches := appDefinitionTopicRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid topic format")
	}
	topic.AppID = matches[1]
	logger.DebugLogger().Printf("Parsed ApplicationDefinitionTopic: %+v", topic)
	return &topic, nil
}
func (t *ApplicationDefinitionTopic) DataTopic() Topic {
	return Topic(fmt.Sprintf("apps/%s/definition", t.AppID))
}
func (t *ApplicationDefinitionTopic) SubscriptionTopic() Topic {
	return Topic(fmt.Sprintf("apps/%s/definition", t.AppID))
}


type ApplicationServicesTopic struct {
	AppID string
}
func ParseApplicationServicesTopic(s string) (*ApplicationServicesTopic, error) {
	var topic ApplicationServicesTopic
	matches := appServicesTopicRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid topic format")
	}
	topic.AppID = matches[1]
	logger.DebugLogger().Printf("Parsed ApplicationServicesTopic: %+v", topic)
	return &topic, nil
}
func (t *ApplicationServicesTopic) DataTopic() Topic {
	return Topic(fmt.Sprintf("apps/%s/services", t.AppID))
}
func (t *ApplicationServicesTopic) SubscriptionTopic() Topic {
	return Topic(fmt.Sprintf("apps/%s/services", t.AppID))
}

type VNFDefinitionTopic struct {
	VNFID string
}
func ParseVNFDefinitionTopic(s string) (*VNFDefinitionTopic, error) {
	var topic VNFDefinitionTopic
	matches := vnfDefinitionTopicRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid topic format")
	}
	topic.VNFID = matches[1]
	logger.DebugLogger().Printf("Parsed VNFDefinitionTopic: %+v", topic)
	return &topic, nil
}
func (t *VNFDefinitionTopic) DataTopic() Topic {
	return Topic(fmt.Sprintf("nfs/%s/definition", t.VNFID))
}
func (t *VNFDefinitionTopic) SubscriptionTopic() Topic {
	return Topic(fmt.Sprintf("nfs/%s/definition", t.VNFID))
}

type RemoteAppGroupInstancesTopic struct {
	AppID   string
	NodeID  string
	GroupID string
}
func ParseRemoteAppGroupInstancesTopic(s string) (*RemoteAppGroupInstancesTopic, error) {
	var topic RemoteAppGroupInstancesTopic
	matches := appInstancesTopicRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid topic format")
	}
	topic.AppID = matches[1]
	topic.NodeID = matches[2]
	topic.GroupID = matches[3]
	logger.DebugLogger().Printf("Parsed RemoteAppGroupInstancesTopic: %+v", topic)
	return &topic, nil
}
func (t *RemoteAppGroupInstancesTopic) DataTopic() Topic {
	return Topic(fmt.Sprintf("apps/%s/nodes/%s/groups/%s", t.AppID, t.NodeID, t.GroupID))
}
func (t *RemoteAppGroupInstancesTopic) SubscriptionTopic() Topic {
	return Topic(fmt.Sprintf("apps/%s/nodes/+/groups/+", t.AppID))
}

type RemoteVNFGroupInstancesTopic struct {
	VNFID   string
	NodeID  string
	GroupID string
}
func ParseRemoteVNFGroupInstancesTopic(s string) (*RemoteVNFGroupInstancesTopic, error) {
	var topic RemoteVNFGroupInstancesTopic
	matches := vnfInstancesTopicRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid topic format")
	}
	topic.VNFID = matches[1]
	topic.NodeID = matches[2]
	topic.GroupID = matches[3]
	logger.DebugLogger().Printf("Parsed RemoteVNFGroupInstancesTopic: %+v", topic)
	return &topic, nil
}
func (t *RemoteVNFGroupInstancesTopic) DataTopic() Topic {
	return Topic(fmt.Sprintf("nfs/%s/nodes/%s/groups/%s", t.VNFID, t.NodeID, t.GroupID))
}
func (t *RemoteVNFGroupInstancesTopic) SubscriptionTopic() Topic {
	return Topic(fmt.Sprintf("nfs/%s/nodes/+/groups/+", t.VNFID))
}

type ServiceChainDefinitionTopic struct {
	ChainID string
}
func ParseServiceChainDefinitionTopic(s string) (*ServiceChainDefinitionTopic, error) {
	var topic ServiceChainDefinitionTopic
	matches := chainDefinitionTopicRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid topic format")
	}
	topic.ChainID = matches[1]
	logger.DebugLogger().Printf("Parsed ServiceChainDefinitionTopic: %+v", topic)
	return &topic, nil
}
func (t *ServiceChainDefinitionTopic) DataTopic() Topic {
	return Topic(fmt.Sprintf("chains/%s/definition", t.ChainID))
}
func (t *ServiceChainDefinitionTopic) SubscriptionTopic() Topic {
	return Topic(fmt.Sprintf("chains/%s/definition", t.ChainID))
}