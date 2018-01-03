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
// */

package attachment

import (
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type MockAttachment struct {
	MockCreate func(volumeAttachment *rookalpha.VolumeAttachment) error
	MockGet    func(namespace, name string) (*rookalpha.VolumeAttachment, error)
	MockList   func(namespace string) (*rookalpha.VolumeAttachmentList, error)
	MockUpdate func(volumeAttachment *rookalpha.VolumeAttachment) error
	MockDelete func(namespace, name string) error
}

func (m *MockAttachment) Create(volumeAttachment *rookalpha.VolumeAttachment) error {
	if m.MockCreate != nil {
		return m.MockCreate(volumeAttachment)
	}
	return nil
}
func (m *MockAttachment) Get(namespace, name string) (*rookalpha.VolumeAttachment, error) {
	if m.MockGet != nil {
		return m.MockGet(namespace, name)
	}
	return &rookalpha.VolumeAttachment{}, nil
}

func (m *MockAttachment) List(namespace string) (*rookalpha.VolumeAttachmentList, error) {
	if m.MockList != nil {
		return m.MockList(namespace)
	}
	return &rookalpha.VolumeAttachmentList{}, nil
}

func (m *MockAttachment) Update(volumeAttachment *rookalpha.VolumeAttachment) error {
	if m.MockUpdate != nil {
		return m.MockUpdate(volumeAttachment)
	}
	return nil
}

func (m *MockAttachment) Delete(namespace, name string) error {
	if m.MockDelete != nil {
		return m.MockDelete(namespace, name)
	}
	return nil
}

// Adds the list of known types to api.Scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(rookalpha.SchemeGroupVersion,
		&rookalpha.VolumeAttachment{},
		&rookalpha.VolumeAttachmentList{},
	)
	metav1.AddToGroupVersion(scheme, rookalpha.SchemeGroupVersion)
	return nil
}
