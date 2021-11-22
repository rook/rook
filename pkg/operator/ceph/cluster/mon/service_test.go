/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package mon

import (
	"context"
	"sync"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateService(t *testing.T) {
	ctx := context.TODO()
	clientset := test.New(t, 1)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", cephv1.ClusterSpec{}, &k8sutil.OwnerInfo{}, &sync.Mutex{})
	c.ClusterInfo = client.AdminTestClusterInfo("rook-ceph")
	m := &monConfig{ResourceName: "rook-ceph-mon-b", DaemonName: "b"}
	clusterIP, err := c.createService(m)
	assert.NoError(t, err)
	// the clusterIP will not be set in a mock service
	assert.Equal(t, "", clusterIP)

	m.PublicIP = "1.2.3.4"
	clusterIP, err = c.createService(m)
	assert.NoError(t, err)
	// the clusterIP will not be set in the mock because the service already exists
	assert.Equal(t, "", clusterIP)

	// delete the service to mock a disaster recovery scenario
	err = clientset.CoreV1().Services(c.Namespace).Delete(ctx, m.ResourceName, metav1.DeleteOptions{})
	assert.NoError(t, err)

	clusterIP, err = c.createService(m)
	assert.NoError(t, err)
	// the clusterIP will now be set to the expected value
	assert.Equal(t, m.PublicIP, clusterIP)
}
