package subscriptions

import (
	"fmt"
	"iml-daemon/mqtt"
)

type SubscriptionType uint8

const (
	AppDefinition SubscriptionType = iota
	VnfDefinition
	RemoteAppGroups
	RemoteVnfGroups
	AppServices
	ServiceChain
	Node
)

type SubscriptionKey struct {
	ID   string
	Type SubscriptionType
}

type Subscription interface {
	Key() SubscriptionKey
	Start(*SubscriptionManager) error
	Stop(*SubscriptionManager) error
	Dependency() Dependency
}

type AppDefinitionSubscription struct {
	AppID         string
	AppDependency *AppDefinitionDependency

	active bool
	topic  mqtt.Topic
}

func (sub *AppDefinitionSubscription) Key() SubscriptionKey {
	return SubscriptionKey{ID: sub.AppID, Type: AppDefinition}
}
func (sub *AppDefinitionSubscription) Dependency() Dependency {
	return sub.AppDependency
}
func (sub *AppDefinitionSubscription) Start(mgr *SubscriptionManager) error {
	if sub.active {
		return nil
	}
	topic, subErr := mgr.mqttClient.Add(&mqtt.ApplicationDefinitionSubscription{AppID: sub.AppID})
	if subErr != nil {
		return fmt.Errorf("AppDefinitionSubscription.Start: failed to subscribe to App ID %s updates: %w", sub.AppID, subErr)
	}
	sub.topic = topic
	sub.active = true
	return nil
}
func (sub *AppDefinitionSubscription) Stop(mgr *SubscriptionManager) error {
	if !sub.active {
		return nil
	}
	err := mgr.mqttClient.Remove(sub.topic)
	if err != nil {
		return fmt.Errorf("AppDefinitionSubscription.Stop: failed to unsubscribe from App ID %s updates: %w", sub.AppID, err)
	}
	err = mgr.repo.MarkAppAsDeleted(sub.AppID)
	if err != nil {
		return fmt.Errorf("AppDefinitionSubscription.Stop: failed to mark App ID %s as deleted in local database: %w", sub.AppID, err)
	}
	sub.active = false
	return nil
}

type VnfDefinitionSubscription struct {
	VnfID         string
	VnfDependency *VnfDefinitionDependency

	active bool
	topic  mqtt.Topic
}

func (sub *VnfDefinitionSubscription) Key() SubscriptionKey {
	return SubscriptionKey{ID: sub.VnfID, Type: VnfDefinition}
}
func (sub *VnfDefinitionSubscription) Dependency() Dependency {
	return sub.VnfDependency
}
func (sub *VnfDefinitionSubscription) Start(mgr *SubscriptionManager) error {
	if sub.active {
		return nil
	}
	topic, subErr := mgr.mqttClient.Add(&mqtt.VNFDefinitionSubscription{VNFID: sub.VnfID})
	if subErr != nil {
		return fmt.Errorf("VnfDefinitionSubscription.Start: failed to subscribe to VNF ID %s updates: %w", sub.VnfID, subErr)
	}
	sub.topic = topic
	sub.active = true
	return nil
}
func (sub *VnfDefinitionSubscription) Stop(mgr *SubscriptionManager) error {
	if !sub.active {
		return nil
	}
	if err := mgr.mqttClient.Remove(sub.topic); err != nil {
		return fmt.Errorf("VnfDefinitionSubscription.Stop: failed to unsubscribe from VNF ID %s updates: %w", sub.VnfID, err)
	}
	sub.active = false
	return nil
}

type RemoteAppGroupsSubscription struct {
	AppID               string
	RemoteAppDependency *RemoteAppDependency

	active bool
	topic  mqtt.Topic
}

func (sub *RemoteAppGroupsSubscription) Key() SubscriptionKey {
	return SubscriptionKey{ID: sub.AppID, Type: RemoteAppGroups}
}
func (sub *RemoteAppGroupsSubscription) Dependency() Dependency {
	return sub.RemoteAppDependency
}
func (sub *RemoteAppGroupsSubscription) Start(mgr *SubscriptionManager) error {
	if sub.active {
		return nil
	}
	topic, subErr := mgr.mqttClient.Add(&mqtt.AppGroupsSubscription{AppID: sub.AppID})
	if subErr != nil {
		return fmt.Errorf("failed to add MQTT subscription for remote app groups of App ID %s: %v", sub.AppID, subErr)
	}
	sub.topic = topic
	sub.active = true
	return nil
}
func (sub *RemoteAppGroupsSubscription) Stop(mgr *SubscriptionManager) error {
	if !sub.active {
		return nil
	}
	if err := mgr.mqttClient.Remove(sub.topic); err != nil {
		return fmt.Errorf("RemoteAppGroupsSubscription.Stop: failed to unsubscribe from remote app group updates for App ID %s: %w", sub.AppID, err)
	}
	sub.active = false
	return nil
}


type RemoteVnfGroupsSubscription struct {
	VnfID               string
	RemoteVnfDependency *RemoteVnfDependency

	active bool
	topic  mqtt.Topic
}

func (sub *RemoteVnfGroupsSubscription) Key() SubscriptionKey {
	return SubscriptionKey{ID: sub.VnfID, Type: RemoteVnfGroups}
}
func (sub *RemoteVnfGroupsSubscription) Dependency() Dependency {
	return sub.RemoteVnfDependency
}
func (sub *RemoteVnfGroupsSubscription) Start(mgr *SubscriptionManager) error {
	if sub.active {
		return nil
	}
	topic, subErr := mgr.mqttClient.Add(&mqtt.VnfGroupsSubscription{NfID: sub.VnfID})
	if subErr != nil {
		return fmt.Errorf("failed to add MQTT subscription for remote VNF groups of VNF ID %s: %v", sub.VnfID, subErr)
	}
	sub.topic = topic
	sub.active = true
	return nil
}
func (sub *RemoteVnfGroupsSubscription) Stop(mgr *SubscriptionManager) error {
	if !sub.active {
		return nil
	}
	err := mgr.mqttClient.Remove(sub.topic)
	if err != nil {
		return fmt.Errorf("RemoteVnfGroupsSubscription.Stop: failed to unsubscribe from remote VNF group updates for VNF ID %s: %w", sub.VnfID, err)
	}
	sub.active = false
	return nil
}

type AppServicesSubscription struct {
	AppID                 string
	AppServicesDependency *AppServicesDependency

	active bool
	topic  mqtt.Topic
}

func (sub *AppServicesSubscription) Key() SubscriptionKey {
	return SubscriptionKey{ID: sub.AppID, Type: AppServices}
}
func (sub *AppServicesSubscription) Dependency() Dependency {
	return sub.AppServicesDependency
}
func (sub *AppServicesSubscription) Start(mgr *SubscriptionManager) error {
	if sub.active {
		return nil
	}
	topic, subErr := mgr.mqttClient.Add(&mqtt.ApplicationServicesSubscription{AppID: sub.AppID})
	if subErr != nil {
		return fmt.Errorf("failed to add MQTT subscription for App ID %s: %v", sub.AppID, subErr)
	}
	sub.topic = topic
	sub.active = true
	return nil
}
func (sub *AppServicesSubscription) Stop(mgr *SubscriptionManager) error {
	if !sub.active {
		return nil
	}
	err := mgr.mqttClient.Remove(sub.topic)
	if err != nil {
		return fmt.Errorf("AppServicesSubscription.Stop: failed to unsubscribe from service chain updates for App ID %s: %w", sub.AppID, err)
	}
	sub.active = false
	return nil
}

type ServiceChainSubscription struct {
	ChainID         string
	ChainDependency *ServiceChainDependency

	active bool
	topic  mqtt.Topic
}

func (sub *ServiceChainSubscription) Key() SubscriptionKey {
	return SubscriptionKey{ID: sub.ChainID, Type: ServiceChain}
}
func (sub *ServiceChainSubscription) Dependency() Dependency {
	return sub.ChainDependency
}
func (sub *ServiceChainSubscription) Start(mgr *SubscriptionManager) error {
	if sub.active {
		return nil
	}
	topic, subErr := mgr.mqttClient.Add(&mqtt.ServiceChainDefinitionSubscription{ChainID: sub.ChainID})
	if subErr != nil {
		return fmt.Errorf("ServiceChainSubscription.Start: failed to subscribe to Service Chain ID %s updates: %w", sub.ChainID, subErr)
	}
	sub.topic = topic
	sub.active = true
	return nil
}
func (sub *ServiceChainSubscription) Stop(mgr *SubscriptionManager) error {
	if !sub.active {
		return nil
	}
	err := mgr.mqttClient.Remove(sub.topic)
	if err != nil {
		return fmt.Errorf("ServiceChainSubscription.Stop: failed to unsubscribe from Service Chain ID %s updates: %w", sub.ChainID, err)
	}
	sub.active = false
	return nil
}


type NodeSubscription struct {
	NodeID         string
	NodeDependency *NodeDependency

	active bool
	topic  mqtt.Topic
}
func (sub *NodeSubscription) Key() SubscriptionKey {
	return SubscriptionKey{ID: sub.NodeID, Type: Node}
}
func (sub *NodeSubscription) Dependency() Dependency {
	return sub.NodeDependency
}
func (sub *NodeSubscription) Start(mgr *SubscriptionManager) error {
	if sub.active {
		return nil
	}
	topic, subErr := mgr.mqttClient.Add(&mqtt.NodeDefinitionSubscription{NodeID: sub.NodeID})
	if subErr != nil {
		return fmt.Errorf("NodeSubscription.Start: failed to subscribe to Node ID %s updates: %w", sub.NodeID, subErr)
	}
	sub.topic = topic
	sub.active = true
	return nil
}
func (sub *NodeSubscription) Stop(mgr *SubscriptionManager) error {
	if !sub.active {
		return nil
	}
	err := mgr.mqttClient.Remove(sub.topic)
	if err != nil {
		return fmt.Errorf("NodeSubscription.Stop: failed to unsubscribe from Node ID %s updates: %w", sub.NodeID, err)
	}
	sub.active = false
	return nil
}