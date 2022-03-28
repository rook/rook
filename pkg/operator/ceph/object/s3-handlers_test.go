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
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/assert"
)

func TestNewS3Agent(t *testing.T) {
	accessKey := "accessKey"
	secretKey := "secretKey"
	endpoint := "endpoint"

	t.Run("test without tls/debug", func(t *testing.T) {
		debug := false
		insecure := false
		s3Agent, err := newS3Agent(accessKey, secretKey, endpoint, debug, nil, insecure)
		assert.NoError(t, err)
		assert.NotEqual(t, aws.LogDebug, s3Agent.Client.Config.LogLevel)
		assert.Equal(t, nil, s3Agent.Client.Config.HTTPClient.Transport)
		assert.True(t, *s3Agent.Client.Config.DisableSSL)
	})
	t.Run("test with debug without tls", func(t *testing.T) {
		debug := true
		logLevel := aws.LogDebug
		insecure := false
		s3Agent, err := newS3Agent(accessKey, secretKey, endpoint, debug, nil, insecure)
		assert.NoError(t, err)
		assert.Equal(t, &logLevel, s3Agent.Client.Config.LogLevel)
		assert.Nil(t, s3Agent.Client.Config.HTTPClient.Transport)
		assert.True(t, *s3Agent.Client.Config.DisableSSL)
	})
	t.Run("test without tls client cert but insecure tls", func(t *testing.T) {
		debug := true
		insecure := true
		s3Agent, err := newS3Agent(accessKey, secretKey, endpoint, debug, nil, insecure)
		assert.NoError(t, err)
		assert.Nil(t, s3Agent.Client.Config.HTTPClient.Transport.(*http.Transport).TLSClientConfig.RootCAs)
		assert.True(t, s3Agent.Client.Config.HTTPClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
		assert.False(t, *s3Agent.Client.Config.DisableSSL)
	})
	t.Run("test with secure tls client cert", func(t *testing.T) {
		debug := true
		insecure := false
		tlsCert := []byte("tlsCert")
		s3Agent, err := newS3Agent(accessKey, secretKey, endpoint, debug, tlsCert, insecure)
		assert.NoError(t, err)
		assert.NotNil(t, s3Agent.Client.Config.HTTPClient.Transport.(*http.Transport).TLSClientConfig.RootCAs)
		assert.False(t, *s3Agent.Client.Config.DisableSSL)
	})
	t.Run("test with insesure tls client cert", func(t *testing.T) {
		debug := true
		insecure := true
		tlsCert := []byte("tlsCert")
		s3Agent, err := newS3Agent(accessKey, secretKey, endpoint, debug, tlsCert, insecure)
		assert.NoError(t, err)
		assert.NotNil(t, s3Agent.Client.Config.HTTPClient.Transport)
		assert.True(t, s3Agent.Client.Config.HTTPClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
		assert.False(t, *s3Agent.Client.Config.DisableSSL)
	})
}
