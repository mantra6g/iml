// From the kubernetes repo, all props go to them
/*
Copyright 2015 The Kubernetes Authors.

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

package nfgc

import (
	"github.com/mantra6g/iml/api/core/v1alpha1"
)

// byEvictionAndCreationTimestamp sorts a list by Evicted status and then creation timestamp,
// using their names as a tie breaker.
// Evicted nfs will be deleted first to avoid impact on terminated nfs created by controllers.
type byEvictionAndCreationTimestamp []*v1alpha1.NetworkFunction

func (o byEvictionAndCreationTimestamp) Len() int      { return len(o) }
func (o byEvictionAndCreationTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o byEvictionAndCreationTimestamp) Less(i, j int) bool {
	iEvicted, jEvicted := NFIsEvicted(o[i].Status), NFIsEvicted(o[j].Status)
	// Evicted pod is smaller
	if iEvicted != jEvicted {
		return iEvicted
	}
	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].CreationTimestamp.Before(&o[j].CreationTimestamp)
}

// NFIsEvicted returns true if the reported nf status is due to an eviction.
func NFIsEvicted(podStatus v1alpha1.NetworkFunctionStatus) bool {
	return podStatus.Phase == v1alpha1.NetworkFunctionFailed && podStatus.Reason == v1alpha1.NetworkFunctionReasonEvicted
}
