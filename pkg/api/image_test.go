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
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetImagesHandler(t *testing.T) {
	context, _, executor := testContext()
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("GET", "http://10.0.0.100/image", nil)
	if err != nil {
		logger.Fatal(err)
	}

	// first return no storage pools, which means no images will be returned either
	w := httptest.NewRecorder()
	executor.MockExecuteCommandWithOutputFile = func(actionName string, command string, outFileArg string, args ...string) (string, error) {
		return `[]`, nil
	}
	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
		return `[]`, nil
	}

	// no images will be returned, should be empty output
	h := newTestHandler(context)
	h.GetImages(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[]`, w.Body.String())

	// now return some storage pools and images from the ceph connection
	w = httptest.NewRecorder()
	executor.MockExecuteCommandWithOutputFile = func(actionName string, command string, outFileArg string, args ...string) (string, error) {
		switch {
		case command == "ceph" && args[0] == "osd" && args[1] == "lspools":
			return `[{"poolnum":0,"poolname":"pool0"},{"poolnum":1,"poolname":"pool1"}]`, nil
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}

	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
		switch {
		case command == "rbd" && args[0] == "ls" && args[1] == "-l":
			poolName := args[2]
			return fmt.Sprintf(`[{"image":"image1 - %s","size":100,"format":2}]`, poolName), nil
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}

	// verify that the expected images are returned
	h = newTestHandler(context)
	h.GetImages(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[{\"imageName\":\"image1 - pool0\",\"poolName\":\"pool0\",\"size\":100,\"device\":\"\",\"mountPoint\":\"\"},{\"imageName\":\"image1 - pool1\",\"poolName\":\"pool1\",\"size\":100,\"device\":\"\",\"mountPoint\":\"\"}]", w.Body.String())
}

func TestGetImagesHandlerFailure(t *testing.T) {
	context, _, executor := testContext()
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("GET", "http://10.0.0.100/image", nil)
	if err != nil {
		logger.Fatal(err)
	}

	executor.MockExecuteCommandWithOutputFile = func(actionName string, command string, outFileArg string, args ...string) (string, error) {
		return "mock error", fmt.Errorf("mock error for list pools")
	}

	// GetImages should fail due to the mocked error for listing pools
	w := httptest.NewRecorder()
	h := newTestHandler(context)
	h.GetImages(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, ``, w.Body.String())
}

func TestCreateImageHandler(t *testing.T) {
	context, _, executor := testContext()
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("POST", "http://10.0.0.100/image", nil)
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()

	// image is missing from request body, should be bad request
	h := newTestHandler(context)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, ``, w.Body.String())

	// request body exists but it's bad json, should be bad request
	req, err = http.NewRequest("POST", "http://10.0.0.100/image", strings.NewReader(`bad json`))
	if err != nil {
		logger.Fatal(err)
	}
	w = httptest.NewRecorder()
	h = newTestHandler(context)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, ``, w.Body.String())

	// missing fields for the image passed via request body, should be bad request
	req, err = http.NewRequest("POST", "http://10.0.0.100/image", strings.NewReader(`{"imageName":"myImage1"}`))
	if err != nil {
		logger.Fatal(err)
	}
	w = httptest.NewRecorder()
	h = newTestHandler(context)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, ``, w.Body.String())

	// well formed successful request to create an image
	req, err = http.NewRequest("POST", "http://10.0.0.100/image", strings.NewReader(`{"imageName":"myImage1","poolName":"myPool1","size":1048576}`))
	if err != nil {
		logger.Fatal(err)
	}
	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
		switch {
		case command == "rbd" && args[0] == "create":
			return "", nil
		case command == "rbd" && args[0] == "ls" && args[1] == "-l":
			return `[{"image":"myImage1","size":1048576,"format":2}]`, nil
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}
	w = httptest.NewRecorder()
	h = newTestHandler(context)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `succeeded created image myImage1`, w.Body.String())
}

func TestCreateImageHandlerFailure(t *testing.T) {
	context, _, executor := testContext()
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("POST", "http://10.0.0.100/image", strings.NewReader(`{"imageName":"myImage1","poolName":"myPool1","size":1024}`))
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
		return "mock failure", fmt.Errorf("mock failure to create image")
	}

	// create image request should fail while creating the image
	h := newTestHandler(context)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, ``, w.Body.String())
}

func TestDeleteImageHandler(t *testing.T) {
	context, _, executor := testContext()
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("DELETE", "http://10.0.0.100/image", nil)
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()

	// no image params are passed via URL query string, bad request
	h := newTestHandler(context)
	h.DeleteImage(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, ``, w.Body.String())

	// bad query param passed, should be bad request
	req, err = http.NewRequest("DELETE", "http://10.0.0.100/image?badparam=foo", nil)
	if err != nil {
		logger.Fatal(err)
	}
	w = httptest.NewRecorder()
	h = newTestHandler(context)
	h.DeleteImage(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, ``, w.Body.String())

	// missing fields for the image passed via query params, should be bad request
	req, err = http.NewRequest("DELETE", "http://10.0.0.100/image?name=myImage1", nil)
	if err != nil {
		logger.Fatal(err)
	}
	w = httptest.NewRecorder()
	h = newTestHandler(context)
	h.DeleteImage(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, ``, w.Body.String())

	// well formed successful request to delete an image
	req, err = http.NewRequest("DELETE", "http://10.0.0.100/image?name=myImage1&pool=myPool1", nil)
	if err != nil {
		logger.Fatal(err)
	}
	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
		switch {
		case command == "rbd" && args[0] == "rm":
			return "", nil
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}
	w = httptest.NewRecorder()
	h = newTestHandler(context)
	h.DeleteImage(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `succeeded deleting image myImage1`, w.Body.String())
}

func TestDeleteImageHandlerFailure(t *testing.T) {
	context, _, executor := testContext()
	defer os.RemoveAll(context.ConfigDir)

	req, err := http.NewRequest("DELETE", "http://10.0.0.100/image?name=myImage1&pool=myPool1", nil)
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	executor.MockExecuteCommandWithOutput = func(actionName string, command string, args ...string) (string, error) {
		switch {
		case command == "rbd" && args[0] == "rm":
			return "mock failure", fmt.Errorf("mock failure to remove image")
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}

	// delete image request should fail while removing the image
	h := newTestHandler(context)
	h.DeleteImage(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, ``, w.Body.String())
}
