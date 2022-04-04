/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package peermap

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"strings"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAddClusterIDMapping(t *testing.T) {
	clusterMap1 := map[string]string{"cluster-1": "cluster-2"}
	m := &PeerIDMappings{}
	m.addClusterIDMapping(clusterMap1)
	assert.Equal(t, 1, len(*m))

	// Add same cluster map again and assert that it didn't get added.
	m.addClusterIDMapping(clusterMap1)
	assert.Equal(t, 1, len(*m))

	// Add new cluster map and assert that it got added.
	clusterMap2 := map[string]string{"cluster-1": "cluster-3"}
	m.addClusterIDMapping(clusterMap2)
	assert.Equal(t, 2, len(*m))
}

func TestUpdateClusterPoolIDMap(t *testing.T) {
	m := &PeerIDMappings{}

	// Ensure only local:peer-1 mapping should be present
	newMappings := PeerIDMapping{
		ClusterIDMapping: map[string]string{"local": "peer-1"},
		RBDPoolIDMapping: []map[string]string{{"1": "2"}},
	}
	m.updateRBDPoolIDMapping(newMappings)
	assert.Equal(t, len(*m), 1)
	assert.Equal(t, (*m)[0].ClusterIDMapping["local"], "peer-1")
	assert.Equal(t, len((*m)[0].RBDPoolIDMapping), 1)
	assert.Equal(t, (*m)[0].RBDPoolIDMapping[0]["1"], "2")

	// Ensure RBD pool ID mappings get updated
	newMappings = PeerIDMapping{
		ClusterIDMapping: map[string]string{"local": "peer-1"},
		RBDPoolIDMapping: []map[string]string{{"1": "3"}},
	}
	m.updateRBDPoolIDMapping(newMappings)
	assert.Equal(t, len(*m), 1)
	assert.Equal(t, (*m)[0].ClusterIDMapping["local"], "peer-1")
	assert.Equal(t, len((*m)[0].RBDPoolIDMapping), 1)
	assert.Equal(t, (*m)[0].RBDPoolIDMapping[0]["1"], "3")

	// Ensure that new pool ID mappings got added
	newMappings = PeerIDMapping{
		ClusterIDMapping: map[string]string{"local": "peer-1"},
		RBDPoolIDMapping: []map[string]string{{"2": "4"}},
	}
	m.updateRBDPoolIDMapping(newMappings)
	assert.Equal(t, len(*m), 1)
	assert.Equal(t, (*m)[0].ClusterIDMapping["local"], "peer-1")
	assert.Equal(t, len((*m)[0].RBDPoolIDMapping), 2)
	assert.Equal(t, (*m)[0].RBDPoolIDMapping[0]["1"], "3")
	assert.Equal(t, (*m)[0].RBDPoolIDMapping[1]["2"], "4")

	// Ensure that new pool ID mappings got added
	newMappings = PeerIDMapping{
		ClusterIDMapping: map[string]string{"local": "peer-1"},
		RBDPoolIDMapping: []map[string]string{{"3": "5"}},
	}
	m.updateRBDPoolIDMapping(newMappings)
	assert.Equal(t, len(*m), 1)
	assert.Equal(t, (*m)[0].ClusterIDMapping["local"], "peer-1")
	assert.Equal(t, len((*m)[0].RBDPoolIDMapping), 3)
	assert.Equal(t, (*m)[0].RBDPoolIDMapping[0]["1"], "3")
	assert.Equal(t, (*m)[0].RBDPoolIDMapping[1]["2"], "4")
	assert.Equal(t, (*m)[0].RBDPoolIDMapping[2]["3"], "5")

	// Ensure that new Cluster ID mappings got added
	newMappings = PeerIDMapping{
		ClusterIDMapping: map[string]string{"local": "peer-2"},
		RBDPoolIDMapping: []map[string]string{{"1": "3"}},
	}
	m.updateRBDPoolIDMapping(newMappings)
	assert.Equal(t, len(*m), 2)
	assert.Equal(t, (*m)[0].ClusterIDMapping["local"], "peer-1")
	assert.Equal(t, len((*m)[0].RBDPoolIDMapping), 3)
	assert.Equal(t, (*m)[0].RBDPoolIDMapping[0]["1"], "3")
	assert.Equal(t, (*m)[0].RBDPoolIDMapping[1]["2"], "4")
	assert.Equal(t, (*m)[0].RBDPoolIDMapping[2]["3"], "5")

	assert.Equal(t, (*m)[1].ClusterIDMapping["local"], "peer-2")
	assert.Equal(t, len((*m)[1].RBDPoolIDMapping), 1)
	assert.Equal(t, (*m)[0].RBDPoolIDMapping[0]["1"], "3")
}

func TestAddPoolIDMapping(t *testing.T) {
	clusterMap1 := map[string]string{"cluster-1": "cluster-2"}
	m := &PeerIDMappings{}
	m.addClusterIDMapping(clusterMap1)
	assert.Equal(t, 1, len(*m))

	// Add two Pool ID mapping
	poolIDMap1 := map[string]string{"1": "2"}
	poolIDMap2 := map[string]string{"2": "3"}

	m.addRBDPoolIDMapping(clusterMap1, poolIDMap1)
	m.addRBDPoolIDMapping(clusterMap1, poolIDMap2)

	assert.Equal(t, 2, len((*m)[0].RBDPoolIDMapping))

	// Add another cluster ID mapping
	clusterMap2 := map[string]string{"cluster-1": "cluster-3"}
	m.addClusterIDMapping(clusterMap2)

	// Add one Pool ID mapping
	poolIDMap3 := map[string]string{"2": "4"}
	m.addRBDPoolIDMapping(clusterMap2, poolIDMap3)

	// Assert total of two mappings are added
	assert.Equal(t, 2, len(*m))

	// Assert two pool ID mappings are available for first cluster mapping
	assert.Equal(t, 2, len((*m)[0].RBDPoolIDMapping))

	// Assert one pool ID mapping is available for second cluster mapping
	assert.Equal(t, 1, len((*m)[1].RBDPoolIDMapping))
}

const (
	ns = "rook-ceph-primary"
)

//nolint:gosec // fake token for peer cluster "peer1"
var fakeTokenPeer1 = "eyJmc2lkIjoiOWY1MjgyZGItYjg5OS00NTk2LTgwOTgtMzIwYzFmYzM5NmYzIiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFBUnczOWQwdkhvQmhBQVlMM1I4RmR5dHNJQU50bkFTZ0lOTVE9PSIsIm1vbl9ob3N0IjoiW3YyOjE5Mi4xNjguMS4zOjY4MjAsdjE6MTkyLjE2OC4xLjM6NjgyMV0iLCAibmFtZXNwYWNlIjogInBlZXIxIn0="

//nolint:gosec // fake token for peer cluster "peer2"
var fakeTokenPeer2 = "eyJmc2lkIjoiOWY1MjgyZGItYjg5OS00NTk2LTgwOTgtMzIwYzFmYzM5NmYzIiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFBUnczOWQwdkhvQmhBQVlMM1I4RmR5dHNJQU50bkFTZ0lOTVE9PSIsIm1vbl9ob3N0IjoiW3YyOjE5Mi4xNjguMS4zOjY4MjAsdjE6MTkyLjE2OC4xLjM6NjgyMV0iLCAibmFtZXNwYWNlIjogInBlZXIyIn0="

//nolint:gosec // fake token for peer cluster "peer3"
var fakeTokenPeer3 = "eyJmc2lkIjoiOWY1MjgyZGItYjg5OS00NTk2LTgwOTgtMzIwYzFmYzM5NmYzIiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFBUnczOWQwdkhvQmhBQVlMM1I4RmR5dHNJQU50bkFTZ0lOTVE9PSIsIm1vbl9ob3N0IjoiW3YyOjE5Mi4xNjguMS4zOjY4MjAsdjE6MTkyLjE2OC4xLjM6NjgyMV0iLCAibmFtZXNwYWNlIjogInBlZXIzIn0="

var peer1Secret = corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "peer1Secret",
		Namespace: ns,
	},
	Data: map[string][]byte{
		"token": []byte(fakeTokenPeer1),
	},
}

var peer2Secret = corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "peer2Secret",
		Namespace: ns,
	},
	Data: map[string][]byte{
		"token": []byte(fakeTokenPeer2),
	},
}

var peer3Secret = corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "peer3Secret",
		Namespace: ns,
	},
	Data: map[string][]byte{
		"token": []byte(fakeTokenPeer3),
	},
}

var fakeSinglePeerCephBlockPool = cephv1.CephBlockPool{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "mirrorPool1",
		Namespace: ns,
	},
	Spec: cephv1.NamedBlockPoolSpec{
		PoolSpec: cephv1.PoolSpec{
			Mirroring: cephv1.MirroringSpec{
				Peers: &cephv1.MirroringPeerSpec{
					SecretNames: []string{
						"peer1Secret",
					},
				},
			},
		},
	},
}

var fakeMultiPeerCephBlockPool = cephv1.CephBlockPool{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "mirrorPool1",
		Namespace: ns,
	},
	Spec: cephv1.NamedBlockPoolSpec{
		PoolSpec: cephv1.PoolSpec{
			Mirroring: cephv1.MirroringSpec{
				Peers: &cephv1.MirroringPeerSpec{
					SecretNames: []string{
						"peer1Secret",
						"peer2Secret",
						"peer3Secret",
					},
				},
			},
		},
	},
}

var mockExecutor = &exectest.MockExecutor{
	MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		// Fake pool details for "rook-ceph-primary" cluster
		if args[0] == "osd" && args[1] == "pool" && args[2] == "get" && strings.HasSuffix(args[6], ns) {
			if args[3] == "mirrorPool1" {
				return `{"pool_id": 1}`, nil
			} else if args[3] == "mirrorPool2" {
				return `{"pool_id": 2}`, nil
			}

		}
		return "", nil
	},
	MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "osd" && args[1] == "pool" && args[2] == "get" && strings.HasSuffix(args[5], "peer1") {
			if args[3] == "mirrorPool1" {
				return `{"pool_id": 2}`, nil
			} else if args[3] == "mirrorPool2" {
				return `{"pool_id": 3}`, nil
			}
		}
		if args[0] == "osd" && args[1] == "pool" && args[2] == "get" && strings.HasSuffix(args[5], "peer2") {
			if args[3] == "mirrorPool1" {
				return `{"pool_id": 3}`, nil
			} else if args[3] == "mirrorPool2" {
				return `{"pool_id": 4}`, nil
			}
		}
		if args[0] == "osd" && args[1] == "pool" && args[2] == "get" && strings.HasSuffix(args[5], "peer3") {
			if args[3] == "mirrorPool1" {
				return `{"pool_id": 4}`, nil
			} else if args[3] == "mirrorPool2" {
				return `{"pool_id": 5}`, nil
			}
		}
		return "", nil
	},
}

func TestSinglePeerMappings(t *testing.T) {
	clusterInfo := cephclient.AdminTestClusterInfo(ns)
	fakeContext := &clusterd.Context{
		Executor:  mockExecutor,
		Clientset: test.New(t, 3),
	}

	// create fake secret with "peer1" cluster token
	_, err := fakeContext.Clientset.CoreV1().Secrets(ns).Create(context.TODO(), &peer1Secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	//expected: &[{ClusterIDMapping:{peer1:rook-ceph-primary}. RBDPoolIDMapping:[{2:1}]}]
	actualMappings, err := getClusterPoolIDMap(
		fakeContext,
		clusterInfo,
		&fakeSinglePeerCephBlockPool,
	)
	assert.NoError(t, err)
	mappings := *actualMappings
	assert.Equal(t, 1, len(mappings))
	assert.Equal(t, ns, mappings[0].ClusterIDMapping["peer1"])
	assert.Equal(t, "1", mappings[0].RBDPoolIDMapping[0]["2"])
}

func TestMultiPeerMappings(t *testing.T) {
	clusterInfo := cephclient.AdminTestClusterInfo(ns)
	fakeContext := &clusterd.Context{
		Executor:  mockExecutor,
		Clientset: test.New(t, 3),
	}

	// create fake secret with "peer1" cluster token
	_, err := fakeContext.Clientset.CoreV1().Secrets(ns).Create(context.TODO(), &peer1Secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	// create fake secret with "peer2" cluster token
	_, err = fakeContext.Clientset.CoreV1().Secrets(ns).Create(context.TODO(), &peer2Secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	// create fake secret with "peer3" cluster token
	_, err = fakeContext.Clientset.CoreV1().Secrets(ns).Create(context.TODO(), &peer3Secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	actualMappings, err := getClusterPoolIDMap(
		fakeContext,
		clusterInfo,
		&fakeMultiPeerCephBlockPool,
	)
	assert.NoError(t, err)
	mappings := *actualMappings
	/* Expected:
	[
	  {ClusterIDMapping:{peer1:rook-ceph-primary}, RBDPoolIDMapping:[{2:1}]}
	  {ClusterIDMapping:{peer2:rook-ceph-primary}, RBDPoolIDMapping:[{3:1}]}
	  {ClusterIDMapping:map{peer3:rook-ceph-primary} RBDPoolIDMapping:[{4:1}]}
	]
	*/

	assert.Equal(t, 3, len(mappings))

	assert.Equal(t, 1, len(mappings[0].ClusterIDMapping))
	assert.Equal(t, ns, mappings[0].ClusterIDMapping["peer1"])
	assert.Equal(t, "1", mappings[0].RBDPoolIDMapping[0]["2"])

	assert.Equal(t, 1, len(mappings[1].ClusterIDMapping))
	assert.Equal(t, ns, mappings[1].ClusterIDMapping["peer2"])
	assert.Equal(t, "1", mappings[1].RBDPoolIDMapping[0]["3"])

	assert.Equal(t, 1, len(mappings[2].ClusterIDMapping))
	assert.Equal(t, ns, mappings[2].ClusterIDMapping["peer3"])
	assert.Equal(t, "1", mappings[2].RBDPoolIDMapping[0]["4"])
}

func TestDecodePeerToken(t *testing.T) {
	// Valid token
	decodedToken, err := decodePeerToken(fakeTokenPeer1)
	assert.NoError(t, err)
	assert.Equal(t, "peer1", decodedToken.Namespace)

	// Invalid token
	_, err = decodePeerToken("invalidToken")
	assert.Error(t, err)
}

func TestCreateOrUpdateConfig(t *testing.T) {
	os.Setenv("POD_NAME", "rook-ceph-operator")
	defer os.Setenv("POD_NAME", "")
	os.Setenv("POD_NAMESPACE", ns)
	defer os.Setenv("POD_NAMESPACE", "")

	scheme := scheme.Scheme
	err := cephv1.AddToScheme(scheme)
	assert.NoError(t, err)

	err = appsv1.AddToScheme(scheme)
	assert.NoError(t, err)

	err = corev1.AddToScheme(scheme)
	assert.NoError(t, err)

	fakeContext := &clusterd.Context{
		Client:    fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build(),
		Executor:  mockExecutor,
		Clientset: test.New(t, 3),
	}

	// Create fake pod
	_, err = fakeContext.Clientset.CoreV1().Pods(ns).Create(context.TODO(), test.FakeOperatorPod(ns), metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create fake replicaset
	_, err = fakeContext.Clientset.AppsV1().ReplicaSets(ns).Create(context.TODO(), test.FakeReplicaSet(ns), metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create empty ID mapping configMap
	err = CreateOrUpdateConfig(context.TODO(), fakeContext, &PeerIDMappings{})
	assert.NoError(t, err)
	validateConfig(t, fakeContext, PeerIDMappings{})

	// Create ID mapping configMap with data
	actualMappings := &PeerIDMappings{
		{
			ClusterIDMapping: map[string]string{"peer1": ns},
			RBDPoolIDMapping: []map[string]string{
				{
					"2": "1",
				},
			},
		},
	}

	err = CreateOrUpdateConfig(context.TODO(), fakeContext, actualMappings)
	assert.NoError(t, err)
	//validateConfig(t, fakeContext, actualMappings)

	//Update existing mapping config
	mappings := *actualMappings
	mappings = append(mappings, PeerIDMapping{
		ClusterIDMapping: map[string]string{"peer2": ns},
		RBDPoolIDMapping: []map[string]string{
			{
				"3": "1",
			},
		},
	})

	err = CreateOrUpdateConfig(context.TODO(), fakeContext, &mappings)
	assert.NoError(t, err)
	validateConfig(t, fakeContext, mappings)
}

func validateConfig(t *testing.T, c *clusterd.Context, mappings PeerIDMappings) {
	cm := &corev1.ConfigMap{}
	err := c.Client.Get(context.TODO(), types.NamespacedName{Name: mappingConfigName, Namespace: ns}, cm)
	assert.NoError(t, err)

	data := cm.Data[mappingConfigkey]
	expectedMappings, err := toObj(data)

	assert.NoError(t, err)
	assert.True(t, reflect.DeepEqual(mappings, expectedMappings))
}
