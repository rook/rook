package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/quantum/castle/pkg/model"
	"github.com/stretchr/testify/assert"
)

const (
	SuccessGetNodesContent             = `[{"nodeID": "node1","ipAddr": "10.0.0.100","storage": 100},{"nodeID": "node2","ipAddr": "10.0.0.101","storage": 200}]`
	SuccessGetPoolsContent             = "[{\"poolName\":\"rbd\",\"poolNum\":0,\"type\":0,\"replicationConfig\":{\"size\":1},\"erasureCodedConfig\":{\"dataChunkCount\":0,\"codingChunkCount\":0,\"algorithm\":\"\"}},{\"poolName\":\"ecPool1\",\"poolNum\":1,\"type\":1,\"replicationConfig\":{\"size\":0},\"erasureCodedConfig\":{\"dataChunkCount\":2,\"codingChunkCount\":1,\"algorithm\":\"jerasure::reed_sol_van\"}}]"
	SuccessCreatePoolContent           = `pool 'ecPool1' created`
	SuccessGetBlockImagesContent       = `[{"imageName":"myimage1","poolName":"rbd","size":10485760,"device":"","mountPoint":""},{"imageName":"myimage2","poolName":"rbd2","size":10485761,"device":"","mountPoint":""}]`
	SuccessCreateBlockImageContent     = `succeeded created image myimage3`
	SuccessGetBlockImageMapInfoContent = `{"monAddresses":["10.37.129.214:6790/0"],"userName":"admin","secretKey":"AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg=="}`
)

func TestURL(t *testing.T) {
	client := NewCastleNetworkRestClient(GetRestURL("10.0.1.2:8124"), http.DefaultClient)
	assert.Equal(t, "http://10.0.1.2:8124", client.URL())
}

func TestCastleRestError(t *testing.T) {
	err := CastleRestError{Query: "foo", Status: http.StatusBadRequest, Body: []byte("error body")}
	assert.Equal(t, "HTTP status code 400 for query foo: 'error body'", err.Error())
}

func TestGetNodes(t *testing.T) {
	mockServer := NewMockHttpServer(200, SuccessGetNodesContent)
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewCastleNetworkRestClient(mockServer.URL, mockHttpClient)

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
	assert.Equal(t, "10.0.0.100", testNode.IPAddress)
	assert.Equal(t, uint64(100), testNode.Storage)
}

func TestGetPools(t *testing.T) {
	mockServer := NewMockHttpServer(200, SuccessGetPoolsContent)
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewCastleNetworkRestClient(mockServer.URL, mockHttpClient)

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
	client := NewCastleNetworkRestClient(mockServer.URL, mockHttpClient)

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
	client := NewCastleNetworkRestClient(mockServer.URL, mockHttpClient)

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
	client := NewCastleNetworkRestClient(mockServer.URL, mockHttpClient)

	newImage := model.BlockImage{
		Name:     "myimage3",
		PoolName: "rbd2",
		Size:     10485762,
	}

	response, err := client.CreateBlockImage(newImage)
	assert.Nil(t, err)
	assert.Equal(t, SuccessCreateBlockImageContent+"\n", response)
}

func TestGetBlockImageMapInfo(t *testing.T) {
	mockServer := NewMockHttpServer(200, SuccessGetBlockImageMapInfoContent)
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewCastleNetworkRestClient(mockServer.URL, mockHttpClient)

	expectedImageMapInfo := model.BlockImageMapInfo{
		MonAddresses: []string{"10.37.129.214:6790/0"},
		UserName:     "admin",
		SecretKey:    "AQBsCv1X5oD9GhAARHVU9N+kFRWDjyLA1dqzIg==",
	}

	actualImageMapInfo, err := client.GetBlockImageMapInfo()
	assert.Nil(t, err)
	assert.Equal(t, expectedImageMapInfo, actualImageMapInfo)
}

func TestGetNodesFailure(t *testing.T) {
	ClientFailureHelper(t, func(client CastleRestClient) (interface{}, error) { return client.GetNodes() })
}

func TestGetPoolsFailure(t *testing.T) {
	ClientFailureHelper(t, func(client CastleRestClient) (interface{}, error) { return client.GetPools() })
}

func TestCreatePoolFailure(t *testing.T) {
	clientFunc := func(client CastleRestClient) (interface{}, error) {
		return client.CreatePool(model.Pool{Name: "pool1"})
	}
	verifyFunc := getStringVerifyFunc(t)
	ClientFailureHelperWithVerification(t, clientFunc, verifyFunc)
}

func TestGetBlockImagesFailure(t *testing.T) {
	ClientFailureHelper(t, func(client CastleRestClient) (interface{}, error) { return client.GetBlockImages() })
}

func TestCreateBlockImageFailure(t *testing.T) {
	clientFunc := func(client CastleRestClient) (interface{}, error) {
		return client.CreateBlockImage(model.BlockImage{Name: "image1"})
	}
	verifyFunc := getStringVerifyFunc(t)
	ClientFailureHelperWithVerification(t, clientFunc, verifyFunc)
}

func TestGetBlockImageMapInfoFailure(t *testing.T) {
	clientFunc := func(client CastleRestClient) (interface{}, error) {
		return client.GetBlockImageMapInfo()
	}
	verifyFunc := func(resp interface{}, err error) {
		assert.NotNil(t, err)
		assert.Equal(t, model.BlockImageMapInfo{}, resp.(model.BlockImageMapInfo))
	}
	ClientFailureHelperWithVerification(t, clientFunc, verifyFunc)
}

func ClientFailureHelper(t *testing.T, clientFunc func(CastleRestClient) (interface{}, error)) {
	verifyFunc := func(resp interface{}, err error) {
		assert.NotNil(t, err)
		assert.Nil(t, resp)
	}
	ClientFailureHelperWithVerification(t, clientFunc, verifyFunc)
}

func ClientFailureHelperWithVerification(t *testing.T, clientFunc func(CastleRestClient) (interface{}, error),
	verifyFunc func(interface{}, error)) {

	mockServer := NewMockHttpServer(500, "something went wrong!")
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewCastleNetworkRestClient(mockServer.URL, mockHttpClient)

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
