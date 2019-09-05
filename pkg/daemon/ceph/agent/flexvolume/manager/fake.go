/*
Copyright 2017 The Rook Authors. All rights reserved.

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

package manager

import "fmt"

// FakeVolumeManager represents a fake (mocked) implementation of the VolumeManager interface for testing.
type FakeVolumeManager struct {
	FakeInit   func() error
	FakeAttach func(image, pool, id, key, clusterName string) (string, error)
	FakeDetach func(image, pool, clusterName string, force bool) error
	FakeExpand func(image, pool, clusterName string, size uint64) error
}

// Init initializes the FakeVolumeManager
func (f *FakeVolumeManager) Init() error {
	if f.FakeInit != nil {
		return f.FakeInit()
	}
	return nil
}

// Attach a volume image to the node
func (f *FakeVolumeManager) Attach(image, pool, id, key, clusterName string) (string, error) {
	if f.FakeAttach != nil {
		return f.FakeAttach(image, pool, id, key, clusterName)
	}
	return fmt.Sprintf("/%s/%s/%s", image, pool, clusterName), nil
}

// Detach a volume image from a node
func (f *FakeVolumeManager) Detach(image, pool, id, key, clusterName string, force bool) error {
	if f.FakeDetach != nil {
		return f.FakeDetach(image, pool, clusterName, force)
	}
	return nil
}

func (f *FakeVolumeManager) Expand(image, pool, clusterName string, size uint64) error {
	if f.FakeExpand != nil {
		return f.FakeExpand(image, pool, clusterName, size)
	}
	return nil
}
