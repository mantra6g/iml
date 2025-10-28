package subscriptions

import (
	"fmt"
	"iml-daemon/db"
	"iml-daemon/logger"
	"iml-daemon/mqtt"
	"sync"
)

type SubscriptionManager struct {
	subscriptions map[SubscriptionKey]Subscription
	dependencies  map[DependencyKey]Dependency
	mqttClient    *mqtt.Client
	repo          *db.Registry

	_mutex sync.RWMutex
}

func NewSubscriptionManager(mqttClient *mqtt.Client, repo *db.Registry) (*SubscriptionManager, error) {
	if mqttClient == nil {
		return nil, fmt.Errorf("SubscriptionManager.NewSubscriptionManager: mqttClient is nil")
	}
	if repo == nil {
		return nil, fmt.Errorf("SubscriptionManager.NewSubscriptionManager: repo is nil")
	}

	return &SubscriptionManager{
		subscriptions: make(map[SubscriptionKey]Subscription),
		dependencies:  make(map[DependencyKey]Dependency),
		mqttClient:    mqttClient,
		repo:         repo,
	}, nil
}

func (mgr *SubscriptionManager) AddDependency(dep Dependency) error {
	key := dep.Key()
	_, exists := mgr.GetDependency(key)
	if !exists {
		return mgr.executeAddDependency(key, dep)
	}
	return mgr.executeAddReferenceToDependency(key)
}

func (mgr *SubscriptionManager) RemoveDependency(dep Dependency) error {
	key := dep.Key()
	_, exists := mgr.GetDependency(key)
	if !exists {
		return fmt.Errorf("SubscriptionManager.RemoveDependency: dependency %v not found", key)
	}
	mgr.executeRemoveReferenceFromDependency(key)
	if dep.GetSubscriberCount() == 0 {
		return mgr.executeRemoveDependency(key)
	}
	return nil
}

func (mgr *SubscriptionManager) addSubscription(sub Subscription) error {
	key := sub.Key()
	_, exists := mgr.GetSubscription(key)
	if !exists {
		return mgr.executeAddSubscription(key, sub)
	}
	return nil
}

func (mgr *SubscriptionManager) removeSubscription(sub Subscription) error {
	key := sub.Key()
	_, exists := mgr.GetSubscription(key)
	if !exists {
		logger.DebugLogger().Printf("SubscriptionManager.removeSubscription: subscription %v not found", key)
		return nil
	}
	return mgr.executeRemoveSubscription(key)
}

func (mgr *SubscriptionManager) OnSubscriptionEnded(sub Subscription) error {
	err := mgr.executeRemoveDependency(sub.Dependency().Key())
	if err != nil {
		return fmt.Errorf("SubscriptionManager.onSubscriptionEnded: failed to remove dependency for subscription %v: %w", sub.Key(), err)
	}
	return nil
}

func (mgr *SubscriptionManager) executeAddDependency(key DependencyKey, dependency Dependency) error {
	if err := dependency.PreAdd(mgr); err != nil {
		return err
	}
	mgr._mutex.Lock()
	if _, exists := mgr.dependencies[key]; !exists {
		mgr.dependencies[key] = dependency
	}
	mgr._mutex.Unlock()
	if err := dependency.PostAdd(mgr); err != nil {
		return err
	}
	return nil
}

func (mgr *SubscriptionManager) executeAddSubscription(key SubscriptionKey, sub Subscription) error {
	if err := sub.Start(mgr); err != nil {
		return fmt.Errorf("SubscriptionManager.addSubscription: failed to start subscription %v: %w", key, err)
	}
	mgr._mutex.Lock()
	mgr.subscriptions[key] = sub
	mgr._mutex.Unlock()
	return nil
}

func (mgr *SubscriptionManager) executeRemoveDependency(key DependencyKey) error {
	dep, exists := mgr.GetDependency(key)
	if !exists {
		logger.DebugLogger().Printf("SubscriptionManager.removeDependency: dependency %v not found", key)
		return nil
	}
	if err := dep.PreRemove(mgr); err != nil {
		return fmt.Errorf("SubscriptionManager.removeDependency: failed to pre-remove dependency %v: %w", key, err)
	}
	mgr._mutex.Lock()
	delete(mgr.dependencies, key)
	mgr._mutex.Unlock()
	if err := dep.PostRemove(mgr); err != nil {
		return fmt.Errorf("SubscriptionManager.removeDependency: failed to post-remove dependency %v: %w", key, err)
	}
	return nil
}

func (mgr *SubscriptionManager) executeRemoveSubscription(key SubscriptionKey) error {
	sub, exists := mgr.GetSubscription(key)
	if !exists {
		return fmt.Errorf("SubscriptionManager.removeSubscription: subscription %v not found", key)
	}
	if err := sub.Stop(mgr); err != nil {
		return fmt.Errorf("SubscriptionManager.removeSubscription: failed to stop subscription %v: %w", key, err)
	}
	mgr._mutex.Lock()
	delete(mgr.subscriptions, key)
	mgr._mutex.Unlock()
	return nil
}

func (mgr *SubscriptionManager) executeAddReferenceToDependency(key DependencyKey) error {
	mgr._mutex.Lock()
	defer mgr._mutex.Unlock()
	dep, exists := mgr.dependencies[key]
	if !exists {
		return fmt.Errorf("SubscriptionManager.addReferenceToDependency: dependency %v not found", key)
	}
	dep.AddSubscriber()
	return nil
}

func (mgr *SubscriptionManager) executeRemoveReferenceFromDependency(key DependencyKey) error {
	mgr._mutex.Lock()
	defer mgr._mutex.Unlock()
	dep, exists := mgr.dependencies[key]
	if !exists {
		return fmt.Errorf("SubscriptionManager.removeReferenceFromDependency: dependency %v not found", key)
	}
	dep.RemoveSubscriber()
	return nil
}

func (mgr *SubscriptionManager) GetDependency(key DependencyKey) (Dependency, bool) {
	mgr._mutex.RLock()
	defer mgr._mutex.RUnlock()
	deps, exists := mgr.dependencies[key]
	return deps, exists
}

func (mgr *SubscriptionManager) GetSubscription(key SubscriptionKey) (Subscription, bool) {
	mgr._mutex.RLock()
	defer mgr._mutex.RUnlock()
	sub, exists := mgr.subscriptions[key]
	return sub, exists
}
