/*
Copyright 2019 The Kubernetes Authors.

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
	"sync"
	"time"

	corev1alpha1 "github.com/mantra6g/iml/api/core/v1alpha1"
)

// TriggerTimeTracker is used to compute an EndpointsLastChangeTriggerTime
// annotation. See the documentation for that annotation for more details.
//
// Please note that this util may compute a wrong EndpointsLastChangeTriggerTime
// if the same object changes multiple times between two consecutive syncs.
// We're aware of this limitation but we decided to accept it, as fixing it
// would require a major rewrite of the endpoint(Slice) controller and
// Informer framework. Such situations, i.e. frequent updates of the same object
// in a single sync period, should be relatively rare and therefore this util
// should provide a good approximation of the EndpointsLastChangeTriggerTime.
type TriggerTimeTracker struct {
	// NetServiceStates is a map, indexed by NetService object key, storing the last
	// known NetworkService object state observed during the most recent call of the
	// ComputeEndpointLastChangeTriggerTime function.
	NetServiceStates map[NetServiceKey]NetServiceState

	// mutex guarding the NetServiceStates map.
	mutex sync.Mutex
}

// NewTriggerTimeTracker creates a new instance of the TriggerTimeTracker.
func NewTriggerTimeTracker() *TriggerTimeTracker {
	return &TriggerTimeTracker{
		NetServiceStates: make(map[NetServiceKey]NetServiceState),
	}
}

// NetServiceKey is a key uniquely identifying a NetworkService.
type NetServiceKey struct {
	// namespace, name composing a namespaced name - an unique identifier of every NetworkService.
	Namespace, Name string
}

// NetServiceState represents a state of a NetworkService object that is known to this util.
type NetServiceState struct {
	// lastServiceTriggerTime is a service trigger time observed most recently.
	lastServiceTriggerTime time.Time
	// lastNFTriggerTimes is a map (NF name -> time) storing the network function trigger
	// times that were observed during the most recent call of the
	// ComputeEndpointLastChangeTriggerTime function.
	lastNFTriggerTimes map[string]time.Time
}

// ComputeEndpointLastChangeTriggerTime updates the state of the NetworkService/Endpoint
// object being synced and returns the time that should be exported as the
// EndpointsLastChangeTriggerTime annotation.
//
// If the method returns a 'zero' time the EndpointsLastChangeTriggerTime
// annotation shouldn't be exported.
//
// Please note that this function may compute a wrong value if the same object
// (nf/ns) changes multiple times between two consecutive syncs.
//
// Important: This method is go-routing safe but only when called for different
// keys. The method shouldn't be called concurrently for the same key! This
// contract is fulfilled in the current implementation of the endpoint(slice)
// controller.
func (t *TriggerTimeTracker) ComputeEndpointLastChangeTriggerTime(
	namespace string, ns *corev1alpha1.NetworkService, nfs []corev1alpha1.NetworkFunction) time.Time {

	key := NetServiceKey{Namespace: namespace, Name: ns.Name}
	// As there won't be any concurrent calls for the same key, we need to guard
	// access only to the serviceStates map.
	t.mutex.Lock()
	state, wasKnown := t.NetServiceStates[key]
	t.mutex.Unlock()

	// Update the state before returning.
	defer func() {
		t.mutex.Lock()
		t.NetServiceStates[key] = state
		t.mutex.Unlock()
	}()

	// minChangedTriggerTime is the min trigger time of all trigger times that
	// have changed since the last sync.
	var minChangedTriggerTime time.Time
	nfTriggerTimes := make(map[string]time.Time)
	for i := range nfs {
		nf := &nfs[i]
		if nfTriggerTime := getNFTriggerTime(nf); !nfTriggerTime.IsZero() {
			nfTriggerTimes[nf.Name] = nfTriggerTime
			if nfTriggerTime.After(state.lastNFTriggerTimes[nf.Name]) {
				// Nf trigger time has changed since the last sync, update minChangedTriggerTime.
				minChangedTriggerTime = min(minChangedTriggerTime, nfTriggerTime)
			}
		}
	}
	nsTriggerTime := getNetworkServiceTriggerTime(ns)
	if nsTriggerTime.After(state.lastServiceTriggerTime) {
		// Service trigger time has changed since the last sync, update minChangedTriggerTime.
		minChangedTriggerTime = min(minChangedTriggerTime, nsTriggerTime)
	}

	state.lastNFTriggerTimes = nfTriggerTimes
	state.lastServiceTriggerTime = nsTriggerTime

	if !wasKnown {
		// New Service, use Service creationTimestamp.
		return ns.CreationTimestamp.Time
	}

	// Regular update of endpoint objects, return min of changed trigger times.
	return minChangedTriggerTime
}

// DeleteNetworkService deletes network service state stored in this util.
func (t *TriggerTimeTracker) DeleteNetworkService(namespace, name string) {
	key := NetServiceKey{Namespace: namespace, Name: name}
	t.mutex.Lock()
	defer t.mutex.Unlock()
	delete(t.NetServiceStates, key)
}

// getNFTriggerTime returns the time of the pod change (trigger) that resulted
// or will result in the endpoint object change.
func getNFTriggerTime(nf *corev1alpha1.NetworkFunction) (triggerTime time.Time) {
	if readyCondition := getNFReadyCondition(&nf.Status); readyCondition != nil {
		triggerTime = readyCondition.LastTransitionTime.Time
	}
	return triggerTime
}

// getNetworkServiceTriggerTime returns the time of the network service change (trigger) that
// resulted or will result in the endpoint change.
func getNetworkServiceTriggerTime(ns *corev1alpha1.NetworkService) (triggerTime time.Time) {
	return ns.CreationTimestamp.Time
}

// min returns minimum of the currentMin and newValue or newValue if the currentMin is not set.
func min(currentMin, newValue time.Time) time.Time {
	if currentMin.IsZero() || newValue.Before(currentMin) {
		return newValue
	}
	return currentMin
}
