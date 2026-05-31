/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package operator

import (
	"fmt"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
)

func TestOperator(t *testing.T) {
	clientset := test.New(t, 3)
	context := &clusterd.Context{Clientset: clientset}
	o := New(context, "", "")

	assert.NotNil(t, o)
	assert.NotNil(t, o.clusterController)
	assert.NotNil(t, o.resources)
	assert.Equal(t, context, o.context)
	assert.Equal(t, 1, len(o.resources))
	for _, r := range o.resources {
		if r.Name != opcontroller.ClusterResource.Name {
			assert.Fail(t, fmt.Sprintf("Resource %s is not valid", r.Name))
		}
	}
}
