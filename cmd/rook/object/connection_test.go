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
package object

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/rook/test"
)

func TestGetConnectionInfo(t *testing.T) {
	access := "UST0JAP8CE61FDE0Q4BE"
	secret := "tVCuH20xTokjEpVJc7mKjL8PLTfGh4NZ3le3zg9X"

	c := &test.MockRookRestClient{
		MockGetObjectStoreConnectionInfo: func() (*model.ObjectStoreConnectInfo, error) {
			return &model.ObjectStoreConnectInfo{
				Host:       "rook-ceph-rgw:12345",
				IPEndpoint: "1.2.3.4:12345",
			}, nil
		},
		MockGetObjectUser: func(s string) (*model.ObjectUser, error) {
			return &model.ObjectUser{AccessKey: &access, SecretKey: &secret}, nil
		},
	}

	// verify pretty format output
	expectedOut := "NAME                    VALUE\n" +
		"AWS_HOST                rook-ceph-rgw:12345                        \n" +
		"AWS_ENDPOINT            1.2.3.4:12345                              \n" +
		"AWS_ACCESS_KEY_ID       UST0JAP8CE61FDE0Q4BE                       \n" +
		"AWS_SECRET_ACCESS_KEY   tVCuH20xTokjEpVJc7mKjL8PLTfGh4NZ3le3zg9X   \n"
	out, err := getConnectionInfo(c, "testuser", FormatPretty)
	assert.Nil(t, err)
	assert.Equal(t, expectedOut, out)

	// verify pretty format output
	expectedOut = "export AWS_HOST=rook-ceph-rgw:12345\n" +
		"export AWS_ENDPOINT=1.2.3.4:12345\n" +
		"export AWS_ACCESS_KEY_ID=UST0JAP8CE61FDE0Q4BE\n" +
		"export AWS_SECRET_ACCESS_KEY=tVCuH20xTokjEpVJc7mKjL8PLTfGh4NZ3le3zg9X\n"
	out, err = getConnectionInfo(c, "testuser", FormatEnvVar)
	assert.Nil(t, err)
	assert.Equal(t, expectedOut, out)
}

func TestGetConnectionInfoNotFound(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetObjectStoreConnectionInfo: func() (*model.ObjectStoreConnectInfo, error) {
			return &model.ObjectStoreConnectInfo{}, client.RookRestError{
				Status: http.StatusNotFound,
			}
		},
	}

	out, err := getConnectionInfo(c, "testuser", FormatPretty)
	assert.Nil(t, err)
	assert.Equal(t, "object store connection info is not ready, if \"object create\" has already been run, please be patient\n", out)
}

func TestGetConnectionInfoError(t *testing.T) {
	c := &test.MockRookRestClient{
		MockGetObjectStoreConnectionInfo: func() (*model.ObjectStoreConnectInfo, error) {
			return &model.ObjectStoreConnectInfo{}, fmt.Errorf("mock get connection info failed")
		},
	}

	out, err := getConnectionInfo(c, "testuser", FormatPretty)
	assert.NotNil(t, err)
	assert.Equal(t, "", out)
}
