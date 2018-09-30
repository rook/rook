/*
Copyright 2017 The Kubernetes Authors.

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

package persistentvolume

import (
	"sync"

	"k8s.io/api/core/v1"
)

// podBindingCache stores PV binding decisions per pod per node.
// Pod entries are removed when the Pod is deleted or updated to
// no longer be schedulable.
type PodBindingCache interface {
	// UpdateBindings will update the cache with the given bindings for the
	// pod and node.
	UpdateBindings(pod *v1.Pod, node string, bindings []*bindingInfo)

	// GetBindings will return the cached bindings for the given pod and node.
	GetBindings(pod *v1.Pod, node string) []*bindingInfo

	// UpdateProvisionedPVCs will update the cache with the given provisioning decisions
	// for the pod and node.
	UpdateProvisionedPVCs(pod *v1.Pod, node string, provisionings []*v1.PersistentVolumeClaim)

	// GetProvisionedPVCs will return the cached provisioning decisions for the given pod and node.
	GetProvisionedPVCs(pod *v1.Pod, node string) []*v1.PersistentVolumeClaim

	// DeleteBindings will remove all cached bindings and provisionings for the given pod.
	// TODO: separate the func if it is needed to delete bindings/provisionings individually
	DeleteBindings(pod *v1.Pod)
}

type podBindingCache struct {
	mutex sync.Mutex

	// Key = pod name
	// Value = nodeDecisions
	bindingDecisions map[string]nodeDecisions
}

// Key = nodeName
// Value = bindings & provisioned PVCs of the node
type nodeDecisions map[string]nodeDecision

// A decision includes bindingInfo and provisioned PVCs of the node
type nodeDecision struct {
	bindings      []*bindingInfo
	provisionings []*v1.PersistentVolumeClaim
}

func NewPodBindingCache() PodBindingCache {
	return &podBindingCache{bindingDecisions: map[string]nodeDecisions{}}
}

func (c *podBindingCache) DeleteBindings(pod *v1.Pod) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	podName := getPodName(pod)
	delete(c.bindingDecisions, podName)
}

func (c *podBindingCache) UpdateBindings(pod *v1.Pod, node string, bindings []*bindingInfo) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	podName := getPodName(pod)
	decisions, ok := c.bindingDecisions[podName]
	if !ok {
		decisions = nodeDecisions{}
		c.bindingDecisions[podName] = decisions
	}
	decision, ok := decisions[node]
	if !ok {
		decision = nodeDecision{
			bindings: bindings,
		}
	} else {
		decision.bindings = bindings
	}
	decisions[node] = decision
}

func (c *podBindingCache) GetBindings(pod *v1.Pod, node string) []*bindingInfo {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	podName := getPodName(pod)
	decisions, ok := c.bindingDecisions[podName]
	if !ok {
		return nil
	}
	decision, ok := decisions[node]
	if !ok {
		return nil
	}
	return decision.bindings
}

func (c *podBindingCache) UpdateProvisionedPVCs(pod *v1.Pod, node string, pvcs []*v1.PersistentVolumeClaim) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	podName := getPodName(pod)
	decisions, ok := c.bindingDecisions[podName]
	if !ok {
		decisions = nodeDecisions{}
		c.bindingDecisions[podName] = decisions
	}
	decision, ok := decisions[node]
	if !ok {
		decision = nodeDecision{
			provisionings: pvcs,
		}
	} else {
		decision.provisionings = pvcs
	}
	decisions[node] = decision
}

func (c *podBindingCache) GetProvisionedPVCs(pod *v1.Pod, node string) []*v1.PersistentVolumeClaim {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	podName := getPodName(pod)
	decisions, ok := c.bindingDecisions[podName]
	if !ok {
		return nil
	}
	decision, ok := decisions[node]
	if !ok {
		return nil
	}
	return decision.provisionings
}
