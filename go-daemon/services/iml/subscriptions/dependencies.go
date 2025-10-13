package subscriptions

import (
	"fmt"
	"iml-daemon/logger"
	"time"
)

type DependencyType uint8

const (
	TemporaryAppDep DependencyType = iota
	TemporaryVnfDep
	LocalAppDep
	LocalVnfDep
	AppDefinitionDep
	VnfDefinitionDep
	RemoteAppGroupsDep
	RemoteVnfGroupsDep
	AppServicesDep
	ServiceChainDep
	NodeDep
)

type DependencyKey struct {
	ID   string
	Type DependencyType
}

type Dependency interface {
	AddSubscriber()
	RemoveSubscriber()
	GetSubscriberCount() uint

	// PreAdd should be used to subscribe to additional dependencies.
	//
	// For example, a LocalAppDependency needs both the app definitions to be up to date and
	// the app services to be up to date. So in PreAdd it would subscribe to both of those using
	// mgr.Subscribe()
	PreAdd(mgr *SubscriptionManager) error

	// PostAdd should be used to start the necessary subscriptions.
	PostAdd(mgr *SubscriptionManager) error

	// PreRemove should be used to stop the subscriptions.
	PreRemove(mgr *SubscriptionManager) error

	// PostRemove should be used to unsubscribe from all dependencies.
	PostRemove(mgr *SubscriptionManager) error

	// Key returns the key of the dependency. Must be implemented by all types.
	Key() DependencyKey
}

type DependencyBase struct {
	subscriberCount uint
}

func (d *DependencyBase) AddSubscriber() { d.subscriberCount++ }
func (d *DependencyBase) RemoveSubscriber() {
	if d.subscriberCount == 0 {
		return
	}
	d.subscriberCount--
}
func (d *DependencyBase) GetSubscriberCount() uint { return d.subscriberCount }





// TemporarySync is a dependent that temporarily subscribes to updates.
//
// It subscribes to updates for 30 seconds. It can be used on Apps and VNFs.
type TemporaryAppDependency struct {
	DependencyBase
	AppID string
}
func (d *TemporaryAppDependency) Key() DependencyKey {
	return DependencyKey{ID: d.AppID, Type: TemporaryAppDep}
}
func (d *TemporaryAppDependency) PreAdd(mgr *SubscriptionManager) error {
	return mgr.AddDependency(&AppDefinitionDependency{AppID: d.AppID})
}
func (d *TemporaryAppDependency) PostAdd(mgr *SubscriptionManager) error {
	time.AfterFunc(30*time.Second, func() {
		err := mgr.RemoveDependency(d)
		if err != nil {
			logger.ErrorLogger().Printf("TemporaryAppDependency: failed to remove dependency for AppID %s: %v", d.AppID, err)
		} else {
			logger.DebugLogger().Printf("TemporaryAppDependency: automatically removed dependency for AppID %s after timeout", d.AppID)
		}
	})
	logger.DebugLogger().Printf("TemporaryAppDependency added for AppID %s", d.AppID)
	return nil
}
func (d *TemporaryAppDependency) PreRemove(mgr *SubscriptionManager) error {
	logger.DebugLogger().Printf("TemporaryAppDependency removed for AppID %s", d.AppID)
	return nil
}
func (d *TemporaryAppDependency) PostRemove(mgr *SubscriptionManager) error {
	return mgr.RemoveDependency(&AppDefinitionDependency{AppID: d.AppID})
}





// TemporarySync is a dependent that temporarily subscribes to updates.
//
// It subscribes to updates for 30 seconds. It can be used on Apps and VNFs.
type TemporaryVnfDependency struct {
	DependencyBase
	VnfID string
}
func (d *TemporaryVnfDependency) Key() DependencyKey {
	return DependencyKey{ID: d.VnfID, Type: TemporaryVnfDep}
}
func (d *TemporaryVnfDependency) PreAdd(mgr *SubscriptionManager) error {
	return mgr.AddDependency(&VnfDefinitionDependency{VnfID: d.VnfID})
}
func (d *TemporaryVnfDependency) PostAdd(mgr *SubscriptionManager) error {
	time.AfterFunc(30*time.Second, func() {
		err := mgr.RemoveDependency(d)
		if err != nil {
			logger.ErrorLogger().Printf("TemporaryVnfDependency: failed to remove dependency for VnfID %s: %v", d.VnfID, err)
		} else {
			logger.DebugLogger().Printf("TemporaryVnfDependency: automatically removed dependency for VnfID %s after timeout", d.VnfID)
		}
	})
	logger.DebugLogger().Printf("TemporaryVnfDependency added for VnfID %s", d.VnfID)
	return nil
}
func (d *TemporaryVnfDependency) PreRemove(mgr *SubscriptionManager) error  { return nil }
func (d *TemporaryVnfDependency) PostRemove(mgr *SubscriptionManager) error {
	return mgr.RemoveDependency(&VnfDefinitionDependency{VnfID: d.VnfID})
}





// LocalVnfGroup is a dependent that represents a local VNF group.
//
// It subscribes to updates for a VNF definition.
type LocalAppDependency struct {
	DependencyBase
	AppID string
}
func (d *LocalAppDependency) Key() DependencyKey {
	return DependencyKey{ID: d.AppID, Type: LocalAppDep}
}
func (d *LocalAppDependency) PreAdd(mgr *SubscriptionManager) error     {
	err := mgr.AddDependency(&AppDefinitionDependency{AppID: d.AppID})
	if err != nil {
		return fmt.Errorf("LocalAppDependency.PreAdd: failed to add AppDefinitionDependency for AppID %s: %w", d.AppID, err)
	}
	err = mgr.AddDependency(&AppServicesDependency{AppID: d.AppID})
	if err != nil {
		mgr.RemoveDependency(&AppDefinitionDependency{AppID: d.AppID})
		return fmt.Errorf("LocalAppDependency.PreAdd: failed to add AppServicesDependency for AppID %s: %w", d.AppID, err)
	}
	return nil
}
func (d *LocalAppDependency) PostAdd(mgr *SubscriptionManager) error    { return nil }
func (d *LocalAppDependency) PreRemove(mgr *SubscriptionManager) error  { return nil }
func (d *LocalAppDependency) PostRemove(mgr *SubscriptionManager) error {
	err := mgr.RemoveDependency(&AppDefinitionDependency{AppID: d.AppID})
	if err != nil {
		return fmt.Errorf("LocalAppDependency.PostRemove: failed to remove AppDefinitionDependency for AppID %s: %w", d.AppID, err)
	}
	err = mgr.RemoveDependency(&AppServicesDependency{AppID: d.AppID})
	if err != nil {
		return fmt.Errorf("LocalAppDependency.PostRemove: failed to remove AppServicesDependency for AppID %s: %w", d.AppID, err)
	}
	return nil
}





// LocalVnfDependency is a dependent that represents a local VNF group.
//
// It subscribes to updates for a VNF definition.
type LocalVnfDependency struct {
	DependencyBase
	VnfID string
}
func (d *LocalVnfDependency) Key() DependencyKey {
	return DependencyKey{ID: d.VnfID, Type: LocalVnfDep}
}
func (d *LocalVnfDependency) PreAdd(mgr *SubscriptionManager) error     {
	err := mgr.AddDependency(&VnfDefinitionDependency{VnfID: d.VnfID})
	if err != nil {
		return fmt.Errorf("LocalVnfDependency.PreAdd: failed to add VnfDefinitionDependency for VnfID %s: %w", d.VnfID, err)
	}
	return nil
}
func (d *LocalVnfDependency) PostAdd(mgr *SubscriptionManager) error    { return nil }
func (d *LocalVnfDependency) PreRemove(mgr *SubscriptionManager) error  { return nil }
func (d *LocalVnfDependency) PostRemove(mgr *SubscriptionManager) error {
	err := mgr.RemoveDependency(&VnfDefinitionDependency{VnfID: d.VnfID})
	if err != nil {
		return fmt.Errorf("LocalVnfDependency.PostRemove: failed to remove VnfDefinitionDependency for VnfID %s: %w", d.VnfID, err)
	}
	return nil
}





// AppDefinitionDependency
type AppDefinitionDependency struct {
	DependencyBase
	AppID string
}
func (d *AppDefinitionDependency) Key() DependencyKey {
	return DependencyKey{ID: d.AppID, Type: AppDefinitionDep}
}
func (d *AppDefinitionDependency) PreAdd(mgr *SubscriptionManager) error     { return nil }
func (d *AppDefinitionDependency) PostAdd(mgr *SubscriptionManager) error    {
	err := mgr.addSubscription(&AppDefinitionSubscription{AppID: d.AppID})
	if err != nil {
		return fmt.Errorf("AppDefinitionDependency.PostAdd: failed to add AppDefinitionSubscription for AppID %s: %w", d.AppID, err)
	}
	return nil
}
func (d *AppDefinitionDependency) PreRemove(mgr *SubscriptionManager) error  {
	err := mgr.removeSubscription(&AppDefinitionSubscription{AppID: d.AppID})
	if err != nil {
		return fmt.Errorf("AppDefinitionDependency.PreRemove: failed to remove AppDefinitionSubscription for AppID %s: %w", d.AppID, err)
	}
	return nil
}
func (d *AppDefinitionDependency) PostRemove(mgr *SubscriptionManager) error { return nil }





// VnfDefinitionDependency
type VnfDefinitionDependency struct {
	DependencyBase
	VnfID string
}
func (d *VnfDefinitionDependency) Key() DependencyKey {
	return DependencyKey{ID: d.VnfID, Type: VnfDefinitionDep}
}
func (d *VnfDefinitionDependency) PreAdd(mgr *SubscriptionManager) error     { return nil }
func (d *VnfDefinitionDependency) PostAdd(mgr *SubscriptionManager) error    {
	err := mgr.addSubscription(&VnfDefinitionSubscription{VnfID: d.VnfID})
	if err != nil {
		return fmt.Errorf("VnfDefinitionDependency.PostAdd: failed to add VnfDefinitionSubscription for VnfID %s: %w", d.VnfID, err)
	}
	return nil
}
func (d *VnfDefinitionDependency) PreRemove(mgr *SubscriptionManager) error  {
	err := mgr.removeSubscription(&VnfDefinitionSubscription{VnfID: d.VnfID})
	if err != nil {
		return fmt.Errorf("VnfDefinitionDependency.PreRemove: failed to remove VnfDefinitionSubscription for VnfID %s: %w", d.VnfID, err)
	}
	return nil
}
func (d *VnfDefinitionDependency) PostRemove(mgr *SubscriptionManager) error { return nil }






// RemoteAppGroup is a dependent that represents a remote application group.
//
// It subscribes to updates for a VNF definition.
type RemoteAppDependency struct {
	DependencyBase
	AppID string
}
func (d *RemoteAppDependency) Key() DependencyKey {
	return DependencyKey{ID: d.AppID, Type: RemoteAppGroupsDep}
}
func (d *RemoteAppDependency) PreAdd(mgr *SubscriptionManager) error     {
	err := mgr.AddDependency(&AppDefinitionDependency{AppID: d.AppID})
	if err != nil {
		return fmt.Errorf("RemoteAppDependency.PreAdd: failed to add AppDefinitionDependency for AppID %s: %w", d.AppID, err)
	}
	return nil
}
func (d *RemoteAppDependency) PostAdd(mgr *SubscriptionManager) error    {
	err := mgr.addSubscription(&RemoteAppGroupsSubscription{AppID: d.AppID})
	if err != nil {
		return fmt.Errorf("RemoteAppDependency.PostAdd: failed to add RemoteAppGroupsSubscription for AppID %s: %w", d.AppID, err)
	}
	return nil
}
func (d *RemoteAppDependency) PreRemove(mgr *SubscriptionManager) error  {
	err := mgr.removeSubscription(&RemoteAppGroupsSubscription{AppID: d.AppID})
	if err != nil {
		return fmt.Errorf("RemoteAppDependency.PreRemove: failed to remove RemoteAppGroupsSubscription for AppID %s: %w", d.AppID, err)
	}
	return nil
}
func (d *RemoteAppDependency) PostRemove(mgr *SubscriptionManager) error {
	err := mgr.RemoveDependency(&AppDefinitionDependency{AppID: d.AppID})
	if err != nil {
		return fmt.Errorf("RemoteAppDependency.PostRemove: failed to remove AppDefinitionDependency for AppID %s: %w", d.AppID, err)
	}
	return nil
}





// LocalVnfGroup is a dependent that represents a local VNF group.
//
// It subscribes to updates for a VNF definition.
type RemoteVnfDependency struct {
	DependencyBase
	VnfID string
}
func (d *RemoteVnfDependency) Key() DependencyKey {
	return DependencyKey{ID: d.VnfID, Type: RemoteVnfGroupsDep}
}
func (d *RemoteVnfDependency) PreAdd(mgr *SubscriptionManager) error     {
	err := mgr.AddDependency(&VnfDefinitionDependency{VnfID: d.VnfID})
	if err != nil {
		return fmt.Errorf("RemoteVnfDependency.PreAdd: failed to add VnfDefinitionDependency for VnfID %s: %w", d.VnfID, err)
	}
	return nil
}
func (d *RemoteVnfDependency) PostAdd(mgr *SubscriptionManager) error    {
	err := mgr.addSubscription(&RemoteVnfGroupsSubscription{VnfID: d.VnfID})
	if err != nil {
		return fmt.Errorf("RemoteVnfDependency.PostAdd: failed to add RemoteVnfGroupsSubscription for VnfID %s: %w", d.VnfID, err)
	}
	return nil
}
func (d *RemoteVnfDependency) PreRemove(mgr *SubscriptionManager) error  {
	err := mgr.removeSubscription(&RemoteVnfGroupsSubscription{VnfID: d.VnfID})
	if err != nil {
		return fmt.Errorf("RemoteVnfDependency.PreRemove: failed to remove RemoteVnfGroupsSubscription for VnfID %s: %w", d.VnfID, err)
	}
	return nil
}
func (d *RemoteVnfDependency) PostRemove(mgr *SubscriptionManager) error {
	err := mgr.RemoveDependency(&VnfDefinitionDependency{VnfID: d.VnfID})
	if err != nil {
		return fmt.Errorf("RemoteVnfDependency.PostRemove: failed to remove VnfDefinitionDependency for VnfID %s: %w", d.VnfID, err)
	}
	return nil
}





type AppServicesDependency struct {
	DependencyBase
	AppID string
}
func (d *AppServicesDependency) Key() DependencyKey {
	return DependencyKey{ID: d.AppID, Type: AppServicesDep}
}
func (d *AppServicesDependency) PreAdd(mgr *SubscriptionManager) error     { return nil }
func (d *AppServicesDependency) PostAdd(mgr *SubscriptionManager) error    {
	err := mgr.addSubscription(&AppServicesSubscription{AppID: d.AppID})
	if err != nil {
		return fmt.Errorf("AppServicesDependency.PostAdd: failed to add AppServicesSubscription for AppID %s: %w", d.AppID, err)
	}
	return nil
}
func (d *AppServicesDependency) PreRemove(mgr *SubscriptionManager) error  {
	err := mgr.removeSubscription(&AppServicesSubscription{AppID: d.AppID})
	if err != nil {
		return fmt.Errorf("AppServicesDependency.PreRemove: failed to remove AppServicesSubscription for AppID %s: %w", d.AppID, err)
	}
	return nil
}
func (d *AppServicesDependency) PostRemove(mgr *SubscriptionManager) error { return nil }





type ServiceChainDependency struct {
	DependencyBase
	ChainID string
}
func (d *ServiceChainDependency) Key() DependencyKey {
	return DependencyKey{ID: d.ChainID, Type: ServiceChainDep}
}
func (d *ServiceChainDependency) PreAdd(mgr *SubscriptionManager) error     { return nil }
func (d *ServiceChainDependency) PostAdd(mgr *SubscriptionManager) error    {
	err := mgr.addSubscription(&ServiceChainSubscription{ChainID: d.ChainID})
	if err != nil {
		return fmt.Errorf("ServiceChainDependency.PostAdd: failed to add ServiceChainSubscription for ChainID %s: %w", d.ChainID, err)
	}
	return nil
}
func (d *ServiceChainDependency) PreRemove(mgr *SubscriptionManager) error  {
	err := mgr.removeSubscription(&ServiceChainSubscription{ChainID: d.ChainID})
	if err != nil {
		return fmt.Errorf("ServiceChainDependency.PreRemove: failed to remove ServiceChainSubscription for ChainID %s: %w", d.ChainID, err)
	}
	return nil
}
func (d *ServiceChainDependency) PostRemove(mgr *SubscriptionManager) error { return nil }




type NodeDependency struct {
	DependencyBase
	NodeID string
}
func (d *NodeDependency) Key() DependencyKey {
	return DependencyKey{ID: d.NodeID, Type: LocalAppDep}
}
func (d *NodeDependency) PreAdd(mgr *SubscriptionManager) error     { return nil }
func (d *NodeDependency) PostAdd(mgr *SubscriptionManager) error		{
	err := mgr.addSubscription(&NodeSubscription{NodeID: d.NodeID})
	if err != nil {
		return fmt.Errorf("NodeDependency.PostAdd: failed to add NodeSubscription for NodeID %s: %w", d.NodeID, err)
	}
	return nil
}
func (d *NodeDependency) PreRemove(mgr *SubscriptionManager) error  {
	err := mgr.removeSubscription(&NodeSubscription{NodeID: d.NodeID})
	if err != nil {
		return fmt.Errorf("NodeDependency.PreRemove: failed to remove NodeSubscription for NodeID %s: %w", d.NodeID, err)
	}
	return nil
}
func (d *NodeDependency) PostRemove(mgr *SubscriptionManager) error {
	return nil
}