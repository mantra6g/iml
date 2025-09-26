package subscriptions

type SubscriptionType uint8
const (
	AppDefinition SubscriptionType = iota
	VnfDefinition
	RemoteAppGroups
	RemoteVnfGroups
	AppServices
	ServiceChain
)

type SubscriptionKey struct {
	ID   string
	Type SubscriptionType
}

type Subscription interface {
	Key() SubscriptionKey
	Start() error
	Stop() error
}

type AppDefinitionSubscription struct {
	AppID    string
}
func (sub *AppDefinitionSubscription) Key() SubscriptionKey {
	return SubscriptionKey{ID: sub.AppID, Type: AppDefinition}
}
func (sub *AppDefinitionSubscription) Start() error { return nil }
func (sub *AppDefinitionSubscription) Stop() error  { return nil }

type VnfDefinitionSubscription struct {
	VnfID string
}
func (sub *VnfDefinitionSubscription) Key() SubscriptionKey {
	return SubscriptionKey{ID: sub.VnfID, Type: VnfDefinition}
}
func (sub *VnfDefinitionSubscription) Start() error { return nil }
func (sub *VnfDefinitionSubscription) Stop() error  { return nil }

type RemoteAppGroupsSubscription struct {
	AppID string
}
func (sub *RemoteAppGroupsSubscription) Key() SubscriptionKey {
	return SubscriptionKey{ID: sub.AppID, Type: RemoteAppGroups}
}
func (sub *RemoteAppGroupsSubscription) Start() error { return nil }
func (sub *RemoteAppGroupsSubscription) Stop() error  { return nil }

type RemoteVnfGroupsSubscription struct {
	VnfID string
}
func (sub *RemoteVnfGroupsSubscription) Key() SubscriptionKey {
	return SubscriptionKey{ID: sub.VnfID, Type: RemoteVnfGroups}
}
func (sub *RemoteVnfGroupsSubscription) Start() error { return nil }
func (sub *RemoteVnfGroupsSubscription) Stop() error  { return nil }

type AppServicesSubscription struct {
	AppID string
}
func (sub *AppServicesSubscription) Key() SubscriptionKey {
	return SubscriptionKey{ID: sub.AppID, Type: AppServices}
}
func (sub *AppServicesSubscription) Start() error { return nil }
func (sub *AppServicesSubscription) Stop() error  { return nil }

type ServiceChainSubscription struct {
	ChainID string
}
func (sub *ServiceChainSubscription) Key() SubscriptionKey {
	return SubscriptionKey{ID: sub.ChainID, Type: ServiceChain}
}
func (sub *ServiceChainSubscription) Start() error { return nil }
func (sub *ServiceChainSubscription) Stop() error  { return nil }
