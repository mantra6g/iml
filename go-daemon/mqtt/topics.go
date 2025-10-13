package mqtt

import "fmt"

type TopicObject interface {
	DataTopic() Topic
	SubscriptionTopic() Topic
}

type ApplicationDefinitionTopic struct {
	AppID string
}
func ParseApplicationDefinitionTopic(s string) (*ApplicationDefinitionTopic, error) {
	var topic ApplicationDefinitionTopic
	_, err := fmt.Sscanf(s, "apps/%s/definition", &topic.AppID)
	if err != nil {
		return nil, err
	}
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
	_, err := fmt.Sscanf(s, "apps/%s/services", &topic.AppID)
	if err != nil {
		return nil, err
	}
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
	_, err := fmt.Sscanf(s, "nfs/%s/definition", &topic.VNFID)
	if err != nil {
		return nil, err
	}
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
	_, err := fmt.Sscanf(s, "apps/%s/nodes/%s/groups/%s", &topic.AppID, &topic.NodeID, &topic.GroupID)
	if err != nil {
		return nil, err
	}
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
	_, err := fmt.Sscanf(s, "nfs/%s/nodes/%s/groups/%s", &topic.VNFID, &topic.NodeID, &topic.GroupID)
	if err != nil {
		return nil, err
	}
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
	_, err := fmt.Sscanf(s, "chains/%s/definition", &topic.ChainID)
	if err != nil {
		return nil, err
	}
	return &topic, nil
}
func (t *ServiceChainDefinitionTopic) DataTopic() Topic {
	return Topic(fmt.Sprintf("chains/%s/definition", t.ChainID))
}
func (t *ServiceChainDefinitionTopic) SubscriptionTopic() Topic {
	return Topic(fmt.Sprintf("chains/%s/definition", t.ChainID))
}