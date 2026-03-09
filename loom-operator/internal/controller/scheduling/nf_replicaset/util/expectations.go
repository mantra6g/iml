// Great deal of code in this file is adapted from pkg/controller/controller_utils.go and
// pkg/controller/replicaset/replicaset_controller.go in Kubernetes, which are licensed under Apache 2.0 License,
// with some modifications to fit the needs of nf_replicaset controller. The original license is as follows:
/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"

	schedulingv1alpha1 "loom/api/scheduling/v1alpha1"
)

const (
	// If a watch drops a delete event for a pod, it'll take this long
	// before a dormant controller waiting for those packets is woken up anyway. It is
	// specifically targeted at the case where some problem prevents an update
	// of expectations, without it the controller could stay asleep forever. This should
	// be set based on the expected latency of watch events.
	//
	// Currently a controller can service (create *and* observe the watch events for said
	// creation) about 10 pods a second, so it takes about 1 min to service
	// 500 pods. Just creation is limited to 20qps, and watching happens with ~10-30s
	// latency/pod at the scale of 3000 pods over 100 nodes.
	ExpectationsTimeout = 5 * time.Minute
)

// Expectations are a way for controllers to tell the controller manager what they expect. eg:
//	ControllerExpectations: {
//		controller1: expects  2 adds in 2 minutes
//		controller2: expects  2 dels in 2 minutes
//		controller3: expects -1 adds in 2 minutes => controller3's expectations have already been met
//	}
//
// Implementation:
//	ControlleeExpectation = pair of atomic counters to track controllee's creation/deletion
//	ControllerExpectationsStore = TTLStore + a ControlleeExpectation per controller
//
// * Once set expectations can only be lowered
// * A controller isn't synced till its expectations are either fulfilled, or expire
// * Controllers that don't set expectations will get woken up for every matching controllee

// KeyFunc to parse out the key from a ReplicaSet
var KeyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc

// BindingKey returns a key unique to the given bindign within a cluster.
// It's used so we consistently use the same key scheme in this module.
// It does exactly what cache.MetaNamespaceKeyFunc would have done
// except there's not possibility for error since we know the exact type.
func BindingKey(binding *schedulingv1alpha1.NetworkFunctionBinding) string {
	return fmt.Sprintf("%v/%v", binding.Namespace, binding.Name)
}

// ExpKeyFunc to parse out the key from a ControlleeExpectation
var ExpKeyFunc = func(obj interface{}) (string, error) {
	if e, ok := obj.(*ControlleeExpectations); ok {
		return e.key, nil
	}
	return "", fmt.Errorf("could not find key for obj %#v", obj)
}

// ControllerExpectationsInterface is an interface that allows users to set and wait on expectations.
// Only abstracted out for testing.
// Warning: if using KeyFunc it is not safe to use a single ControllerExpectationsInterface with different
// types of controllers, because the keys might conflict across types.
type ControllerExpectationsInterface interface {
	GetExpectations(controllerKey string) (*ControlleeExpectations, bool, error)
	SatisfiedExpectations(logger klog.Logger, controllerKey string) bool
	DeleteExpectations(logger klog.Logger, controllerKey string)
	SetExpectations(logger klog.Logger, controllerKey string, add, del int) error
	ExpectCreations(logger klog.Logger, controllerKey string, adds int) error
	ExpectDeletions(logger klog.Logger, controllerKey string, dels int) error
	CreationObserved(logger klog.Logger, controllerKey string)
	DeletionObserved(logger klog.Logger, controllerKey string)
	RaiseExpectations(logger klog.Logger, controllerKey string, add, del int)
	LowerExpectations(logger klog.Logger, controllerKey string, add, del int)
}

// ControllerExpectations is a cache mapping controllers to what they expect to see before being woken up for a sync.
type ControllerExpectations struct {
	cache.Store
}

// GetExpectations returns the ControlleeExpectations of the given controller.
func (r *ControllerExpectations) GetExpectations(controllerKey string) (*ControlleeExpectations, bool, error) {
	exp, exists, err := r.GetByKey(controllerKey)
	if err == nil && exists {
		return exp.(*ControlleeExpectations), true, nil
	}
	return nil, false, err
}

// DeleteExpectations deletes the expectations of the given controller from the TTLStore.
func (r *ControllerExpectations) DeleteExpectations(logger klog.Logger, controllerKey string) {
	if exp, exists, err := r.GetByKey(controllerKey); err == nil && exists {
		if err := r.Delete(exp); err != nil {

			logger.V(2).Info("Error deleting expectations", "controller", controllerKey, "err", err)
		}
	}
}

// SatisfiedExpectations returns true if the required adds/dels for the given controller have been observed.
// Add/del counts are established by the controller at sync time, and updated as controllees are observed by the controller
// manager.
func (r *ControllerExpectations) SatisfiedExpectations(logger klog.Logger, controllerKey string) bool {
	if exp, exists, err := r.GetExpectations(controllerKey); exists {
		if exp.Fulfilled() {
			logger.V(4).Info("Controller expectations fulfilled", "expectations", exp)
			return true
		} else if exp.isExpired() {
			logger.V(4).Info("Controller expectations expired", "expectations", exp)
			return true
		} else {
			logger.V(4).Info("Controller still waiting on expectations", "expectations", exp)
			return false
		}
	} else if err != nil {
		logger.V(2).Info("Error encountered while checking expectations, forcing sync", "err", err)
	} else {
		// When a new controller is created, it doesn't have expectations.
		// When it doesn't see expected watch events for > TTL, the expectations expire.
		//	- In this case it wakes up, creates/deletes controllees, and sets expectations again.
		// When it has satisfied expectations and no controllees need to be created/destroyed > TTL, the expectations expire.
		//	- In this case it continues without setting expectations till it needs to create/delete controllees.
		logger.V(4).Info("Controller either never recorded expectations, or the ttl expired", "controller", controllerKey)
	}
	// Trigger a sync if we either encountered and error (which shouldn't happen since we're
	// getting from local store) or this controller hasn't established expectations.
	return true
}

// SetExpectations registers new expectations for the given controller. Forgets existing expectations.
func (r *ControllerExpectations) SetExpectations(logger klog.Logger, controllerKey string, add, del int) error {
	exp := &ControlleeExpectations{add: int64(add), del: int64(del), key: controllerKey, timestamp: clock.RealClock{}.Now()}
	logger.V(4).Info("Setting expectations", "expectations", exp)
	return r.Add(exp)
}

func (r *ControllerExpectations) ExpectCreations(logger klog.Logger, controllerKey string, adds int) error {
	return r.SetExpectations(logger, controllerKey, adds, 0)
}

func (r *ControllerExpectations) ExpectDeletions(logger klog.Logger, controllerKey string, dels int) error {
	return r.SetExpectations(logger, controllerKey, 0, dels)
}

// LowerExpectations decrements the expectation counts of the given controller.
func (r *ControllerExpectations) LowerExpectations(logger klog.Logger, controllerKey string, add, del int) {
	if exp, exists, err := r.GetExpectations(controllerKey); err == nil && exists {
		exp.Add(int64(-add), int64(-del))
		// The expectations might've been modified since the update on the previous line.
		logger.V(4).Info("Lowered expectations", "expectations", exp)
	}
}

// RaiseExpectations increments the expectation counts of the given controller.
func (r *ControllerExpectations) RaiseExpectations(logger klog.Logger, controllerKey string, add, del int) {
	if exp, exists, err := r.GetExpectations(controllerKey); err == nil && exists {
		exp.Add(int64(add), int64(del))
		// The expectations might've been modified since the update on the previous line.
		logger.V(4).Info("Raised expectations", "expectations", exp)
	}
}

// CreationObserved atomically decrements the `add` expectation count of the given controller.
func (r *ControllerExpectations) CreationObserved(logger klog.Logger, controllerKey string) {
	r.LowerExpectations(logger, controllerKey, 1, 0)
}

// DeletionObserved atomically decrements the `del` expectation count of the given controller.
func (r *ControllerExpectations) DeletionObserved(logger klog.Logger, controllerKey string) {
	r.LowerExpectations(logger, controllerKey, 0, 1)
}

// NewControllerExpectations returns a store for ControllerExpectations.
func NewControllerExpectations() *ControllerExpectations {
	return &ControllerExpectations{cache.NewStore(ExpKeyFunc)}
}

// UIDSetKeyFunc to parse out the key from a UIDSet.
var UIDSetKeyFunc = func(obj interface{}) (string, error) {
	if u, ok := obj.(*UIDSet); ok {
		return u.key, nil
	}
	return "", fmt.Errorf("could not find key for obj %#v", obj)
}

// UIDSet holds a key and a set of UIDs. Used by the
// UIDTrackingControllerExpectations to remember which UID it has seen/still
// waiting for.
type UIDSet struct {
	sets.Set[string]
	key string
}

// UIDTrackingControllerExpectations tracks the UID of the pods it deletes.
// This cache is needed over plain old expectations to safely handle graceful
// deletion. The desired behavior is to treat an update that sets the
// DeletionTimestamp on an object as a delete. To do so consistently, one needs
// to remember the expected deletes so they aren't double counted.
// TODO: Track creates as well (#22599)
type UIDTrackingControllerExpectations struct {
	ControllerExpectationsInterface
	// TODO: There is a much nicer way to do this that involves a single store,
	//   a lock per entry, and a ControlleeExpectationsInterface type.
	uidStoreLock sync.Mutex
	// Store used for the UIDs associated with any expectation tracked via the
	// ControllerExpectationsInterface.
	uidStore cache.Store
}

// GetUIDs is a convenience method to avoid exposing the set of expected uids.
// The returned set is not thread safe, all modifications must be made holding
// the uidStoreLock.
func (u *UIDTrackingControllerExpectations) GetUIDs(controllerKey string) sets.Set[string] {
	if uid, exists, err := u.uidStore.GetByKey(controllerKey); err == nil && exists {
		return uid.(*UIDSet).Set
	}
	return nil
}

// ExpectDeletions records expectations for the given deleteKeys, against the given controller.
func (u *UIDTrackingControllerExpectations) ExpectDeletions(logger klog.Logger, rcKey string, deletedKeys []string) error {
	expectedUIDs := sets.New[string]()
	for _, k := range deletedKeys {
		expectedUIDs.Insert(k)
	}
	logger.V(4).Info("Controller waiting on deletions", "controller", rcKey, "keys", deletedKeys)
	u.uidStoreLock.Lock()
	defer u.uidStoreLock.Unlock()

	if existing := u.GetUIDs(rcKey); existing != nil && existing.Len() != 0 {
		logger.Error(nil, "Clobbering existing delete keys", "keys", existing)
	}
	if err := u.uidStore.Add(&UIDSet{expectedUIDs, rcKey}); err != nil {
		return err
	}
	return u.ControllerExpectationsInterface.ExpectDeletions(logger, rcKey, expectedUIDs.Len())
}

// DeletionObserved records the given deleteKey as a deletion, for the given rc.
func (u *UIDTrackingControllerExpectations) DeletionObserved(logger klog.Logger, rcKey, deleteKey string) {
	u.uidStoreLock.Lock()
	defer u.uidStoreLock.Unlock()

	uids := u.GetUIDs(rcKey)
	if uids != nil && uids.Has(deleteKey) {
		logger.V(4).Info("Controller received delete for pod", "controller", rcKey, "key", deleteKey)
		u.ControllerExpectationsInterface.DeletionObserved(logger, rcKey)
		uids.Delete(deleteKey)
	}
}

// DeleteExpectations deletes the UID set and invokes DeleteExpectations on the
// underlying ControllerExpectationsInterface.
func (u *UIDTrackingControllerExpectations) DeleteExpectations(logger klog.Logger, rcKey string) {
	u.uidStoreLock.Lock()
	defer u.uidStoreLock.Unlock()

	u.ControllerExpectationsInterface.DeleteExpectations(logger, rcKey)
	if uidExp, exists, err := u.uidStore.GetByKey(rcKey); err == nil && exists {
		if err := u.uidStore.Delete(uidExp); err != nil {
			logger.V(2).Info("Error deleting uid expectations", "controller", rcKey, "err", err)
		}
	}
}

// NewUIDTrackingControllerExpectations returns a wrapper around
// ControllerExpectations that is aware of deleteKeys.
func NewUIDTrackingControllerExpectations(ce ControllerExpectationsInterface) *UIDTrackingControllerExpectations {
	return &UIDTrackingControllerExpectations{ControllerExpectationsInterface: ce, uidStore: cache.NewStore(UIDSetKeyFunc)}
}

// ControlleeExpectations track controllee creates/deletes.
type ControlleeExpectations struct {
	// Important: Since these two int64 fields are using sync/atomic, they have to be at the top of the struct due to a bug on 32-bit platforms
	// See: https://golang.org/pkg/sync/atomic/ for more information
	add       int64
	del       int64
	key       string
	timestamp time.Time
}

// Add increments the add and del counters.
func (e *ControlleeExpectations) Add(add, del int64) {
	atomic.AddInt64(&e.add, add)
	atomic.AddInt64(&e.del, del)
}

// Fulfilled returns true if this expectation has been fulfilled.
func (e *ControlleeExpectations) Fulfilled() bool {
	// TODO: think about why this line being atomic doesn't matter
	return atomic.LoadInt64(&e.add) <= 0 && atomic.LoadInt64(&e.del) <= 0
}

// GetExpectations returns the add and del expectations of the controllee.
func (e *ControlleeExpectations) GetExpectations() (int64, int64) {
	return atomic.LoadInt64(&e.add), atomic.LoadInt64(&e.del)
}

// MarshalLog makes a thread-safe copy of the values of the expectations that
// can be used for logging.
func (e *ControlleeExpectations) MarshalLog() interface{} {
	return struct {
		add int64
		del int64
		key string
	}{
		add: atomic.LoadInt64(&e.add),
		del: atomic.LoadInt64(&e.del),
		key: e.key,
	}
}

// TODO: Extend ExpirationCache to support explicit expiration.
// TODO: Make this possible to disable in tests.
// TODO: Support injection of clock.
func (e *ControlleeExpectations) isExpired() bool {
	return clock.RealClock{}.Since(e.timestamp) > ExpectationsTimeout
}
