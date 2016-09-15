package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	SuccessCastleGetNodesContent = `[{"nodeID": "node1","ipAddr": "10.0.0.100","storage": 100},{"nodeID": "node2","ipAddr": "10.0.0.101","storage": 200}]`
)

func TestURL(t *testing.T) {
	client := NewCastleNetworkRestClient(GetRestURL("10.0.1.2:8124"), http.DefaultClient)
	assert.Equal(t, "http://10.0.1.2:8124/", client.URL())
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

	var testNode Node
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

func TestGetNodesFailure(t *testing.T) {
	ClientFailureHelper(t, func(client CastleRestClient) (interface{}, error) { return client.GetNodes() })
}

func ClientFailureHelper(t *testing.T, ClientFunc func(CastleRestClient) (interface{}, error)) {
	mockServer := NewMockHttpServer(500, "something went wrong!")
	defer mockServer.Close()
	mockHttpClient := NewMockHttpClient(mockServer.URL)
	client := NewCastleNetworkRestClient(mockServer.URL, mockHttpClient)

	resp, err := ClientFunc(client)
	assert.NotNil(t, err)
	assert.Nil(t, resp)
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
