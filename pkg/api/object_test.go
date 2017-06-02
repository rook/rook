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
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	cephtest "github.com/rook/rook/pkg/ceph/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/util"
	testexec "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/proc"
	"github.com/stretchr/testify/assert"
)

func TestCreateObjectStoreHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient}}

	req, err := http.NewRequest("POST", "http://10.0.0.100/objectstore", nil)
	if err != nil {
		logger.Fatal(err)
	}

	// call the CreateObjectStore handler, which should return http 202 Accepted and record info
	// about the file system request in etcd
	w := httptest.NewRecorder()
	h := newTestHandler(context)
	h.CreateObjectStore(w, req)
	assert.Equal(t, http.StatusAccepted, w.Code)
	assert.Equal(t, "1", etcdClient.GetValue("/rook/services/ceph/object/desired/state"))
}

func TestRemoveObjectStoreHandler(t *testing.T) {
	// simulate object store already being installed by setting the desired key in etcd
	etcdClient := util.NewMockEtcdClient()
	etcdClient.SetValue("/rook/services/ceph/object/desired/state", "1")

	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient}}

	req, err := http.NewRequest("DELETE", "http://10.0.0.100/objectstore", nil)
	if err != nil {
		logger.Fatal(err)
	}

	// call RemoveObjectStore handler and verify the response is 202 Accepted and the desired
	// key has been deleted from etcd
	w := httptest.NewRecorder()
	h := newTestHandler(context)
	h.RemoveObjectStore(w, req)
	assert.Equal(t, http.StatusAccepted, w.Code)
	assert.Equal(t, 0, etcdClient.GetChildDirs("/rook/services/ceph/object/desired").Count())
}

func TestGetObjectStoreConnectionInfoHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	inventory.SetIPAddress(etcdClient, "123", "1.2.3.4", "2.3.4.5")
	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient}}

	req, err := http.NewRequest("GET", "http://10.0.0.100/objectstore/connectioninfo", nil)
	if err != nil {
		logger.Fatal(err)
	}

	// before RGW has been installed or any user accounts have been created, the handler will return 404 not found
	w := httptest.NewRecorder()
	h := newTestHandler(context)
	h.GetObjectStoreConnectionInfo(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// simulate RGW being installed and the built
	etcdClient.SetValue("/rook/services/ceph/rgw/applied/node/123", "")

	w = httptest.NewRecorder()
	h = newTestHandler(context)
	h.GetObjectStoreConnectionInfo(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	expectedRespObj := model.ObjectStoreConnectInfo{
		Host:       "rook-rgw:53390",
		IPEndpoint: "1.2.3.4:53390",
	}

	// unmarshal the http response to get the actual object and compare it to the expected object
	var actualResultObj model.ObjectStoreConnectInfo
	bodyBytes, _ := ioutil.ReadAll(w.Body)
	json.Unmarshal(bodyBytes, &actualResultObj)
	assert.Equal(t, expectedRespObj, actualResultObj)
}

func TestListUsers(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	req, err := http.NewRequest("GET", "http://10.0.0.100/objectstore/users", nil)
	if err != nil {
		logger.Fatal(err)
	}

	defer os.RemoveAll("/tmp/rgw")
	cephtest.CreateClusterInfo(etcdClient, []string{"mymon"})

	runTest := func(runner func(args ...string) (string, error)) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(command string, subcommand string, args ...string) (string, error) { return runner(args...) }}
		context := &clusterd.Context{
			DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
			Executor:      executor,
			ProcMan:       proc.New(executor),
			ConfigDir:     "/tmp/rgw"}
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		h.ListUsers(w, req)
		return w
	}

	// Empty list
	w := runTest(func(args ...string) (string, error) {
		assert.Equal(t, []string{"user", "list", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring"}, args[3:])
		return "[]", nil
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[]", w.Body.String())

	// One item
	w = runTest(func(args ...string) (string, error) {
		if args[4] == "list" {
			assert.Equal(t, []string{"user", "list", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring"}, args[3:])
			return `["testuser"]`, nil
		}
		assert.Equal(t, []string{"user", "info", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "testuser"}, args[3:])
		return `{"user_id":"testuser","display_name":"Test User","email":"testuser@example.com","keys":[{"access_key":"somekey","secret_key":"somesecret","user":"testuser"}]}`, nil
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[{"userId":"testuser","displayName":"Test User","email":"testuser@example.com","accessKey":"somekey","secretKey":"somesecret"}]`, w.Body.String())

	// Two items
	first := true
	w = runTest(func(args ...string) (string, error) {
		if args[4] == "list" {
			assert.Equal(t, []string{"user", "list", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring"}, args[3:])
			return `["testuser","otheruser"]`, nil
		}

		if first {
			first = false
			assert.Equal(t, []string{"user", "info", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "testuser"}, args[3:])
			return `{"user_id":"testuser","display_name":"Test User","email":"testuser@example.com","keys":[]}`, nil
		}
		assert.Equal(t, []string{"user", "info", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "otheruser"}, args[3:])
		return `{"user_id":"otheruser","display_name":"Other User","keys":[{"access_key":"otherkey","secret_key":"othersecret","user":"otheruser"},{"access_key":"we","secret_key":"foo","user":"bar"}]}`, nil
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[{"userId":"testuser","displayName":"Test User","email":"testuser@example.com","accessKey":null,"secretKey":null},{"userId":"otheruser","displayName":"Other User","email":"","accessKey":"otherkey","secretKey":"othersecret"}]`, w.Body.String())

	// Error getting users list
	w = runTest(func(args ...string) (string, error) {
		assert.Equal(t, []string{"user", "list", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring"}, args[3:])
		return "", fmt.Errorf("some error")
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Error getting user
	w = runTest(func(args ...string) (string, error) {
		if args[4] == "list" {
			assert.Equal(t, []string{"user", "list", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring"}, args[3:])
			return `["testuser"]`, nil
		}
		return "", fmt.Errorf("some error")
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// User list no parseable
	w = runTest(func(args ...string) (string, error) {
		assert.Equal(t, []string{"user", "list", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring"}, args[3:])
		return "[bad ,format", nil
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func TestGetUser(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	req, err := http.NewRequest("GET", "http://10.0.0.100/objectstore/users/someuser", nil)
	if err != nil {
		logger.Fatal(err)
	}

	defer os.RemoveAll("/tmp/rgw")
	cephtest.CreateClusterInfo(etcdClient, []string{"mymon"})

	runTest := func(s string, e error, expectedArgs ...string) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(command string, subcommand string, args ...string) (string, error) {
			assert.Equal(t, args[3:], expectedArgs)
			return s, e
		}}
		context := &clusterd.Context{
			DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
			ConfigDir:     "/tmp/rgw",
			Executor:      executor,
			ProcMan:       proc.New(executor),
		}
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		r := newRouter(h.GetRoutes())

		r.ServeHTTP(w, req)

		return w
	}

	// Error getting user
	w := runTest("", fmt.Errorf("some error"), "user", "info", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "someuser")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Get user with no keys
	w = runTest(`{"user_id":"testuser","display_name":"Test User","email":"testuser@example.com","keys":[]}`, nil, "user", "info", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "someuser")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"userId":"testuser","displayName":"Test User","email":"testuser@example.com","accessKey":null,"secretKey":null}`, w.Body.String())

	// Get user with one key
	w = runTest(`{"user_id":"testuser","display_name":"Test User","email":"testuser@example.com","keys":[{"access_key":"otherkey","secret_key":"othersecret","user":"otheruser"},{"access_key":"we","secret_key":"foo","user":"bar"}]}`, nil, "user", "info", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "someuser")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"userId":"testuser","displayName":"Test User","email":"testuser@example.com","accessKey":"otherkey","secretKey":"othersecret"}`, w.Body.String())

	// Get user with multiple keys
	w = runTest(`{"user_id":"testuser","display_name":"Test User","email":"testuser@example.com","keys":[{"access_key":"we","secret_key":"foo","user":"bar"}]}`, nil, "user", "info", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "someuser")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"userId":"testuser","displayName":"Test User","email":"testuser@example.com","accessKey":"we","secretKey":"foo"}`, w.Body.String())

	// Get user that does not exist
	w = runTest("could not fetch user info: no user info saved", nil, "user", "info", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "someuser")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Unable to parse user json
	w = runTest("[bad, format", nil, "user", "info", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "someuser")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func TestCreateUser(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	req, err := http.NewRequest("POST", "http://10.0.0.100/objectstore/users", nil)
	if err != nil {
		logger.Fatal(err)
	}

	defer os.RemoveAll("/tmp/rgw")
	cephtest.CreateClusterInfo(etcdClient, []string{"mymon"})

	runTest := func(body string, s string, e error, expectedArgs ...string) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(command string, subcommand string, args ...string) (string, error) {
			assert.Equal(t, args[3:], expectedArgs)
			return s, e
		}}
		context := &clusterd.Context{
			DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
			ConfigDir:     "/tmp/rgw",
			ProcMan:       proc.New(executor),
			Executor:      executor,
		}
		req.Body = ioutil.NopCloser(bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		r := newRouter(h.GetRoutes())

		r.ServeHTTP(w, req)

		return w
	}

	// Empty body
	w := runTest("", "", nil, "", "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "", w.Body.String())

	// User id empty
	w = runTest("{}", "", nil, "", "")
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, "userId cannot be empty", w.Body.String())

	// No display name
	w = runTest(`{"userId":"foo"}`, "", nil, "", "")
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, "displayName is required", w.Body.String())

	// Error creating
	w = runTest(`{"userId":"foo","displayName":"the foo"}`, "", fmt.Errorf("some error"), "user", "create", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "foo", "--display-name", "the foo")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// UserID already exists
	w = runTest(`{"userId":"foo","displayName":"the foo"}`, "could not create user: unable to create user, user: foo exists", nil, "user", "create", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "foo", "--display-name", "the foo")
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, "user already exists", w.Body.String())

	// Email already exists
	w = runTest(`{"userId":"foo","displayName":"the foo","email":"test@example.com"}`, "could not create user: unable to create user, email: test@example.com is the email address an existing user", nil, "user", "create", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "foo", "--display-name", "the foo", "--email", "test@example.com")
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, "email already in use", w.Body.String())

	// Success without email
	w = runTest(`{"userId":"foo","displayName":"the foo"}`, `{"user_id":"foo","display_name":"the foo","keys":[{"secret_key":"sk","access_key":"ak"}]}`, nil, "user", "create", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "foo", "--display-name", "the foo")
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, `{"userId":"foo","displayName":"the foo","email":"","accessKey":"ak","secretKey":"sk"}`, w.Body.String())

	// Success with email
	w = runTest(`{"userId":"foo","displayName":"the foo","email":"test@example.com"}`, `{"user_id":"foo","display_name":"the foo","email":"test@example.com","keys":[{"secret_key":"sk","access_key":"ak"}]}`, nil, "user", "create", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "foo", "--display-name", "the foo", "--email", "test@example.com")
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, `{"userId":"foo","displayName":"the foo","email":"test@example.com","accessKey":"ak","secretKey":"sk"}`, w.Body.String())
}

func TestUpdateUser(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	req, err := http.NewRequest("PUT", "http://10.0.0.100/objectstore/users/foo", nil)
	if err != nil {
		logger.Fatal(err)
	}

	defer os.RemoveAll("/tmp/rgw")
	cephtest.CreateClusterInfo(etcdClient, []string{"mymon"})

	runTest := func(body string, s string, e error, expectedArgs ...string) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(command string, subcommand string, args ...string) (string, error) {
			assert.Equal(t, args[3:], expectedArgs)
			return s, e
		}}
		context := &clusterd.Context{
			DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
			ConfigDir:     "/tmp/rgw",
			ProcMan:       proc.New(executor),
			Executor:      executor,
		}
		req.Body = ioutil.NopCloser(bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		r := newRouter(h.GetRoutes())

		r.ServeHTTP(w, req)

		return w
	}

	// Empty body
	w := runTest("", "", nil, "", "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Error updating user
	w = runTest("{}", "", fmt.Errorf("some error"), "user", "modify", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "foo")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// User not found
	w = runTest("{}", "could not modify user: unable to modify user, user not found", nil, "user", "modify", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "foo")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Success with display name
	w = runTest(`{"displayName":"different name"}`, `{"user_id":"foo","display_name":"different name","email":"test@example.com","keys":[{"secret_key":"sk","access_key":"ak"},{"secret_key":"ok","access_key":"bk"}]}`, nil, "user", "modify", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "foo", "--display-name", "different name")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"userId":"foo","displayName":"different name","email":"test@example.com","accessKey":"ak","secretKey":"sk"}`, w.Body.String())

	// Success with email
	w = runTest(`{"email":"different@example.com"}`, `{"user_id":"foo","display_name":"old name","email":"different@example.com","keys":[{"secret_key":"sk","access_key":"ak"},{"secret_key":"ok","access_key":"bk"}]}`, nil, "user", "modify", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "foo", "--email", "different@example.com")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"userId":"foo","displayName":"old name","email":"different@example.com","accessKey":"ak","secretKey":"sk"}`, w.Body.String())

	// Success with display name and email
	w = runTest(`{"displayName":"different name","email":"different@example.com"}`, `{"user_id":"foo","display_name":"different name","email":"different@example.com","keys":[{"secret_key":"sk","access_key":"ak"},{"secret_key":"ok","access_key":"bk"}]}`, nil, "user", "modify", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "foo", "--display-name", "different name", "--email", "different@example.com")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"userId":"foo","displayName":"different name","email":"different@example.com","accessKey":"ak","secretKey":"sk"}`, w.Body.String())
}

func TestDeleteUser(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	req, err := http.NewRequest("DELETE", "http://10.0.0.100/objectstore/users/foo", nil)
	if err != nil {
		logger.Fatal(err)
	}

	defer os.RemoveAll("/tmp/rgw")
	cephtest.CreateClusterInfo(etcdClient, []string{"mymon"})

	runTest := func(s string, e error, expectedArgs ...string) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(command string, subcommand string, args ...string) (string, error) {
			assert.Equal(t, args[3:], expectedArgs)
			return s, e
		}}
		context := &clusterd.Context{
			DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
			ConfigDir:     "/tmp/rgw",
			ProcMan:       proc.New(executor),
			Executor:      executor,
		}
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		r := newRouter(h.GetRoutes())

		r.ServeHTTP(w, req)

		return w
	}

	// Some error
	w := runTest("", fmt.Errorf("some error"), "user", "rm", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "foo")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// User not found
	w = runTest("unable to remove user, user does not exist", nil, "user", "rm", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "foo")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Success
	w = runTest("", nil, "user", "rm", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--uid", "foo")
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func TestListBuckets(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	req, err := http.NewRequest("GET", "http://10.0.0.100/objectstore/buckets", nil)
	if err != nil {
		logger.Fatal(err)
	}

	defer os.RemoveAll("/tmp/rgw")
	cephtest.CreateClusterInfo(etcdClient, []string{"mymon"})

	runTest := func(runner func(args ...string) (string, error)) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(command string, subcommand string, args ...string) (string, error) { return runner(args...) }}
		context := &clusterd.Context{
			DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
			ConfigDir:     "/tmp/rgw",
			ProcMan:       proc.New(executor),
			Executor:      executor,
		}
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		h.ListBuckets(w, req)
		return w
	}

	// List error
	w := runTest(func(args ...string) (string, error) {
		assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring"}, args[3:])
		return "", fmt.Errorf("some error")
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Empty list
	w = runTest(func(args ...string) (string, error) {
		assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring"}, args[3:])
		return "[]", nil
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[]", w.Body.String())

	// Bad list format
	w = runTest(func(args ...string) (string, error) {
		assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring"}, args[3:])
		return "[bad, format", nil
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	oneStat := `[{"bucket":"foo","usage":{"pool1":{"size":4,"num_objects":2},"pool2":{"size":5,"num_objects":4}}}]`

	// error getting metadata
	first := true
	w = runTest(func(args ...string) (string, error) {
		if first {
			first = false
			assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring"}, args[3:])
			return oneStat, nil
		}
		assert.Equal(t, []string{"metadata", "get", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "bucket:foo"}, args[3:])
		return "", fmt.Errorf("some error")
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Bad metadata format
	first = true
	w = runTest(func(args ...string) (string, error) {
		if first {
			first = false
			assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring"}, args[3:])
			return oneStat, nil
		}
		assert.Equal(t, []string{"metadata", "get", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "bucket:foo"}, args[3:])
		return "[bad, format", nil
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Bad date format
	first = true
	w = runTest(func(args ...string) (string, error) {
		if first {
			first = false
			assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring"}, args[3:])
			return oneStat, nil
		}
		assert.Equal(t, []string{"metadata", "get", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "bucket:foo"}, args[3:])
		return `{"data":{"owner":"bob","creation_time":"fds"}}`, nil
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Success
	first = true
	w = runTest(func(args ...string) (string, error) {
		if first {
			first = false
			assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring"}, args[3:])
			return oneStat, nil
		}
		assert.Equal(t, []string{"metadata", "get", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "bucket:foo"}, args[3:])
		return `{"data":{"owner":"bob","creation_time":"2016-08-05 16:23:34.343343Z"}}`, nil
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[{"name":"foo","owner":"bob","createdAt":"2016-08-05T16:23:34.343343Z","size":9,"numberOfObjects":6}]`, w.Body.String())

	// Two buckets
	first = true
	w = runTest(func(args ...string) (string, error) {
		if first {
			// Expect the stats call
			first = false
			assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring"}, args[3:])
			return `[{"bucket":"foo","usage":{"pool1":{"size":4,"num_objects":2}}},{"bucket":"bar","usage":{"pool2":{"size":5,"num_objects":4}}}]`, nil
		} else {
			// Expect the bucket metadata calls
			if args[len(args)-1] == "bucket:foo" {
				assert.Equal(t, []string{"metadata", "get", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "bucket:foo"}, args[3:])
				return `{"data":{"owner":"bob","creation_time":"2016-08-05 16:23:34.343343Z"}}`, nil
			} else if args[len(args)-1] == "bucket:bar" {
				assert.Equal(t, []string{"metadata", "get", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "bucket:bar"}, args[3:])
				return `{"data":{"owner":"bill","creation_time":"2016-08-05 18:31:22.445343Z"}}`, nil
			} else {
				assert.Fail(t, "Wasn't foo or bar: %+v", args[3:])
			}
		}
		assert.Fail(t, "Shouldn't return more than 3 times")
		return "", fmt.Errorf("Shouldn't return more than 3 times")
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[{"name":"bar","owner":"bill","createdAt":"2016-08-05T18:31:22.445343Z","size":5,"numberOfObjects":4},{"name":"foo","owner":"bob","createdAt":"2016-08-05T16:23:34.343343Z","size":4,"numberOfObjects":2}]`, w.Body.String())
}

func TestGetBucket(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	req, err := http.NewRequest("GET", "http://10.0.0.100/objectstore/buckets/test", nil)
	if err != nil {
		logger.Fatal(err)
	}

	defer os.RemoveAll("/tmp/rgw")
	cephtest.CreateClusterInfo(etcdClient, []string{"mymon"})

	runTest := func(runner func(args ...string) (string, error)) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(command string, subcommand string, args ...string) (string, error) { return runner(args...) }}
		context := &clusterd.Context{
			DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
			ConfigDir:     "/tmp/rgw",
			ProcMan:       proc.New(executor),
			Executor:      executor,
		}
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		r := newRouter(h.GetRoutes())

		r.ServeHTTP(w, req)

		return w
	}

	// Stats fails
	w := runTest(func(args ...string) (string, error) {
		assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--bucket", "test"}, args[3:])
		return "", fmt.Errorf("some error")
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// stats not found
	w = runTest(func(args ...string) (string, error) {
		assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--bucket", "test"}, args[3:])
		return `2017-03-07 09:07:30.868797 c269240  0 could not get bucket info for bucket=tesdsft
2017-03-07 09:07:30.868797 c269240  0 could not get bucket info for bucket=tesdsft`, nil
	})
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Error parsing stats
	w = runTest(func(args ...string) (string, error) {
		assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--bucket", "test"}, args[3:])
		return "{", nil
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// metadata fail
	w = runTest(func(args ...string) (string, error) {
		if args[4] == "stats" {
			assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--bucket", "test"}, args[3:])
			return "{}", nil
		} else {
			assert.Equal(t, []string{"metadata", "get", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "bucket:test"}, args[3:])
			return "", fmt.Errorf("some error")
		}
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// metadata not found
	w = runTest(func(args ...string) (string, error) {
		if args[4] == "stats" {
			assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--bucket", "test"}, args[3:])
			return "{}", nil
		} else {
			assert.Equal(t, []string{"metadata", "get", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "bucket:test"}, args[3:])
			return "ERROR: can't get key: (2) No such file or directory", nil
		}
	})
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "", w.Body.String())

	// metadata parse fail
	w = runTest(func(args ...string) (string, error) {
		if args[4] == "stats" {
			assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--bucket", "test"}, args[3:])
			return "{}", nil
		} else {
			assert.Equal(t, []string{"metadata", "get", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "bucket:test"}, args[3:])
			return "{", nil
		}
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Success
	w = runTest(func(args ...string) (string, error) {
		if args[4] == "stats" {
			assert.Equal(t, []string{"bucket", "stats", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--bucket", "test"}, args[3:])
			return `{"bucket":"test","usage":{"pool2":{"size":5,"num_objects":4}}}`, nil
		} else {
			assert.Equal(t, []string{"metadata", "get", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "bucket:test"}, args[3:])
			return `{"data":{"owner":"bill","creation_time":"2016-08-05 18:31:22.445343Z"}}`, nil
		}
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "{\"name\":\"test\",\"owner\":\"bill\",\"createdAt\":\"2016-08-05T18:31:22.445343Z\",\"size\":5,\"numberOfObjects\":4}", w.Body.String())
}

func TestBucketDelete(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	req, err := http.NewRequest("DELETE", "http://10.0.0.100/objectstore/buckets/test", nil)
	if err != nil {
		logger.Fatal(err)
	}

	defer os.RemoveAll("/tmp/rgw")
	cephtest.CreateClusterInfo(etcdClient, []string{"mymon"})

	runTest := func(s string, e error, expectedArgs ...string) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(command string, subcommand string, args ...string) (string, error) {
			assert.Equal(t, args[3:], expectedArgs)
			return s, e
		}}
		context := &clusterd.Context{
			DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
			ConfigDir:     "/tmp/rgw",
			Executor:      executor,
			ProcMan:       proc.New(executor),
		}
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		r := newRouter(h.GetRoutes())

		r.ServeHTTP(w, req)

		return w
	}

	// errors
	w := runTest("", fmt.Errorf("some error"), "bucket", "rm", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--bucket", "test")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Not found
	w = runTest(`2017-03-07 09:36:45.605774 c081240  0 could not get bucket info for bucket=tesdsft
2017-03-07 09:36:45.605774 c081240  0 could not get bucket info for bucket=tesdsft`, nil, "bucket", "rm", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--bucket", "test")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Not found
	w = runTest("unexpected content", nil, "bucket", "rm", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--bucket", "test")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Succeeds
	w = runTest("", nil, "bucket", "rm", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--bucket", "test")
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "", w.Body.String())

	// not Purge
	req, err = http.NewRequest("DELETE", "http://10.0.0.100/objectstore/buckets/test?purge=false", nil)
	if err != nil {
		logger.Fatal(err)
	}
	w = runTest("", nil, "bucket", "rm", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--bucket", "test")
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "", w.Body.String())

	//  Purge
	req, err = http.NewRequest("DELETE", "http://10.0.0.100/objectstore/buckets/test?purge=true", nil)
	if err != nil {
		logger.Fatal(err)
	}
	w = runTest("", nil, "bucket", "rm", "--cluster=default", "--conf=/tmp/rgw/tmp/default.config", "--keyring=/tmp/rgw/tmp/keyring", "--bucket", "test", "--purge-objects")
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "", w.Body.String())
}
