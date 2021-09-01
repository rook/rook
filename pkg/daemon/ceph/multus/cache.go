/*
Copyright 2022 The Rook Authors. All rights reserved.

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

package multus

import "github.com/pkg/errors"

// MovedInterfaceInfo contains information about the interface that was moved from the holder pod's
// multus network namespace to the host's network namespace. This information is critical to
// understand which interfaces have been moved in order for them to be un-migrated.
type MovedInterfaceInfo struct {
	// NetIface is the name of the moved interface in the host net namespace.
	NetIface string
}

// MovedInterfaceCache is a cache to keep track of which holder pods have had their interfaces
// migrated by the mover.
type MovedInterfaceCache struct {
	cache map[string]MovedInterfaceInfo
}

func NewMovedInterfaceCache() *MovedInterfaceCache {
	return &MovedInterfaceCache{
		cache: map[string]MovedInterfaceInfo{},
	}
}

func (c *MovedInterfaceCache) Exists(holderPodName string) bool {
	_, ok := c.cache[holderPodName]
	return ok
}

func (c *MovedInterfaceCache) Get(holderPodName string) (MovedInterfaceInfo, error) {
	info, ok := c.cache[holderPodName]
	if !ok {
		return MovedInterfaceInfo{}, errors.Errorf("info does not exist for holder pod %q in cache", holderPodName)
	}
	return info, nil
}

func (c *MovedInterfaceCache) Add(holderPodName string, info MovedInterfaceInfo) {
	if c.Exists(holderPodName) {
		logger.Infof("overwriting existing moved interface info for holder pod %q in cache", holderPodName)
	}
	c.cache[holderPodName] = info
}

func (c *MovedInterfaceCache) Remove(holderPodName string) {
	if !c.Exists(holderPodName) {
		logger.Infof("moved interface info for holder pod %q already removed from cache", holderPodName)
	}
	delete(c.cache, holderPodName)
}

func (c *MovedInterfaceCache) AsMap() map[string]MovedInterfaceInfo {
	ret := map[string]MovedInterfaceInfo{}
	for pod, info := range c.cache {
		ret[pod] = info
	}
	return ret
}
