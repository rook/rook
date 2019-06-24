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
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"
)

// RPCClient makes API calls to noobaa.
// Requests to noobaa are plain http requests with json request and json response.
type RPCClient struct {
	Router     RPCRouter
	HTTPClient http.Client
	AuthToken  string
}

// RPCRequest is the interface for request structs.
// Structs that implement it will build a request body and a response to be decoded.
type RPCRequest interface {
	build() (*RPCRequestBody, RPCResponse)
}

// RPCResponse is the interface for response structs.
// RPCResponseBody is the only real implementor of it.
// Specific response structures should just include RPCResponseBody inline.
type RPCResponse interface {
	GetError() *RPCError
}

// RPCRequestBody is the structure encoded in every request
// Params is the specific request params.
type RPCRequestBody struct {
	API       string      `json:"api,omitempty"`
	Method    string      `json:"method,omitempty"`
	AuthToken string      `json:"auth_token,omitempty"`
	Params    interface{} `json:"params,omitempty"`
}

// RPCResponseBody is the structure encoded in every response
// The specific response structure should include this inline,
// and add the standard Reply field with the specific fields.
type RPCResponseBody struct {
	Op        string    `json:"op,omitempty"`
	RequestID string    `json:"reqid,omitempty"`
	Took      float64   `json:"took,omitempty"`
	Error     *RPCError `json:"error,omitempty"`
}

// GetError is implementing RPCResponse interface
func (r *RPCResponseBody) GetError() *RPCError { return r.Error }

// RPCError is a struct sent by noobaa servers to denote an error response.
type RPCError struct {
	RPCCode string `json:"rpc_code"`
	Message string `json:"message"`
}

// Error is implementing the standard error type interface
func (e *RPCError) Error() string { return e.Message }

var _ error = &RPCError{}

// NewClient initializes an RPCClient with defaults
func NewClient(router RPCRouter) *RPCClient {
	return &RPCClient{
		Router: router,
		HTTPClient: http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

// Call an API method to noobaa.
// The response type should be defined to include RPCResponse inline - see below the specific response types.
// This is needed in order for json.Unmarshal() to decode into the reply structure.
func (c *RPCClient) Call(req RPCRequest) error {

	body, res := req.build()
	api := body.API
	method := body.Method
	if body.AuthToken == "" {
		body.AuthToken = c.AuthToken
	}
	address := c.Router.GetAddress(api)
	logger.Infof("RPC: %s.%s - Call to %s: %+v", api, method, address, req)

	reqBytes, err := json.Marshal(body)
	if err != nil {
		logger.Errorf("❌ RPC: %s.%s - Encoding request failed: %s", api, method, err)
		return err
	}

	httpRequest, err := http.NewRequest("PUT", address, bytes.NewReader(reqBytes))
	if err != nil {
		logger.Errorf("❌ RPC: %s.%s - Creating http request failed: %s", api, method, err)
		return err
	}

	httpResponse, err := c.HTTPClient.Do(httpRequest)
	defer func() {
		if httpResponse != nil && httpResponse.Body != nil {
			httpResponse.Body.Close()
		}
	}()
	if err != nil {
		logger.Errorf("❌ RPC: %s.%s - Sending http request failed: %s", api, method, err)
		return err
	}

	resBytes, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		logger.Errorf("❌ RPC: %s.%s - Reading http response failed: %s", api, method, err)
		return err
	}

	err = json.Unmarshal(resBytes, res)
	if err != nil {
		logger.Errorf("❌ RPC: %s.%s - Decoding response failed: %s", api, method, err)
		return err
	}

	if res.GetError() != nil {
		logger.Errorf("❌ RPC: %s.%s - Error: %+v", api, method, res.GetError())
		return res.GetError()
	}

	logger.Infof("✅ RPC: %s.%s - Response: %+v", api, method, res)
	return nil
}
