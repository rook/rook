/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package csi

import (
	"testing"

	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/test"

	"github.com/stretchr/testify/assert"
)

func TestStartCSI(t *testing.T) {
	CSIParam = Param{
		CSIPluginImage:   "image",
		RegistrarImage:   "image",
		ProvisionerImage: "image",
		AttacherImage:    "image",
		SnapshotterImage: "image",
	}
	clientset := test.New(t, 3)
	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookfake.NewSimpleClientset(),
	}
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		assert.Nil(t, err)
	}
	AllowUnsupported = true
	err = startDrivers(context.Clientset, context.RookClientset, "ns", serverVersion, nil, nil)
	assert.Nil(t, err)
}
