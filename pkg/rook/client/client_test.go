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
package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/rook/rook/pkg/model"
	"github.com/stretchr/testify/assert"
)

const (
	SuccessGetNodesContent                     = `[{"nodeID": "node1","publicIp": "1.2.3.100","privateIp": "10.0.0.100","storage": 100},{"nodeID": "node2","ipAddr": "10.0.0.101","storage": 200}]`
	SuccessGetPoolsContent                     = "[{\"poolName\":\"rbd\",\"poolNum\":0,\"type\":0,\"replicationConfig\":{\"size\":1},\"erasureCodedConfig\":{\"dataChunkCount\":0,\"codingChunkCount\":0,\"algorithm\":\"\"}},{\"poolName\":\"ecPool1\",\"poolNum\":1,\"type\":1,\"replicationConfig\":{\"size\":0},\"erasureCodedConfig\":{\"dataChunkCount\":2,\"codingChunkCount\":1,\"algorithm\":\"jerasure::reed_sol_van\"}}]"
	SuccessCreatePoolContent                   = `pool 'ecPool1' created`
	SuccessGetBlockImagesContent               = `[{"imageName":"myimage1","poolName":"rbd","size":10485760,"device":"","mountPoint":""},{"imageName":"myimage2","poolName":"rbd2","size":10485761,"device":"","mountPoint":""}]`
	SuccessCreateBlockImageContent             = `succeeded created image myimage3`
	SuccessDeleteBlockImageContent             = `succeeded deleting image myimage3`
	SuccessGetClientAccessInfoContent          = `{"monAddresses":["10.37.129.214:6790/0"],"userName":"admin","secretKey":"AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg=="}`
	SuccessGetFilesystemsContent               = `[{"name":"myfs1","metadataPool":"myfs1-metadata","dataPools":["myfs1-data"]}]`
	SuccessGetObjectStoreConnectionInfoContent = `{"host":"rook-ceph-rgw:12345", "accessKey":"UST0JAP8CE61FDE0Q4BE", "secretKey":"tVCuH20xTokjEpVJc7mKjL8PLTfGh4NZ3le3zg9X"}`
)

func TestURL(t *testing.T) {
	client := NewRookNetworkRestClient(GetRestURL("10.0.1.2:8124"), http.DefaultClient)
	assert.Equal(t, "http://10.0.1.2:8124", client.URL())
}

func TestRookRestError(t *testing.T) {
	err := RookRestError{Query: "foo", Status: http.StatusBadRequest, Body: []byte("error body")}
	assert.Equal(t, "HTTP status code 400 for query foo: 'error body'", err.Error())
}

func TestGetNodes(t *testing.T) {
	mockServer := NewMockHttpServer(200, SuccessGetNodesContent)
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewRookNetworkRestClient(mockServer.URL, mockHttpClient)

	// invoke the GetNodes method that will use our mock http client/server to return a successful response
	getNodesResponse, err := client.GetNodes()
	assert.Nil(t, err)
	assert.NotNil(t, getNodesResponse)
	assert.Equal(t, 2, len(getNodesResponse))

	var testNode model.Node
	for i := range getNodesResponse {
		if getNodesResponse[i].NodeID == "node1" {
			testNode = getNodesResponse[i]
			break
		}
	}

	assert.NotNil(t, testNode)
	assert.Equal(t, "1.2.3.100", testNode.PublicIP)
	assert.Equal(t, "10.0.0.100", testNode.PrivateIP)
	assert.Equal(t, uint64(100), testNode.Storage)
}

func TestGetPools(t *testing.T) {
	mockServer := NewMockHttpServer(200, SuccessGetPoolsContent)
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewRookNetworkRestClient(mockServer.URL, mockHttpClient)

	// invoke the GetPools method that will use our mock http client/server to return a successful response
	getPoolsResponse, err := client.GetPools()
	assert.Nil(t, err)
	assert.NotNil(t, getPoolsResponse)
	assert.Equal(t, 2, len(getPoolsResponse))

	expectedPool1 := model.Pool{
		Name:   "ecPool1",
		Number: 1,
		Type:   model.ErasureCoded,
		ErasureCodedConfig: model.ErasureCodedPoolConfig{
			DataChunkCount:   2,
			CodingChunkCount: 1,
			Algorithm:        "jerasure::reed_sol_van",
		},
	}
	var actualPool model.Pool
	for i := range getPoolsResponse {
		if getPoolsResponse[i].Name == "ecPool1" {
			actualPool = getPoolsResponse[i]
			break
		}
	}

	assert.NotNil(t, actualPool)
	assert.Equal(t, expectedPool1, actualPool)
}

func TestCreatePool(t *testing.T) {
	mockServer := NewMockHttpServer(200, SuccessCreatePoolContent)
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewRookNetworkRestClient(mockServer.URL, mockHttpClient)

	// invoke the CreatePool method that will use our mock http client/server to return a successful response
	newPool := model.Pool{
		Name:   "ecPool1",
		Number: 1,
		Type:   model.ErasureCoded,
		ErasureCodedConfig: model.ErasureCodedPoolConfig{
			DataChunkCount:   2,
			CodingChunkCount: 1,
			Algorithm:        "jerasure::reed_sol_van",
		},
	}
	createPoolResponse, err := client.CreatePool(newPool)
	assert.Nil(t, err)
	assert.Equal(t, "pool 'ecPool1' created\n", createPoolResponse)
}

func TestGetBlockImages(t *testing.T) {
	mockServer := NewMockHttpServer(200, SuccessGetBlockImagesContent)
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewRookNetworkRestClient(mockServer.URL, mockHttpClient)

	getBlockImagesResponse, err := client.GetBlockImages()
	assert.Nil(t, err)
	assert.Equal(t, 2, len(getBlockImagesResponse))

	expectedImage := model.BlockImage{
		Name:     "myimage2",
		PoolName: "rbd2",
		Size:     10485761,
	}
	var actualImage *model.BlockImage
	for i := range getBlockImagesResponse {
		if getBlockImagesResponse[i].Name == expectedImage.Name {
			actualImage = &(getBlockImagesResponse[i])
			break
		}
	}
	assert.NotNil(t, actualImage)
	assert.Equal(t, expectedImage, *actualImage)
}

func TestCreateBlockImage(t *testing.T) {
	mockServer := NewMockHttpServer(200, SuccessCreateBlockImageContent)
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewRookNetworkRestClient(mockServer.URL, mockHttpClient)

	newImage := model.BlockImage{
		Name:     "myimage3",
		PoolName: "rbd2",
		Size:     10485762,
	}

	response, err := client.CreateBlockImage(newImage)
	assert.Nil(t, err)
	assert.Equal(t, SuccessCreateBlockImageContent+"\n", response)
}

func TestDeleteBlockImage(t *testing.T) {
	mockServer := NewMockHttpServer(200, SuccessDeleteBlockImageContent)
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewRookNetworkRestClient(mockServer.URL, mockHttpClient)

	deleteImage := model.BlockImage{
		Name:     "myimage3",
		PoolName: "rbd2",
	}

	response, err := client.DeleteBlockImage(deleteImage)
	assert.Nil(t, err)
	assert.Equal(t, SuccessDeleteBlockImageContent+"\n", response)
}

func TestGetClientAccessInfo(t *testing.T) {
	mockServer := NewMockHttpServer(200, SuccessGetClientAccessInfoContent)
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewRookNetworkRestClient(mockServer.URL, mockHttpClient)

	expectedClientAccessInfo := model.ClientAccessInfo{
		MonAddresses: []string{"10.37.129.214:6790/0"},
		UserName:     "admin",
		SecretKey:    "AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg==",
	}

	actualClientAccessInfo, err := client.GetClientAccessInfo()
	assert.Nil(t, err)
	assert.Equal(t, expectedClientAccessInfo, actualClientAccessInfo)
}

func TestGetFilesystems(t *testing.T) {
	mockServer := NewMockHttpServer(200, SuccessGetFilesystemsContent)
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewRookNetworkRestClient(mockServer.URL, mockHttpClient)

	expectedFilesystems := []model.Filesystem{
		{Name: "myfs1", MetadataPool: "myfs1-metadata", DataPools: []string{"myfs1-data"}},
	}

	actualFilesystems, err := client.GetFilesystems()
	assert.Nil(t, err)
	assert.Equal(t, expectedFilesystems, actualFilesystems)
}

func TestCreateFilesystem(t *testing.T) {
	mockServer := NewMockHttpServer(202, "")
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewRookNetworkRestClient(mockServer.URL, mockHttpClient)

	fsr := model.FilesystemRequest{Name: "myfs1", PoolName: "myfs1-pool"}
	resp, err := client.CreateFilesystem(fsr)
	assert.NotNil(t, err)
	assert.True(t, IsHttpAccepted(err))
	assert.Equal(t, "", resp)
}

func TestDeleteFilesystem(t *testing.T) {
	mockServer := NewMockHttpServer(202, "")
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewRookNetworkRestClient(mockServer.URL, mockHttpClient)

	fsr := model.FilesystemRequest{Name: "myfs1", PoolName: "myfs1-pool"}
	resp, err := client.DeleteFilesystem(fsr)
	assert.NotNil(t, err)
	assert.True(t, IsHttpAccepted(err))
	assert.Equal(t, "", resp)
}

func TestCreateObjectStore(t *testing.T) {
	mockServer := NewMockHttpServer(202, "")
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewRookNetworkRestClient(mockServer.URL, mockHttpClient)

	resp, err := client.CreateObjectStore()
	assert.NotNil(t, err)
	assert.True(t, IsHttpAccepted(err))
	assert.Equal(t, "", resp)
}

func TestGetObjectStoreConnectionInfo(t *testing.T) {
	mockServer := NewMockHttpServer(200, SuccessGetObjectStoreConnectionInfoContent)
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewRookNetworkRestClient(mockServer.URL, mockHttpClient)

	expectedResp := model.ObjectStoreConnectInfo{
		Host: "rook-ceph-rgw:12345",
	}

	resp, err := client.GetObjectStoreConnectionInfo()
	assert.Nil(t, err)
	assert.Equal(t, expectedResp, *resp)
}

func TestGetNodesFailure(t *testing.T) {
	ClientFailureHelper(t, func(client RookRestClient) (interface{}, error) { return client.GetNodes() })
}

func TestGetPoolsFailure(t *testing.T) {
	ClientFailureHelper(t, func(client RookRestClient) (interface{}, error) { return client.GetPools() })
}

func TestCreatePoolFailure(t *testing.T) {
	clientFunc := func(client RookRestClient) (interface{}, error) {
		return client.CreatePool(model.Pool{Name: "pool1"})
	}
	verifyFunc := getStringVerifyFunc(t)
	ClientFailureHelperWithVerification(t, clientFunc, verifyFunc)
}

func TestGetBlockImagesFailure(t *testing.T) {
	ClientFailureHelper(t, func(client RookRestClient) (interface{}, error) { return client.GetBlockImages() })
}

func TestCreateBlockImageFailure(t *testing.T) {
	clientFunc := func(client RookRestClient) (interface{}, error) {
		return client.CreateBlockImage(model.BlockImage{Name: "image1"})
	}
	verifyFunc := getStringVerifyFunc(t)
	ClientFailureHelperWithVerification(t, clientFunc, verifyFunc)
}

func TestDeleteBlockImageFailure(t *testing.T) {
	clientFunc := func(client RookRestClient) (interface{}, error) {
		return client.DeleteBlockImage(model.BlockImage{Name: "image1"})
	}
	verifyFunc := getStringVerifyFunc(t)
	ClientFailureHelperWithVerification(t, clientFunc, verifyFunc)
}

func TestGetClientAccessInfoFailure(t *testing.T) {
	clientFunc := func(client RookRestClient) (interface{}, error) {
		return client.GetClientAccessInfo()
	}
	verifyFunc := func(resp interface{}, err error) {
		assert.NotNil(t, err)
		assert.Equal(t, model.ClientAccessInfo{}, resp.(model.ClientAccessInfo))
	}
	ClientFailureHelperWithVerification(t, clientFunc, verifyFunc)
}

func TestGetFilesystemsFailure(t *testing.T) {
	ClientFailureHelper(t, func(client RookRestClient) (interface{}, error) { return client.GetFilesystems() })
}

func TestCreateFilesystemFailure(t *testing.T) {
	clientFunc := func(client RookRestClient) (interface{}, error) {
		return client.CreateFilesystem(model.FilesystemRequest{Name: "myfs1"})
	}
	verifyFunc := getStringVerifyFunc(t)
	ClientFailureHelperWithVerification(t, clientFunc, verifyFunc)
}

func TestDeleteFilesystemFailure(t *testing.T) {
	clientFunc := func(client RookRestClient) (interface{}, error) {
		return client.DeleteFilesystem(model.FilesystemRequest{Name: "myfs1"})
	}
	verifyFunc := getStringVerifyFunc(t)
	ClientFailureHelperWithVerification(t, clientFunc, verifyFunc)
}

func TestCreateObjectStoreFailure(t *testing.T) {
	clientFunc := func(client RookRestClient) (interface{}, error) {
		return client.CreateObjectStore()
	}
	verifyFunc := getStringVerifyFunc(t)
	ClientFailureHelperWithVerification(t, clientFunc, verifyFunc)
}

func TestGetObjectStoreConnectionInfoFailure(t *testing.T) {
	clientFunc := func(client RookRestClient) (interface{}, error) {
		return client.GetObjectStoreConnectionInfo()
	}
	verifyFunc := func(resp interface{}, err error) {
		assert.NotNil(t, err)
		assert.Nil(t, resp)
	}
	ClientFailureHelperWithVerification(t, clientFunc, verifyFunc)
}

func ClientFailureHelper(t *testing.T, clientFunc func(RookRestClient) (interface{}, error)) {
	verifyFunc := func(resp interface{}, err error) {
		assert.NotNil(t, err)
		assert.Nil(t, resp)
	}
	ClientFailureHelperWithVerification(t, clientFunc, verifyFunc)
}

func ClientFailureHelperWithVerification(t *testing.T, clientFunc func(RookRestClient) (interface{}, error),
	verifyFunc func(interface{}, error)) {

	mockServer := NewMockHttpServer(500, "something went wrong!")
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewRookNetworkRestClient(mockServer.URL, mockHttpClient)

	// invoke the client func
	resp, err := clientFunc(client)

	// invoke the verification func to verify resp and err
	verifyFunc(resp, err)
}

func getStringVerifyFunc(t *testing.T) func(resp interface{}, err error) {
	return func(resp interface{}, err error) {
		assert.NotNil(t, err)
		assert.Equal(t, "", resp.(string))
	}
}

func NewMockHttpServer(responseStatusCode int, responseBody string) *httptest.Server {
	// create and return a mock http server that will return the specified HTTP status code and response body content
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(responseStatusCode)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, responseBody)
	}))
}

func NewMockHttpClient(mockServerURL string) *http.Client {
	// create a transport that will use direct all network traffic to the mock HTTP server
	transport := &http.Transport{
		Proxy: func(request *http.Request) (*url.URL, error) {
			return url.Parse(mockServerURL)
		},
	}

	// return a http client that uses our transport
	return &http.Client{Transport: transport}
}
