// /*
// Copyright 2017 The Rook Authors. All rights reserved.

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

// 	http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Some of the code below came from https://github.com/coreos/etcd-operator
// which also has the apache 2.0 license.
// */

package crd

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/api"
)

func RegisterFakeAPI() *runtime.Scheme {
	scheme := runtime.NewScheme()
	api.SchemeBuilder.AddToScheme(scheme)
	api.SchemeBuilder.Register(addKnownTypes)
	return scheme
}

type MockVolumeAttachmentController struct {
	MockCreate func(volumeAttachment VolumeAttachment) error
	MockGet    func(namespace, name string) (VolumeAttachment, error)
	MockList   func(namespace string) (VolumeAttachmentList, error)
	MockUpdate func(volumeAttachment VolumeAttachment) error
	MockDelete func(namespace, name string) error
}

func (m *MockVolumeAttachmentController) Create(volumeAttachment VolumeAttachment) error {
	if m.MockCreate != nil {
		return m.MockCreate(volumeAttachment)
	}
	return nil
}
func (m *MockVolumeAttachmentController) Get(namespace, name string) (VolumeAttachment, error) {
	if m.MockGet != nil {
		return m.MockGet(namespace, name)
	}
	return VolumeAttachment{}, nil
}

func (m *MockVolumeAttachmentController) List(namespace string) (VolumeAttachmentList, error) {
	if m.MockList != nil {
		return m.MockList(namespace)
	}
	return VolumeAttachmentList{}, nil
}

func (m *MockVolumeAttachmentController) Update(volumeAttachment VolumeAttachment) error {
	if m.MockUpdate != nil {
		return m.MockUpdate(volumeAttachment)
	}
	return nil
}

func (m *MockVolumeAttachmentController) Delete(namespace, name string) error {
	if m.MockDelete != nil {
		return m.MockDelete(namespace, name)
	}
	return nil
}
