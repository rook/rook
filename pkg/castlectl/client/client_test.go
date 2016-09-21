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
	SuccessCastleGetNodesContent   = `[{"nodeID": "node1","ipAddr": "10.0.0.100","storage": 100},{"nodeID": "node2","ipAddr": "10.0.0.101","storage": 200}]`
	SuccessCastleGetPoolsContent   = `[{"poolname":"pool1","poolnum":1},{"poolname":"pool50","poolnum":50}]`
	SuccessCastleCreatePoolContent = `created pool1 successfully`
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
	mockServer := NewMockHttpServer(200, SuccessCastleGetNodesContent)
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
	mockServer := NewMockHttpServer(200, SuccessCastleGetPoolsContent)
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewCastleNetworkRestClient(mockServer.URL, mockHttpClient)

	// invoke the GetPools method that will use our mock http client/server to return a successful response
	getPoolsResponse, err := client.GetPools()
	assert.Nil(t, err)
	assert.NotNil(t, getPoolsResponse)
	assert.Equal(t, 2, len(getPoolsResponse))

	var testPool model.Pool
	for i := range getPoolsResponse {
		if getPoolsResponse[i].Name == "pool1" {
			testPool = getPoolsResponse[i]
			break
		}
	}

	assert.NotNil(t, testPool)
	assert.Equal(t, 1, testPool.Number)
}

func TestCreatePool(t *testing.T) {
	mockServer := NewMockHttpServer(200, SuccessCastleCreatePoolContent)
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewCastleNetworkRestClient(mockServer.URL, mockHttpClient)

	// invoke the CreatePool method that will use our mock http client/server to return a successful response
	createPoolResponse, err := client.CreatePool(model.Pool{Name: "pool1"})
	assert.Nil(t, err)
	assert.Equal(t, "created pool1 successfully\n", createPoolResponse)
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
	verifyFunc := func(resp interface{}, err error) {
		assert.NotNil(t, err)
		assert.Equal(t, "", resp.(string))
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
