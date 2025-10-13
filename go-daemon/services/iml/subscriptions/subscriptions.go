package subscriptions

import (
	"fmt"
	"iml-daemon/logger"
	"iml-daemon/models"
	"iml-daemon/mqtt"

	"github.com/r3labs/diff"
)

type SubscriptionType uint8

const (
	AppDefinition SubscriptionType = iota
	VnfDefinition
	RemoteAppGroups
	RemoteAppInstances
	RemoteVnfGroups
	RemoteVnfInstances
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
	topic, subErr := mgr.mqttClient.Add(&mqtt.ApplicationDefinitionSubscription{
		AppID: sub.AppID,
		Handlers: mqtt.Handlers{
			OnUpdate: func(update mqtt.TopicUpdate) {
				logger.InfoLogger().Printf("Received update for App ID %s: %+v", sub.AppID, update)
				newAppDef, ok := update.NewMessage.(*mqtt.ApplicationDefinition)
				if !ok {
					logger.ErrorLogger().Printf("Failed to cast update message for App ID %s", sub.AppID)
					return
				}
				localApp, _ := mgr.repo.FindActiveAppByGlobalID(newAppDef.ID)
				if localApp == nil {
					localApp = &models.Application{
						GlobalID: newAppDef.ID,
						Status:   models.AppStatusActive,
					}
				}
				for _, change := range update.ChangeLog {
					switch change.Path {
					default:
						logger.DebugLogger().Printf("Unhandled change path '%s' for App ID %s", change.Path, sub.AppID)
					}
				}
				if err := mgr.repo.SaveApp(localApp); err != nil {
					logger.ErrorLogger().Printf("Failed to update App ID %s in local database: %v", localApp.GlobalID, err)
				}
			},
			OnDelete: func(update mqtt.TopicUpdate) {
				logger.InfoLogger().Printf("Received delete for App ID %s: %+v", sub.AppID, update)
				if err := sub.Stop(mgr); err != nil {
					logger.ErrorLogger().Printf("Failed to stop subscription for App ID %s: %v", sub.AppID, err)
					return
				}
				if err := mgr.onSubscriptionEnded(sub); err != nil {
					logger.ErrorLogger().Printf("Failed to handle subscription end for App ID %s: %v", sub.AppID, err)
					return
				}
				logger.InfoLogger().Printf("Successfully stopped subscription for App ID %s", sub.AppID)
			},
		},
	})
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
	topic, subErr := mgr.mqttClient.Add(&mqtt.VNFDefinitionSubscription{
		VNFID: sub.VnfID,
		Handlers: mqtt.Handlers{
			OnUpdate: func(update mqtt.TopicUpdate) {
				logger.InfoLogger().Printf("Received update for VNF ID %s: %+v", sub.VnfID, update)
				newVnfDef, ok := update.NewMessage.(*mqtt.NetworkFunctionDefinition)
				if !ok {
					logger.ErrorLogger().Printf("Failed to cast new message to NetworkFunctionDefinition for VNF ID %s", sub.VnfID)
					return
				}
				localVnf, _ := mgr.repo.FindActiveNetworkFunctionByGlobalID(newVnfDef.ID)
				if localVnf == nil {
					localVnf = &models.VirtualNetworkFunction{
						GlobalID: newVnfDef.ID,
						Status:   models.VNFStatusActive,
					}
				}
				for _, change := range update.ChangeLog {
					switch change.Path {
					default:
						logger.DebugLogger().Printf("Unhandled change path '%s' for VNF ID %s", change.Path, sub.VnfID)
					}
				}
				if err := mgr.repo.SaveVnf(localVnf); err != nil {
					logger.ErrorLogger().Printf("Failed to update VNF ID %s in local database: %v", localVnf.GlobalID, err)
				}
			},
			OnDelete: func(update mqtt.TopicUpdate) {
				logger.InfoLogger().Printf("Received delete for VNF ID %s: %+v", sub.VnfID, update)
				if err := sub.Stop(mgr); err != nil {
					logger.ErrorLogger().Printf("Failed to stop subscription for VNF ID %s: %v", sub.VnfID, err)
				}
				if err := mgr.onSubscriptionEnded(sub); err != nil {
					logger.ErrorLogger().Printf("Failed to handle subscription end for VNF ID %s: %v", sub.VnfID, err)
					return
				}
				logger.InfoLogger().Printf("Successfully stopped subscription for VNF ID %s", sub.VnfID)
			},
		},
	})
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
	if err := mgr.repo.MarkVnfAsDeleted(sub.VnfID); err != nil {
		return fmt.Errorf("failed to mark VNF ID %s as deleted in local database: %v", sub.VnfID, err)
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
	topic, subErr := mgr.mqttClient.Add(&mqtt.AppGroupsSubscription{
		AppID: sub.AppID,
		Handlers: mqtt.Handlers{
			OnUpdate: func(update mqtt.TopicUpdate) {
				logger.InfoLogger().Printf("Received update for remote app groups of App ID %s: %+v", sub.AppID, update)
				newAppGroup, ok := update.NewMessage.(*mqtt.AppInstances)
				if !ok {
					logger.ErrorLogger().Printf("Failed to cast new message to AppInstances for App ID %s", sub.AppID)
					return
				}
				appGroupEntry, _ := mgr.repo.FindRemoteAppGroupByNodeAndExternalID(newAppGroup.NodeID, newAppGroup.GroupID)
				if appGroupEntry == nil {
					appGroupEntry = &models.RemoteAppGroup{
						ExternalGroupID: newAppGroup.AppID,
					}
				}

				var addedNodeID string
				var addedInstanceIPs, removedInstanceIPs []string
				for _, change := range update.ChangeLog {
					switch change.Path[0] {
					case "node_id":
						if change.Type != diff.CREATE {
							logger.DebugLogger().Printf("Unhandled change type '%s' for node_id in remote app groups of App ID %s", change.Type, sub.AppID)
							continue
						}
						if newNodeID, ok := change.To.(string); ok {
							addedNodeID = newNodeID
						}
					case "instance_ips":
						if change.Type == diff.CREATE {
							if instanceID, ok := change.To.(string); ok {
								addedInstanceIPs = append(addedInstanceIPs, instanceID)
							}
						} else if change.Type == diff.DELETE {
							if instanceID, ok := change.From.(string); ok {
								removedInstanceIPs = append(removedInstanceIPs, instanceID)
							}
						}
					default:
						logger.DebugLogger().Printf("Unhandled change path '%s' for remote app groups of App ID %s", change.Path, sub.AppID)
					}
				}
				if addedNodeID != "" {
					err := mgr.AddDependency(&NodeDependency{NodeID: addedNodeID})
					if err != nil {
						logger.ErrorLogger().Printf("Failed to add dependency for node ID %s: %v", addedNodeID, err)
						return
					}
				}

				node, err := mgr.repo.FindActiveNodeByGlobalID(newAppGroup.NodeID)
				if err != nil {
					logger.ErrorLogger().Printf("Failed to find active node by global ID %s: %v", newAppGroup.NodeID, err)
					return
				}
				appGroupEntry.NodeID = node.ID


				err = mgr.repo.RemoveRemoteAppInstancesByIP(removedInstanceIPs, appGroupEntry.ID)
				if err != nil {
					logger.ErrorLogger().Printf("Failed to remove remote app instances by IPs %v: %v", removedInstanceIPs, err)
					return
				}
				var instances []models.RemoteAppInstance
				for _, instanceIP := range addedInstanceIPs {
					instance := models.RemoteAppInstance{
						GroupID: appGroupEntry.ID,
						IP:      instanceIP,
					}
					instances = append(instances, instance)
				}
				appGroupEntry.Instances = instances
				err = mgr.repo.SaveRemoteAppGroup(appGroupEntry)
				if err != nil {
					logger.ErrorLogger().Printf("Failed to add remote app instances by IPs %v: %v", addedInstanceIPs, err)
					return
				}
				logger.InfoLogger().Printf("Successfully processed remote app group update for App ID %s", sub.AppID)
			},
			OnDelete: func(update mqtt.TopicUpdate) {
				logger.InfoLogger().Printf("Received delete for remote app groups of App ID %s: %+v", sub.AppID, update)
				if err := sub.Stop(mgr); err != nil {
					logger.ErrorLogger().Printf("Failed to stop subscription for remote app groups of App ID %s: %v", sub.AppID, err)
					return
				}
				if err := mgr.onSubscriptionEnded(sub); err != nil {
					logger.ErrorLogger().Printf("Failed to call onSubscriptionEnded for remote app groups of App ID %s: %v", sub.AppID, err)
					return
				}
				logger.InfoLogger().Printf("Successfully stopped subscription for remote app groups of App ID %s", sub.AppID)
			},
		},
	})
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
	if err := mgr.repo.RemoveRemoteAppGroupsByGlobalAppID(sub.AppID); err != nil {
		return fmt.Errorf("failed to remove remote app groups for App ID %s from local database: %v", sub.AppID, err)
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
	topic, subErr := mgr.mqttClient.Add(&mqtt.VnfGroupsSubscription{
		NfID: sub.VnfID,
		Handlers: mqtt.Handlers{
			OnUpdate: func(update mqtt.TopicUpdate) {
				logger.InfoLogger().Printf("Received update for remote VNF groups of VNF ID %s: %+v", sub.VnfID, update)
				newVnfGroup, ok := update.NewMessage.(*mqtt.VnfInstances)
				if !ok {
					logger.ErrorLogger().Printf("Failed to cast new message to VnfInstances for VNF ID %s", sub.VnfID)
					return
				}
				vnfGroupEntry, _ := mgr.repo.FindRemoteVnfGroupByNodeAndExternalID(newVnfGroup.NodeID, newVnfGroup.GroupID)
				if vnfGroupEntry == nil {
					vnfGroupEntry = &models.RemoteVnfGroup{
						ExternalGroupID:  newVnfGroup.GroupID,
					}
				}
				var addedNodeID string
				for _, change := range update.ChangeLog {
					switch change.Path[0] {
					case "node_id":
						if change.Type != diff.CREATE {
							logger.DebugLogger().Printf("Unhandled change type '%s' for node_id in remote vnf groups of VNF ID %s", change.Type, sub.VnfID)
							continue
						}
						if newNodeID, ok := change.To.(string); ok {
							addedNodeID = newNodeID
						}
					case "group_sid":
						if change.Type != diff.CREATE && change.Type != diff.UPDATE {
							logger.DebugLogger().Printf("Unhandled change type '%s' for remote group's SID", change.Type)
							continue
						}
						if newSID, ok := change.To.(string); ok {
							vnfGroupEntry.SID = newSID
						}
					default:
						logger.DebugLogger().Printf("Unhandled change path '%s' for remote VNF groups of VNF ID %s", change.Path, sub.VnfID)
					}
				}

				if addedNodeID != "" {
					err := mgr.AddDependency(&NodeDependency{NodeID: addedNodeID})
					if err != nil {
						logger.ErrorLogger().Printf("Failed to add dependency for node ID %s: %v", addedNodeID, err)
						return
					}
				}

				node, err := mgr.repo.FindActiveNodeByGlobalID(newVnfGroup.NodeID)
				if err != nil {
					logger.ErrorLogger().Printf("Failed to find active node by global ID %s: %v", newVnfGroup.NodeID, err)
					return
				}
				vnfGroupEntry.WorkerID = node.ID
				err = mgr.repo.SaveRemoteVnfGroup(vnfGroupEntry)
				if err != nil {
					logger.ErrorLogger().Printf("Failed to save remote VNF group for VNF ID %s: %v", sub.VnfID, err)
					return
				}
				logger.InfoLogger().Printf("Successfully processed remote VNF group update for VNF ID %s", sub.VnfID)
			},
			OnDelete: func(update mqtt.TopicUpdate) {
				logger.InfoLogger().Printf("Received delete for remote VNF groups of VNF ID %s: %+v", sub.VnfID, update)
				if err := sub.Stop(mgr); err != nil {
					logger.ErrorLogger().Printf("Failed to stop subscription for remote VNF groups of VNF ID %s: %v", sub.VnfID, err)
					return
				}
				if err := mgr.onSubscriptionEnded(sub); err != nil {
					logger.ErrorLogger().Printf("Failed to call onSubscriptionEnded for remote VNF groups of VNF ID %s: %v", sub.VnfID, err)
					return
				}
				logger.InfoLogger().Printf("Successfully stopped subscription for remote VNF groups of VNF ID %s", sub.VnfID)
			},
		},
	})
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
	if err := mgr.repo.RemoveRemoteVnfGroupsByGlobalVnfID(sub.VnfID); err != nil {
		return fmt.Errorf("failed to remove remote VNF groups for VNF ID %s from local database: %v", sub.VnfID, err)
	}
	sub.active = false
	return nil
}

type AppServicesSubscription struct {
	AppID                 string
	Chains                []string
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
	topic, subErr := mgr.mqttClient.Add(&mqtt.ApplicationServicesSubscription{
		AppID: sub.AppID,
		Handlers: mqtt.Handlers{
			OnUpdate: func(update mqtt.TopicUpdate) {
				logger.InfoLogger().Printf("Received update for Services of App ID %s: %+v", sub.AppID, update)
				newAppServices, ok := update.NewMessage.(*mqtt.ApplicationServiceChains)
				if !ok {
					logger.ErrorLogger().Printf("Failed to cast new message to ApplicationServiceChains for App ID %s", sub.AppID)
					return
				}
				localApp, err := mgr.repo.FindActiveAppByGlobalID(newAppServices.AppID)
				if err != nil || localApp == nil {
					logger.ErrorLogger().Printf("App ID %s for service chains not found in local database", newAppServices.AppID)
					return
				}
				var addedChains, removedChains []string
				for _, change := range update.ChangeLog {
					switch change.Path[0] {
					case "chains":
						if change.Type == "add" {
							if chainID, ok := change.To.(string); ok {
								addedChains = append(addedChains, chainID)
							}
						} else if change.Type == "remove" {
							if chainID, ok := change.From.(string); ok {
								removedChains = append(removedChains, chainID)
							}
						}
					default:
						logger.DebugLogger().Printf("Unhandled change path '%s' for Services of App ID %s", change.Path, sub.AppID)
					}
				}
				for _, chainID := range addedChains {
					err := mgr.AddDependency(&ServiceChainDependency{ChainID: chainID})
					if err != nil {
						logger.ErrorLogger().Printf("Failed to add dependency for Service Chain ID %s of App ID %s: %v", chainID, sub.AppID, err)
						continue
					}
				}
				for _, chainID := range removedChains {
					err := mgr.RemoveDependency(&ServiceChainDependency{ChainID: chainID})
					if err != nil {
						logger.ErrorLogger().Printf("Failed to remove dependency for Service Chain ID %s of App ID %s: %v", chainID, sub.AppID, err)
						continue
					}
				}
				sub.Chains = newAppServices.Chains
				logger.InfoLogger().Printf("Successfully processed service chains update for App ID %s in local database", localApp.GlobalID)
			},
			OnDelete: func(update mqtt.TopicUpdate) {
				logger.InfoLogger().Printf("Received delete for Services of App ID %s: %+v", sub.AppID, update)
				if err := sub.Stop(mgr); err != nil {
					logger.ErrorLogger().Printf("Failed to stop subscription for Services of App ID %s: %v", sub.AppID, err)
					return
				}
				if err := mgr.onSubscriptionEnded(sub); err != nil {
					logger.ErrorLogger().Printf("Failed to handle subscription end for Services of App ID %s: %v", sub.AppID, err)
					return
				}
				logger.InfoLogger().Printf("Successfully stopped subscription for Services of App ID %s", sub.AppID)
			},
		},
	})
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
	for _, chainID := range sub.Chains {
		err := mgr.RemoveDependency(&ServiceChainDependency{ChainID: chainID})
		if err != nil {
			logger.ErrorLogger().Printf("Failed to remove dependency for Service Chain ID %s of App ID %s: %v", chainID, sub.AppID, err)
			continue
		}
	}
	sub.active = false
	return nil
}

type ServiceChainSubscription struct {
	ChainID         string
	SrcAppID        string
	DstAppID        string
	Vnfs            []string
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
	topic, subErr := mgr.mqttClient.Add(&mqtt.ServiceChainDefinitionSubscription{
		ChainID: sub.ChainID,
		Handlers: mqtt.Handlers{
			OnUpdate: func(update mqtt.TopicUpdate) {
				logger.InfoLogger().Printf("Received update for Service Chain ID %s: %+v", sub.ChainID, update)
				chain, ok := update.NewMessage.(*mqtt.ServiceChainDefinition)
				if !ok {
					logger.ErrorLogger().Printf("Failed to cast new message to ServiceChainDefinition for Chain ID %s", sub.ChainID)
					return
				}
				localChain, _ := mgr.repo.FindActiveNetworkServiceChainByGlobalID(chain.ID)
				if localChain == nil {
					localChain = &models.ServiceChain{
						GlobalID: chain.ID,
						Status:   models.ServiceChainStatusActive,
					}
				}
				var removedApps, addedApps []string
				var removedVnfs, addedVnfs []string
				for _, change := range update.ChangeLog {
					switch change.Path[0] {
					case "dst_app_id":
						if change.Type == "replace" {
							if oldAppID, ok := change.From.(string); ok {
								removedApps = append(removedApps, oldAppID)
							}
							if newAppID, ok := change.To.(string); ok {
								addedApps = append(addedApps, newAppID)
							}
						}
					case "functions":
						if change.Type == "add" {
							if vnfID, ok := change.To.(string); ok {
								addedVnfs = append(addedVnfs, vnfID)
							}
						} else if change.Type == "remove" {
							if vnfID, ok := change.From.(string); ok {
								removedVnfs = append(removedVnfs, vnfID)
							}
						}
					default:
						logger.DebugLogger().Printf("Unhandled change path '%s' for Service Chain ID %s", change.Path, sub.ChainID)
					}
				}
				for _, appID := range removedApps {
					err := mgr.RemoveDependency(&RemoteAppDependency{AppID: appID})
					if err != nil {
						logger.ErrorLogger().Printf("Failed to remove dependency for App ID %s of Service Chain ID %s: %v", appID, sub.ChainID, err)
						continue
					}
				}
				for _, appID := range addedApps {
					err := mgr.AddDependency(&RemoteAppDependency{AppID: appID})
					if err != nil {
						logger.ErrorLogger().Printf("Failed to add dependency for App ID %s of Service Chain ID %s: %v", appID, sub.ChainID, err)
						continue
					}
				}
				for _, vnfID := range removedVnfs {
					err := mgr.RemoveDependency(&RemoteVnfDependency{VnfID: vnfID})
					if err != nil {
						logger.ErrorLogger().Printf("Failed to remove dependency for VNF ID %s of Service Chain ID %s: %v", vnfID, sub.ChainID, err)
						continue
					}
				}
				for _, vnfID := range addedVnfs {
					err := mgr.AddDependency(&RemoteVnfDependency{VnfID: vnfID})
					if err != nil {
						logger.ErrorLogger().Printf("Failed to add dependency for VNF ID %s of Service Chain ID %s: %v", vnfID, sub.ChainID, err)
						continue
					}
				}
				srcApp, err := mgr.repo.FindActiveAppByGlobalID(chain.SrcAppID)
				if err != nil || srcApp == nil {
					logger.ErrorLogger().Printf("Source App ID %s for Service Chain ID %s not found in local database", chain.SrcAppID, sub.ChainID)
					return
				}
				dstApp, err := mgr.repo.FindActiveAppByGlobalID(chain.DstAppID)
				if err != nil || dstApp == nil {
					logger.ErrorLogger().Printf("Destination App ID %s for Service Chain ID %s not found in local database", chain.DstAppID, sub.ChainID)
					return
				}
				var vnfs []models.ServiceChainVnfs
				for i, vnfID := range chain.Functions {
					vnfRec, err := mgr.repo.FindActiveNetworkFunctionByGlobalID(vnfID)
					if err != nil || vnfRec == nil {
						logger.ErrorLogger().Printf("Virtual Network Function ID %s for Service Chain ID %s not found in local database", vnfID, sub.ChainID)
						return
					}
					vnfs = append(vnfs, models.ServiceChainVnfs{
						Position: uint8(i),
						VnfID:    vnfRec.ID,
					})
				}
				localChain.SrcAppID = srcApp.ID
				localChain.DstAppID = dstApp.ID
				localChain.Elements = vnfs
				if err := mgr.repo.SaveNetworkServiceChain(localChain); err != nil {
					logger.ErrorLogger().Printf("Failed to update/create Service Chain ID %s in local database: %v", sub.ChainID, err)
					return
				}
			},
			OnDelete: func(update mqtt.TopicUpdate) {
				logger.InfoLogger().Printf("Received delete for Service Chain ID %s: %+v", sub.ChainID, update)
				if err := sub.Stop(mgr); err != nil {
					logger.ErrorLogger().Printf("Failed to stop subscription for Service Chain ID %s: %v", sub.ChainID, err)
					return
				}
				if err := mgr.onSubscriptionEnded(sub); err != nil {
					logger.ErrorLogger().Printf("Failed to handle subscription end for Service Chain ID %s: %v", sub.ChainID, err)
					return
				}
				logger.InfoLogger().Printf("Successfully stopped subscription for Service Chain ID %s", sub.ChainID)
			},
		},
	})
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
	if err := mgr.repo.MarkServiceChainAsDeleted(sub.ChainID); err != nil {
		return fmt.Errorf("failed to mark Service Chain ID %s as deleted in local database: %v", sub.ChainID, err)
	}
	if err = mgr.RemoveDependency(&RemoteAppDependency{AppID: sub.DstAppID}); err != nil {
		logger.ErrorLogger().Printf("Failed to remove dependency for Destination App ID %s of Service Chain ID %s: %v", sub.DstAppID, sub.ChainID, err)
	}
	for _, vnfID := range sub.Vnfs {
		err := mgr.RemoveDependency(&RemoteVnfDependency{VnfID: vnfID})
		if err != nil {
			logger.ErrorLogger().Printf("Failed to remove dependency for Service Chain ID %s of VNF ID %s: %v", sub.ChainID, vnfID, err)
			continue
		}
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
	topic, subErr := mgr.mqttClient.Add(&mqtt.NodeDefinitionSubscription{
		NodeID: sub.NodeID,
		Handlers: mqtt.Handlers{
			OnUpdate: func(update mqtt.TopicUpdate) {
				logger.InfoLogger().Printf("Received update for Node ID %s: %+v", sub.NodeID, update)
				newNodeDef, ok := update.NewMessage.(*mqtt.NodeDefinition)
				if !ok {
					logger.ErrorLogger().Printf("Failed to cast new message to NodeDefinition for Node ID %s", sub.NodeID)
					return
				}
				localNode, _ := mgr.repo.FindActiveNodeByGlobalID(newNodeDef.ID)
				if localNode == nil {
					localNode = &models.Worker{
						Status:   models.WorkerStatusActive,
						GlobalID: newNodeDef.ID,
					}
				}
				for _, change := range update.ChangeLog {
					switch change.Path[0] {
					case "ip":
						if change.Type != diff.CREATE && change.Type != diff.UPDATE {
							logger.DebugLogger().Printf("Unhandled change type '%s' for Node ID %s", change.Type, sub.NodeID)
							continue
						}
						if newIP, ok := change.To.(string); ok {
							localNode.IP = newIP
						}
					case "decapsulation_sid":
						if change.Type != diff.CREATE && change.Type != diff.UPDATE {
							logger.DebugLogger().Printf("Unhandled change type '%s' for decapsulation_sid of Node ID %s", change.Type, sub.NodeID)
							continue
						}
						if newSID, ok := change.To.(string); ok {
							localNode.DecapSID = newSID
						}
					default:
						logger.DebugLogger().Printf("Unhandled change path '%s' for Node ID %s", change.Path, sub.NodeID)
					}
				}
				if err := mgr.repo.SaveNode(localNode); err != nil {
					logger.ErrorLogger().Printf("Failed to update/create Node ID %s in local database: %v", localNode.GlobalID, err)
					return
				}
				logger.InfoLogger().Printf("Successfully updated/created Node ID %s in local database", localNode.GlobalID)
			},
			OnDelete: func(update mqtt.TopicUpdate) {

			},
		},
	})
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
	if err := mgr.repo.MarkNodeAsDeleted(sub.NodeID); err != nil {
		return fmt.Errorf("failed to mark Node ID %s as deleted in local database: %v", sub.NodeID, err)
	}
	sub.active = false
	return nil
}