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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/
package operator

import (
	"fmt"
	"os"
	"testing"

	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
	testclient "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
)

func TestCreateSecrets(t *testing.T) {
	clientset := testop.New(3)
	info := testop.CreateClusterInfo(1)
	factory := &testclient.MockConnectionFactory{Fsid: "fsid", SecretKey: "mysecret"}
	conn, _ := factory.NewConnWithClusterAndUser(info.Name, "user")
	conn.(*testceph.MockConnection).MockMonCommand = func(buf []byte) (buffer []byte, info string, err error) {
		response := "{\"key\":\"mysecurekey\"}"
		return []byte(response), "", nil
	}
	spec := ClusterSpec{Namespace: "ns", Version: "myversion"}
	c := newCluster(spec, factory, clientset)
	c.dataDir = "/tmp/testdir"
	defer os.RemoveAll(c.dataDir)

	err := c.createClientAccess(info)
	assert.Nil(t, err)

	secretName := fmt.Sprintf("%s-rbd-user", spec.Namespace)
	secret, err := clientset.CoreV1().Secrets(k8sutil.DefaultNamespace).Get(secretName)
	assert.Nil(t, err)
	assert.Equal(t, secretName, secret.Name)
	assert.Equal(t, 1, len(secret.StringData))
}
