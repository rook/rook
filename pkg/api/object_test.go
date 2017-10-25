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
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	testexec "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateObjectStoreHandler(t *testing.T) {
	createdZone := false
	poolInit := true
	executor := &testexec.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			//logger.Infof("Command: %s %v", command, args)
			if args[0] == "auth" {
				return `{"key":"mykey"}`, nil
			}
			if args[1] == "pool" {
				if args[2] == "create" {
					return "", nil
				}
				if args[2] == "set" && args[4] == "size" {
					assert.Equal(t, "1", args[5])
					return "", nil
				}
				if args[2] == "application" {
					poolInit = true
					return "", nil
				}
			}
			return "", fmt.Errorf("unexpected command '%s'", args[0])
		},
		MockExecuteCommandWithCombinedOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			//logger.Infof("Command: %s %v", command, args)
			if args[0] == "realm" && args[1] == "create" {
				return `{"id":"realm"}`, nil
			}
			if args[0] == "zonegroup" && args[1] == "create" {
				return `{"id":"group"}`, nil
			}
			if args[0] == "zone" && args[1] == "create" {
				createdZone = true
				return `{"id":"myzone"}`, nil
			}
			if args[0] == "period" && args[1] == "update" {
				return "", nil
			}

			return "", fmt.Errorf("unexpected combined output command '%s'", args[0])
		},
	}
	w := httptest.NewRecorder()
	h := newTestHandler(&clusterd.Context{Executor: executor})

	// Valid parameters return a 200
	req, err := http.NewRequest("POST", "http://10.0.0.100/objectstore", strings.NewReader(`{"name": "default","dataConfig":{"replicatedConfig":{"size":1}},"metadataConfig":{"replicatedConfig":{"size":1}},"gateway": {"port":1234}}`))
	assert.Nil(t, err)
	h.CreateObjectStore(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, createdZone)
	assert.True(t, poolInit)

	// Invalid parameters return a 400
	req, err = http.NewRequest("POST", "http://10.0.0.100/objectstore", strings.NewReader(`{"name": "default"}`))
	assert.Nil(t, err)
	h.CreateObjectStore(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRemoveObjectStoreHandler(t *testing.T) {
	req, err := http.NewRequest("DELETE", "http://10.0.0.100/objectstore/mystore", nil)
	if err != nil {
		logger.Fatal(err)
	}
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)

	// call RemoveObjectStore handler and verify the response is 202 Accepted
	context := &clusterd.Context{}
	w := httptest.NewRecorder()
	h := newTestHandler(context)
	h.RemoveObjectStore(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetObjectStoreConnectionInfoHandler(t *testing.T) {
	context := &clusterd.Context{}
	req, err := http.NewRequest("GET", "http://10.0.0.100/objectstore/myinst/connectioninfo?name=myinst", nil)
	require.Nil(t, err)

	// before RGW has been installed or any user accounts have been created, the handler will return 404 not found
	w := httptest.NewRecorder()
	h := newTestHandler(context)

	// get the connection info
	r := newRouter(h.GetRoutes())
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// now simulate the rgw service with the kubernetes service
	ns := "default"
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-ceph-rgw-myinst",
			Namespace: ns,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(int(80)),
					Protocol:   v1.ProtocolTCP,
				},
			},
		},
	}
	_, err = context.Clientset.CoreV1().Services(ns).Create(svc)
	assert.Nil(t, err)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	expectedRespObj := model.ObjectStoreConnectInfo{
		Host:  "rook-ceph-rgw-myinst",
		Ports: []int32{80},
	}

	// unmarshal the http response to get the actual object and compare it to the expected object
	var actualResultObj model.ObjectStoreConnectInfo
	bodyBytes, _ := ioutil.ReadAll(w.Body)
	json.Unmarshal(bodyBytes, &actualResultObj)
	assert.Equal(t, expectedRespObj, actualResultObj)
}

func TestListUsers(t *testing.T) {
	req, err := http.NewRequest("GET", "http://10.0.0.100/objectstore/default/users", nil)
	if err != nil {
		logger.Fatal(err)
	}

	runTest := func(runner func(args ...string) (string, error)) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(debug bool, actionName, command string, args ...string) (string, error) {
			return runner(args...)
		}}
		context := &clusterd.Context{Executor: executor}
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		h.ListUsers(w, req)
		return w
	}

	expectedListArgs := []string{"user", "list"}
	expectedInfoArgs := []string{"user", "info", "--uid"}

	// Empty list
	w := runTest(func(args ...string) (string, error) {
		checkArgs(t, args, expectedListArgs)
		return "[]", nil
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[]", w.Body.String())

	// One item
	w = runTest(func(args ...string) (string, error) {
		if args[1] == "list" {
			checkArgs(t, args, expectedListArgs)
			return `["testuser"]`, nil
		}
		checkArgs(t, args, append(expectedInfoArgs, "testuser"))
		return `{"user_id":"testuser","display_name":"Test User","email":"testuser@example.com","keys":[{"access_key":"somekey","secret_key":"somesecret","user":"testuser"}]}`, nil
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[{"userId":"testuser","displayName":"Test User","email":"testuser@example.com","accessKey":"somekey","secretKey":"somesecret"}]`, w.Body.String())

	// Two items
	first := true
	w = runTest(func(args ...string) (string, error) {
		if args[1] == "list" {
			checkArgs(t, args, expectedListArgs)
			return `["testuser","otheruser"]`, nil
		}

		if first {
			first = false
			checkArgs(t, args, append(expectedInfoArgs, "testuser"))
			return `{"user_id":"testuser","display_name":"Test User","email":"testuser@example.com","keys":[]}`, nil
		}
		checkArgs(t, args, append(expectedInfoArgs, "otheruser"))
		return `{"user_id":"otheruser","display_name":"Other User","keys":[{"access_key":"otherkey","secret_key":"othersecret","user":"otheruser"},{"access_key":"we","secret_key":"foo","user":"bar"}]}`, nil
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[{"userId":"testuser","displayName":"Test User","email":"testuser@example.com","accessKey":null,"secretKey":null},{"userId":"otheruser","displayName":"Other User","email":"","accessKey":"otherkey","secretKey":"othersecret"}]`, w.Body.String())

	// Error getting users list
	w = runTest(func(args ...string) (string, error) {
		checkArgs(t, args, expectedListArgs)
		return "", fmt.Errorf("some error")
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Error getting user
	w = runTest(func(args ...string) (string, error) {
		if args[4] == "list" {
			checkArgs(t, args, expectedListArgs)
			return `["testuser"]`, nil
		}
		return "", fmt.Errorf("some error")
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// User list no parseable
	w = runTest(func(args ...string) (string, error) {
		checkArgs(t, args, expectedListArgs)
		return "[bad ,format", nil
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func TestGetUser(t *testing.T) {
	req, err := http.NewRequest("GET", "http://10.0.0.100/objectstore/default/users/someuser", nil)
	if err != nil {
		logger.Fatal(err)
	}

	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)

	runTest := func(s string, e error, expectedArgs ...string) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(debug bool, actionName, command string, args ...string) (string, error) {
			checkArgs(t, args, expectedArgs)
			return s, e
		}}
		context := &clusterd.Context{Executor: executor}
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		r := newRouter(h.GetRoutes())

		r.ServeHTTP(w, req)

		return w
	}

	expectedArgs := []string{"user", "info", "--uid", "someuser"}

	// Error getting user
	w := runTest("", fmt.Errorf("some error"), expectedArgs...)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Get user with no keys
	w = runTest(`{"user_id":"testuser","display_name":"Test User","email":"testuser@example.com","keys":[]}`, nil, expectedArgs...)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"userId":"testuser","displayName":"Test User","email":"testuser@example.com","accessKey":null,"secretKey":null}`, w.Body.String())

	// Get user with one key
	w = runTest(`{"user_id":"testuser","display_name":"Test User","email":"testuser@example.com","keys":[{"access_key":"otherkey","secret_key":"othersecret","user":"otheruser"},{"access_key":"we","secret_key":"foo","user":"bar"}]}`, nil, expectedArgs...)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"userId":"testuser","displayName":"Test User","email":"testuser@example.com","accessKey":"otherkey","secretKey":"othersecret"}`, w.Body.String())

	// Get user with multiple keys
	w = runTest(`{"user_id":"testuser","display_name":"Test User","email":"testuser@example.com","keys":[{"access_key":"we","secret_key":"foo","user":"bar"}]}`, nil, expectedArgs...)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"userId":"testuser","displayName":"Test User","email":"testuser@example.com","accessKey":"we","secretKey":"foo"}`, w.Body.String())

	// Get user that does not exist
	w = runTest("could not fetch user info: no user info saved", nil, expectedArgs...)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Unable to parse user json
	w = runTest("[bad, format", nil, expectedArgs...)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func TestCreateUser(t *testing.T) {
	req, err := http.NewRequest("POST", "http://10.0.0.100/objectstore/default/users", nil)
	if err != nil {
		logger.Fatal(err)
	}

	runTest := func(body string, s string, e error, expectedArgs ...string) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(debug bool, actionName, command string, args ...string) (string, error) {
			checkArgs(t, args, expectedArgs)
			return s, e
		}}
		context := &clusterd.Context{Executor: executor}
		req.Body = ioutil.NopCloser(bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		r := newRouter(h.GetRoutes())

		r.ServeHTTP(w, req)

		return w
	}

	expectedDisplayNameArgs := []string{"user", "create", "--uid", "foo", "--display-name", "the foo"}
	expectedDisplayNameAndEmailArgs := []string{"user", "create", "--uid", "foo", "--display-name", "the foo", "--email", "test@example.com"}

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
	w = runTest(`{"userId":"foo","displayName":"the foo"}`, "", fmt.Errorf("some error"), expectedDisplayNameArgs...)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// UserID already exists
	w = runTest(`{"userId":"foo","displayName":"the foo"}`, "could not create user: unable to create user, user: foo exists", nil, expectedDisplayNameArgs...)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, "user already exists", w.Body.String())

	// Email already exists
	w = runTest(`{"userId":"foo","displayName":"the foo","email":"test@example.com"}`, "could not create user: unable to create user, email: test@example.com is the email address an existing user", nil, expectedDisplayNameAndEmailArgs...)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, "email already in use", w.Body.String())

	// Success without email
	w = runTest(`{"userId":"foo","displayName":"the foo"}`, `{"user_id":"foo","display_name":"the foo","keys":[{"secret_key":"sk","access_key":"ak"}]}`, nil, expectedDisplayNameArgs...)
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, `{"userId":"foo","displayName":"the foo","email":"","accessKey":"ak","secretKey":"sk"}`, w.Body.String())

	// Success with email
	w = runTest(`{"userId":"foo","displayName":"the foo","email":"test@example.com"}`, `{"user_id":"foo","display_name":"the foo","email":"test@example.com","keys":[{"secret_key":"sk","access_key":"ak"}]}`, nil, expectedDisplayNameAndEmailArgs...)
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, `{"userId":"foo","displayName":"the foo","email":"test@example.com","accessKey":"ak","secretKey":"sk"}`, w.Body.String())
}

func TestUpdateUser(t *testing.T) {
	req, err := http.NewRequest("PUT", "http://10.0.0.100/objectstore/default/users/foo", nil)
	if err != nil {
		logger.Fatal(err)
	}

	runTest := func(body string, s string, e error, expectedArgs ...string) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(debug bool, actionName, command string, args ...string) (string, error) {
			checkArgs(t, args, expectedArgs)
			return s, e
		}}
		context := &clusterd.Context{Executor: executor}
		req.Body = ioutil.NopCloser(bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		r := newRouter(h.GetRoutes())

		r.ServeHTTP(w, req)

		return w
	}

	expectedArgs := []string{"user", "modify", "--uid", "foo"}

	// Empty body
	w := runTest("", "", nil, "", "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Error updating user
	w = runTest("{}", "", fmt.Errorf("some error"), expectedArgs...)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// User not found
	w = runTest("{}", "could not modify user: unable to modify user, user not found", nil, expectedArgs...)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Success with display name
	expectedArgs = []string{"user", "modify", "--uid", "foo", "--display-name", "different name"}
	w = runTest(
		`{"displayName":"different name"}`,
		`{"user_id":"foo","display_name":"different name","email":"test@example.com","keys":[{"secret_key":"sk","access_key":"ak"},{"secret_key":"ok","access_key":"bk"}]}`,
		nil,
		expectedArgs...)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"userId":"foo","displayName":"different name","email":"test@example.com","accessKey":"ak","secretKey":"sk"}`, w.Body.String())

	// Success with email
	expectedArgs = []string{"user", "modify", "--uid", "foo", "--email", "different@example.com"}
	w = runTest(
		`{"email":"different@example.com"}`,
		`{"user_id":"foo","display_name":"old name","email":"different@example.com","keys":[{"secret_key":"sk","access_key":"ak"},{"secret_key":"ok","access_key":"bk"}]}`,
		nil,
		expectedArgs...)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"userId":"foo","displayName":"old name","email":"different@example.com","accessKey":"ak","secretKey":"sk"}`, w.Body.String())

	// Success with display name and email
	expectedArgs = []string{"user", "modify", "--uid", "foo", "--display-name", "different name", "--email", "different@example.com"}
	w = runTest(
		`{"displayName":"different name","email":"different@example.com"}`,
		`{"user_id":"foo","display_name":"different name","email":"different@example.com","keys":[{"secret_key":"sk","access_key":"ak"},{"secret_key":"ok","access_key":"bk"}]}`,
		nil,
		expectedArgs...)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"userId":"foo","displayName":"different name","email":"different@example.com","accessKey":"ak","secretKey":"sk"}`, w.Body.String())
}

func TestDeleteUser(t *testing.T) {
	req, err := http.NewRequest("DELETE", "http://10.0.0.100/objectstore/default/users/foo", nil)
	if err != nil {
		logger.Fatal(err)
	}

	runTest := func(s string, e error, expectedArgs ...string) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(debug bool, actionName, command string, args ...string) (string, error) {
			checkArgs(t, args, expectedArgs)
			return s, e
		}}
		context := &clusterd.Context{Executor: executor}
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		r := newRouter(h.GetRoutes())

		r.ServeHTTP(w, req)

		return w
	}

	expectedArgs := []string{"user", "rm", "--uid", "foo"}

	// Some error
	w := runTest("", fmt.Errorf("some error"), expectedArgs...)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// User not found
	w = runTest("unable to remove user, user does not exist", nil, expectedArgs...)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Success
	w = runTest("", nil, expectedArgs...)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func TestListBuckets(t *testing.T) {
	req, err := http.NewRequest("GET", "http://10.0.0.100/objectstore/default/buckets", nil)
	if err != nil {
		logger.Fatal(err)
	}

	runTest := func(runner func(args ...string) (string, error)) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(debug bool, actionName, command string, args ...string) (string, error) {
			return runner(args...)
		}}
		context := &clusterd.Context{Executor: executor}
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		h.ListBuckets(w, req)
		return w
	}

	expectedStatsArgs := []string{"bucket", "stats"}
	expectedMetadataArgs := []string{"metadata", "get", "bucket:foo"}

	// List error
	w := runTest(func(args ...string) (string, error) {
		checkArgs(t, args, expectedStatsArgs)
		return "", fmt.Errorf("some error")
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Empty list
	w = runTest(func(args ...string) (string, error) {
		checkArgs(t, args, expectedStatsArgs)
		return "[]", nil
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[]", w.Body.String())

	// Bad list format
	w = runTest(func(args ...string) (string, error) {
		checkArgs(t, args, expectedStatsArgs)
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
			checkArgs(t, args, expectedStatsArgs)
			return oneStat, nil
		}
		checkArgs(t, args, expectedMetadataArgs)
		return "", fmt.Errorf("some error")
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Bad metadata format
	first = true
	w = runTest(func(args ...string) (string, error) {
		if first {
			first = false
			checkArgs(t, args, expectedStatsArgs)
			return oneStat, nil
		}
		checkArgs(t, args, expectedMetadataArgs)
		return "[bad, format", nil
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Bad date format
	first = true
	w = runTest(func(args ...string) (string, error) {
		if first {
			first = false
			checkArgs(t, args, expectedStatsArgs)
			return oneStat, nil
		}
		checkArgs(t, args, expectedMetadataArgs)
		return `{"data":{"owner":"bob","creation_time":"fds"}}`, nil
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Success
	first = true
	w = runTest(func(args ...string) (string, error) {
		if first {
			first = false
			checkArgs(t, args, expectedStatsArgs)
			return oneStat, nil
		}
		checkArgs(t, args, expectedMetadataArgs)
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
			checkArgs(t, args, expectedStatsArgs)
			return `[{"bucket":"foo","usage":{"pool1":{"size":4,"num_objects":2}}},{"bucket":"bar","usage":{"pool2":{"size":5,"num_objects":4}}}]`, nil
		} else {
			// Expect the bucket metadata calls
			if containsArg(args, "bucket:foo") {
				return `{"data":{"owner":"bob","creation_time":"2016-08-05 16:23:34.343343Z"}}`, nil
			} else if containsArg(args, "bucket:bar") {
				return `{"data":{"owner":"bill","creation_time":"2016-08-05 18:31:22.445343Z"}}`, nil
			} else {
				assert.Fail(t, fmt.Sprintf("Wasn't foo or bar: %+v", args))
			}
		}
		assert.Fail(t, "Shouldn't return more than 3 times")
		return "", fmt.Errorf("Shouldn't return more than 3 times")
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[{"name":"bar","owner":"bill","createdAt":"2016-08-05T18:31:22.445343Z","size":5,"numberOfObjects":4},{"name":"foo","owner":"bob","createdAt":"2016-08-05T16:23:34.343343Z","size":4,"numberOfObjects":2}]`, w.Body.String())
}

func checkArgs(t *testing.T, args, desired []string) {
	// check that all hte desired args are found in order in the list of args
	start := indexIn(args, desired[0])
	for i, d := range desired {
		assert.Equal(t, args[(start+i)], d, fmt.Sprintf("%s not found at position %d in %v", d, (start+i), args))
	}
}

func indexIn(values []string, desired string) int {
	for i, val := range values {
		if val == desired {
			return i
		}
	}
	return -1
}

func containsArg(values []string, desired string) bool {
	for _, val := range values {
		if val == desired {
			return true
		}
	}
	return false
}

func TestGetBucket(t *testing.T) {
	req, err := http.NewRequest("GET", "http://10.0.0.100/objectstore/default/buckets/test", nil)
	if err != nil {
		logger.Fatal(err)
	}

	runTest := func(runner func(args ...string) (string, error)) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{MockExecuteCommandWithCombinedOutput: func(debug bool, actionName, command string, args ...string) (string, error) {
			return runner(args...)
		}}
		context := &clusterd.Context{Executor: executor}
		w := httptest.NewRecorder()
		h := newTestHandler(context)
		r := newRouter(h.GetRoutes())

		r.ServeHTTP(w, req)

		return w
	}

	expectedStatsArgs := []string{"bucket", "stats", "--bucket", "test"}
	expectedMetadataArgs := []string{"metadata", "get", "bucket:test"}

	// Stats fails
	w := runTest(func(args ...string) (string, error) {
		checkArgs(t, args, expectedStatsArgs)
		return "", fmt.Errorf("some error")
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// stats not found
	w = runTest(func(args ...string) (string, error) {
		checkArgs(t, args, expectedStatsArgs)
		return `2017-03-07 09:07:30.868797 c269240  0 could not get bucket info for bucket=tesdsft
	   2017-03-07 09:07:30.868797 c269240  0 could not get bucket info for bucket=tesdsft`, nil
	})
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Error parsing stats
	w = runTest(func(args ...string) (string, error) {
		checkArgs(t, args, expectedStatsArgs)
		return "{", nil
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// metadata fail
	w = runTest(func(args ...string) (string, error) {
		if args[1] == "stats" {
			checkArgs(t, args, expectedStatsArgs)
			return "{}", nil
		} else {
			checkArgs(t, args, expectedMetadataArgs)
			return "", fmt.Errorf("some error")
		}
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// metadata not found
	w = runTest(func(args ...string) (string, error) {
		if args[1] == "stats" {
			checkArgs(t, args, expectedStatsArgs)
			return "{}", nil
		} else {
			checkArgs(t, args, expectedMetadataArgs)
			return "ERROR: can't get key: (2) No such file or directory", nil
		}
	})
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "", w.Body.String())

	// metadata parse fail
	w = runTest(func(args ...string) (string, error) {
		if args[1] == "stats" {
			checkArgs(t, args, expectedStatsArgs)
			return "{}", nil
		} else {
			checkArgs(t, args, expectedMetadataArgs)
			return "{", nil
		}
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Success
	w = runTest(func(args ...string) (string, error) {
		if args[1] == "stats" {
			checkArgs(t, args, expectedStatsArgs)
			return `{"bucket":"test","usage":{"pool2":{"size":5,"num_objects":4}}}`, nil
		} else {
			checkArgs(t, args, expectedMetadataArgs)
			return `{"data":{"owner":"bill","creation_time":"2016-08-05 18:31:22.445343Z"}}`, nil
		}
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "{\"name\":\"test\",\"owner\":\"bill\",\"createdAt\":\"2016-08-05T18:31:22.445343Z\",\"size\":5,\"numberOfObjects\":4}", w.Body.String())
}

func TestBucketDelete(t *testing.T) {
	req, err := http.NewRequest("DELETE", "http://10.0.0.100/objectstore/default/buckets/test", nil)
	if err != nil {
		logger.Fatal(err)
	}

	runTest := func(s string, e error, expectedArgs ...string) *httptest.ResponseRecorder {
		executor := &testexec.MockExecutor{
			MockExecuteCommandWithCombinedOutput: func(debug bool, actionName, command string, args ...string) (string, error) {
				checkArgs(t, args, expectedArgs)
				return s, e
			},
		}
		context := &clusterd.Context{Executor: executor}

		w := httptest.NewRecorder()
		h := newTestHandler(context)
		r := newRouter(h.GetRoutes())

		r.ServeHTTP(w, req)

		return w
	}

	expectedArgs := []string{"bucket", "rm", "--bucket", "test"}

	// errors
	w := runTest("", fmt.Errorf("some error"), expectedArgs...)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Not found
	w = runTest(`2017-03-07 09:36:45.605774 c081240  0 could not get bucket info for bucket=tesdsft
	   2017-03-07 09:36:45.605774 c081240  0 could not get bucket info for bucket=tesdsft`, nil, expectedArgs...)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Not found
	w = runTest("unexpected content", nil, expectedArgs...)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "", w.Body.String())

	// Succeeds
	w = runTest("", nil, expectedArgs...)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "", w.Body.String())

	// not Purge
	req, err = http.NewRequest("DELETE", "http://10.0.0.100/objectstore/default/buckets/test?purge=false", nil)
	if err != nil {
		logger.Fatal(err)
	}
	w = runTest("", nil, expectedArgs...)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "", w.Body.String())

	//  Purge
	expectedArgs = []string{"bucket", "rm", "--bucket", "test", "--purge-objects"}
	req, err = http.NewRequest("DELETE", "http://10.0.0.100/objectstore/default/buckets/test?purge=true", nil)
	if err != nil {
		logger.Fatal(err)
	}
	w = runTest("", nil, expectedArgs...)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func getConfigSubDir(configDir string) string {
	return filepath.Join(configDir, "rookcluster")
}

func getExectedConfigArg(configSubDir string) string {
	return fmt.Sprintf("--conf=%s/rookcluster.config", configSubDir)
}

func getExpectedKeyringArg(configSubDir string) string {
	return fmt.Sprintf("--keyring=%s/client.admin.keyring", configSubDir)
}
