/*
Copyright 2021 The Rook Authors. All rights reserved.

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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewS3Agent(t *testing.T) {
	accessKey := "accessKey"
	secretKey := "secretKey"
	endpoint := "endpoint"

	t.Run("test without tls/debug", func(t *testing.T) {
		debug := false
		insecure := false
		s3Agent, err := NewS3Agent(accessKey, secretKey, endpoint, debug, nil, insecure, nil)
		assert.NoError(t, err)
		assert.NotNil(t, s3Agent)
		assert.NotNil(t, s3Agent.Client)
	})
	t.Run("test with debug without tls", func(t *testing.T) {
		debug := true
		insecure := false
		s3Agent, err := NewS3Agent(accessKey, secretKey, endpoint, debug, nil, insecure, nil)
		assert.NoError(t, err)
		assert.NoError(t, err)
		assert.NotNil(t, s3Agent)
		assert.NotNil(t, s3Agent.Client)
	})
	// t.Run("test without tls client cert but insecure tls", func(t *testing.T) {
	// 	debug := true
	// 	insecure := true

	// 	httpClient := &http.Client{}
	// 	s3Agent, err := NewS3Agent(accessKey, secretKey, endpoint, debug, nil, insecure, nil)
	// 	assert.NoError(t, err)
	// 	assert.NotNil(t, s3Agent)

	// 	// Inspect the HTTP client you passed in
	// 	transport, ok := httpClient.Transport.(*http.Transport)
	// 	assert.True(t, ok)
	// 	assert.NotNil(t, transport.TLSClientConfig)
	// 	assert.NotNil(t, transport.TLSClientConfig.RootCAs)
	// 	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	// })
	// t.Run("test with secure tls client cert", func(t *testing.T) {
	// 	debug := true
	// 	insecure := false
	// 	tlsCert := []byte("tlsCert")

	// 	httpClient := &http.Client{}
	// 	s3Agent, err := NewS3Agent(accessKey, secretKey, endpoint, debug, tlsCert, insecure, httpClient)
	// 	assert.NoError(t, err)
	// 	assert.NotNil(t, s3Agent)

	// 	transport, ok := httpClient.Transport.(*http.Transport)
	// 	assert.True(t, ok)
	// 	assert.NotNil(t, transport.TLSClientConfig)
	// 	assert.NotNil(t, transport.TLSClientConfig.RootCAs)
	// 	assert.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	// })
	// t.Run("test with insecure tls client cert", func(t *testing.T) {
	// 	debug := true
	// 	insecure := true
	// 	tlsCert := []byte("tlsCert")

	// 	httpClient := &http.Client{}
	// 	s3Agent, err := NewS3Agent(accessKey, secretKey, endpoint, debug, tlsCert, insecure, httpClient)
	// 	assert.NoError(t, err)
	// 	assert.NotNil(t, s3Agent)

	// 	// Check the TLS transport used in the client
	// 	transport, ok := httpClient.Transport.(*http.Transport)
	// 	assert.True(t, ok)
	// 	assert.NotNil(t, transport.TLSClientConfig)
	// 	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	// })
	// t.Run("test with custom http.Client", func(t *testing.T) {
	// 	debug := true
	// 	insecure := false
	// 	httpClient := &http.Client{
	// 		Transport: &http.Transport{
	// 			MaxIdleConns:        7,
	// 			MaxIdleConnsPerHost: 13,
	// 			MaxConnsPerHost:     17,
	// 		},
	// 	}
	// 	s3Agent, err := NewS3Agent(accessKey, secretKey, endpoint, debug, nil, insecure, httpClient)
	// 	assert.NoError(t, err)
	// 	assert.NotNil(t, s3Agent)

	// 	// Validate that the same client was used and configured
	// 	transport := httpClient.Transport.(*http.Transport)
	// 	assert.Equal(t, 7, transport.MaxIdleConns)
	// 	assert.Equal(t, 13, transport.MaxIdleConnsPerHost)
	// 	assert.Equal(t, 17, transport.MaxConnsPerHost)
	// 	assert.False(t, transport.TLSClientConfig.InsecureSkipVerify) // because insecure=false
	// })
}
