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

package client

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateClient(t *testing.T) {
	context := &clusterd.Context{Executor: &exectest.MockExecutor{}}

	// must specify caps
	p := cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client1", Namespace: "myns"}}
	err := ValidateClient(context, &p)
	assert.NotNil(t, err)

	// must specify name
	p = cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Namespace: "myns"}}
	err = ValidateClient(context, &p)
	assert.NotNil(t, err)

	// must specify namespace
	p = cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client1"}}
	err = ValidateClient(context, &p)
	assert.NotNil(t, err)

	// succeed with caps properly defined
	p = cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client1", Namespace: "myns"}}
	p.Spec.Caps = map[string]string{
		"osd": "allow *",
		"mon": "allow *",
		"mds": "allow *",
	}
	err = ValidateClient(context, &p)
	assert.Nil(t, err)
}

func TestGenerateClient(t *testing.T) {
	clientset := testop.New(1)
	context := &clusterd.Context{Clientset: clientset}

	p := &cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client1", Namespace: "myns"},
		Spec: cephv1.ClientSpec{
			Caps: map[string]string{
				"osd": "allow *",
				"mon": "allow rw",
				"mds": "allow rwx",
			},
		},
	}

	client, caps, err := genClientEntity(p, context)
	equal := bytes.Compare([]byte(client), []byte("client.client1"))
	var res bool = equal == 0
	assert.True(t, res)
	assert.True(t, strings.Contains(strings.Join(caps, " "), "osd allow *"))
	assert.True(t, strings.Contains(strings.Join(caps, " "), "mon allow rw"))
	assert.True(t, strings.Contains(strings.Join(caps, " "), "mds allow rwx"))
	assert.Nil(t, err)

	// Fail if caps are empty
	p2 := &cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client2", Namespace: "myns"},
		Spec: cephv1.ClientSpec{
			Caps: map[string]string{
				"osd": "",
				"mon": "",
			},
		},
	}

	client, _, err = genClientEntity(p2, context)
	equal = bytes.Compare([]byte(client), []byte("client.client2"))
	res = equal == 0
	assert.True(t, res)
	assert.NotNil(t, err)
}

func TestCreateClient(t *testing.T) {
	clientset := testop.New(1)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command, outfileArg string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if command == "ceph" && args[1] == "get-or-create-key" {
				return `{"key":"AQC7ilJdAPijOBAABp+YAzg2QupRAWdnIh7w/Q=="}`, nil
			}
			return "", errors.Errorf("unexpected ceph command '%v'", args)
		},
	}
	context := &clusterd.Context{Executor: executor, Clientset: clientset}

	p := &cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client1", Namespace: "myns"},
		Spec: cephv1.ClientSpec{
			Caps: map[string]string{
				"osd": "allow *",
				"mon": "allow *",
				"mds": "allow *",
			},
		},
	}

	exists, _ := clientExists(context, p)
	assert.False(t, exists)
	err := createClient(context, p)
	assert.Nil(t, err)

	// fail if caps are empty
	p = &cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client1", Namespace: "myns"},
		Spec: cephv1.ClientSpec{
			Caps: map[string]string{
				"osd": "",
				"mon": "",
				"mds": "",
			},
		},
	}
	exists, _ = clientExists(context, p)
	assert.False(t, exists)
	err = createClient(context, p)
	assert.NotNil(t, err)
}

func TestUpdateClient(t *testing.T) {
	clientset := testop.New(1)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command, outfileArg string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			return "", nil
		},
	}
	context := &clusterd.Context{Executor: executor, Clientset: clientset}
	new := &cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client1", Namespace: "myns"},
		Spec: cephv1.ClientSpec{
			Caps: map[string]string{
				"osd": "allow *",
				"mon": "allow *",
			},
		},
	}

	err := updateClient(context, new)
	assert.Nil(t, err)

	new = &cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client1", Namespace: "myns"},
		Spec: cephv1.ClientSpec{
			Caps: map[string]string{
				"osd": "allow rw",
				"mon": "allow rw",
			},
		},
	}

	err = updateClient(context, new)
	assert.Nil(t, err)
}

func TestDeleteClient(t *testing.T) {
	clientset := testop.New(2)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command, outfileArg string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if command == "ceph" && args[1] == "get-key" && args[2] == "client1" {
				return `{"key":"AQC7ilJdAPijOBAABp+YAzg2QupRAWdnIh7w/Q=="}`, nil
			}
			if command == "ceph" && args[1] == "del" {
				return "updated", nil
			}
			return "", errors.Errorf("unexpected ceph command '%v'", args)
		},
	}
	context := &clusterd.Context{Executor: executor, Clientset: clientset}

	// delete a client that exists
	p := &cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client1", Namespace: "myns"}}
	exists, err := clientExists(context, p)
	assert.Nil(t, err)
	assert.True(t, exists)
	err = deleteClient(context, p)
	assert.Nil(t, err)

	// succeed even if the client doesn't exist
	p = &cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client2", Namespace: "myns"}}
	exists, err = clientExists(context, p)
	assert.NotNil(t, err)
	assert.False(t, exists)
	err = deleteClient(context, p)
	assert.Nil(t, err)
}
