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
	"strings"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestGetImagesHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
		Executor:      &exectest.MockExecutor{},
	}

	req, err := http.NewRequest("GET", "http://10.0.0.100/image", nil)
	if err != nil {
		logger.Fatal(err)
	}

	// first return no storage pools, which means no images will be returned either
	w := httptest.NewRecorder()
	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			return `[]`, nil
		},
	}

	// no images will be returned, should be empty output
	h := newTestHandler(context)
	h.GetImages(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `[]`, w.Body.String())

	// now return some storage pools and images from the ceph connection
	w = httptest.NewRecorder()
	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			switch {
			case args[0] == "osd" && args[1] == "lspools":
				return `[{"poolnum":0,"poolname":"pool0"},{"poolnum":1,"poolname":"pool1"}]`, nil
			}
			return "", fmt.Errorf("unexpected mon_command '%v'", args)
			/*	return &testceph.MockIOContext{
				MockGetImageNames: func() (names []string, err error) {
					return []string{fmt.Sprintf("image1 - %s", pool)}, nil
				},
				MockGetImage: func(name string) ceph.Image {
					return &testceph.MockImage{
						MockName: name,
						MockStat: func() (info *ceph.ImageInfo, err error) {
							return &ceph.ImageInfo{
								Size: 100,
							}, nil
						},
					}
				},
			},*/
		},
	}

	// verify that the expected images are returned
	h = newTestHandler(context)
	h.GetImages(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[{\"imageName\":\"image1 - pool0\",\"poolName\":\"pool0\",\"size\":100,\"device\":\"\",\"mountPoint\":\"\"},{\"imageName\":\"image1 - pool1\",\"poolName\":\"pool1\",\"size\":100,\"device\":\"\",\"mountPoint\":\"\"}]", w.Body.String())
}

func TestGetImagesHandlerFailure(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
	}

	req, err := http.NewRequest("GET", "http://10.0.0.100/image", nil)
	if err != nil {
		logger.Fatal(err)
	}

	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			return "mock error", fmt.Errorf("mock error for list pools")
		},
	}

	// GetImages should fail due to the mocked error for listing pools
	w := httptest.NewRecorder()
	h := newTestHandler(context)
	h.GetImages(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, ``, w.Body.String())
}

func TestCreateImageHandler(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
		Executor:      &exectest.MockExecutor{},
	}

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
	req, err = http.NewRequest("POST", "http://10.0.0.100/image", strings.NewReader(`{"imageName":"myImage1","poolName":"myPool1","size":1024}`))
	if err != nil {
		logger.Fatal(err)
	}
	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			/*return &testceph.MockIOContext{
				MockCreateImage: func(name string, size uint64, order int, args ...uint64) (image ceph.Image, err error) {
					return &testceph.MockImage{MockName: name}, nil
				},
			},*/
			return "", fmt.Errorf("not implemented")
		},
	}
	w = httptest.NewRecorder()
	h = newTestHandler(context)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `succeeded created image myImage1`, w.Body.String())
}

func TestCreateImageHandlerFailure(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient},
	}

	req, err := http.NewRequest("POST", "http://10.0.0.100/image", strings.NewReader(`{"imageName":"myImage1","poolName":"myPool1","size":1024}`))
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			/*return &testceph.MockIOContext{
				// mock a failure in the create image call
				MockCreateImage: func(name string, size uint64, order int, args ...uint64) (image ceph.Image, err error) {
					return &testceph.MockImage{}, fmt.Errorf("mock failure to create image")
				},
			}*/return "", fmt.Errorf("not implemented")
		},
	}

	// create image request should fail while creating the image
	h := newTestHandler(context)
	h.CreateImage(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, ``, w.Body.String())
}

func TestDeleteImageHandler(t *testing.T) {
	context := &clusterd.Context{Executor: &exectest.MockExecutor{}}

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
	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			/*return &testceph.MockIOContext{
				MockGetImage: func(name string) ceph.Image {
					return &testceph.MockImage{
						MockName: name,
					}
				},
			}*/return "", fmt.Errorf("not implemented")
		},
	}
	w = httptest.NewRecorder()
	h = newTestHandler(context)
	h.DeleteImage(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `succeeded deleting image myImage1`, w.Body.String())
}

func TestDeleteImageHandlerFailure(t *testing.T) {
	context := &clusterd.Context{}

	req, err := http.NewRequest("DELETE", "http://10.0.0.100/image?name=myImage1&pool=myPool1", nil)
	if err != nil {
		logger.Fatal(err)
	}

	w := httptest.NewRecorder()
	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			/*return &testceph.MockIOContext{
				MockGetImage: func(name string) ceph.Image {
					return &testceph.MockImage{
						MockName: name,
						// mock a failure in the Remove func
						MockRemove: func() error {
							return fmt.Errorf("mock failure to remove image")
						},
					}
				},
			}*/return "", fmt.Errorf("not implemented")
		},
	}

	// delete image request should fail while removing the image
	h := newTestHandler(context)
	h.DeleteImage(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, ``, w.Body.String())
}
