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
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		assert.NotNil(t, s3Agent.Client)
		assert.Equal(t, "http://endpoint", *s3Agent.Client.Options().BaseEndpoint)
		assert.Equal(t, aws.ClientLogMode(0), s3Agent.Client.Options().ClientLogMode)
	})
	t.Run("test with debug without tls", func(t *testing.T) {
		debug := true
		insecure := false
		s3Agent, err := NewS3Agent(accessKey, secretKey, endpoint, debug, nil, insecure, nil)
		assert.NoError(t, err)
		assert.NotNil(t, s3Agent.Client)
		assert.Equal(t, "http://endpoint", *s3Agent.Client.Options().BaseEndpoint)
		assert.Equal(t, aws.LogSigning, s3Agent.Client.Options().ClientLogMode)
	})
	t.Run("test without tls client cert but insecure tls", func(t *testing.T) {
		debug := true
		insecure := true
		s3Agent, err := NewS3Agent(accessKey, secretKey, endpoint, debug, nil, insecure, nil)
		assert.NoError(t, err)
		assert.NotNil(t, s3Agent.Client)
		assert.Equal(t, "https://endpoint", *s3Agent.Client.Options().BaseEndpoint)
		httpClient := s3Agent.Client.Options().HTTPClient.(*http.Client)
		assert.NotNil(t, httpClient.Transport.(*http.Transport).TLSClientConfig.RootCAs)
		assert.True(t, httpClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
	})
	t.Run("test with secure tls client cert", func(t *testing.T) {
		debug := true
		insecure := false
		tlsCert := []byte("tlsCert")
		s3Agent, err := NewS3Agent(accessKey, secretKey, endpoint, debug, tlsCert, insecure, nil)
		assert.NoError(t, err)
		assert.NotNil(t, s3Agent.Client)
		assert.Equal(t, "https://endpoint", *s3Agent.Client.Options().BaseEndpoint)
		httpClient := s3Agent.Client.Options().HTTPClient.(*http.Client)
		assert.NotNil(t, httpClient.Transport.(*http.Transport).TLSClientConfig.RootCAs)
		assert.False(t, httpClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
	})
	t.Run("test with insecure tls client cert", func(t *testing.T) {
		debug := true
		insecure := true
		tlsCert := []byte("tlsCert")
		s3Agent, err := NewS3Agent(accessKey, secretKey, endpoint, debug, tlsCert, insecure, nil)
		assert.NoError(t, err)
		assert.NotNil(t, s3Agent.Client)
		assert.Equal(t, "https://endpoint", *s3Agent.Client.Options().BaseEndpoint)
		httpClient := s3Agent.Client.Options().HTTPClient.(*http.Client)
		assert.NotNil(t, httpClient.Transport)
		assert.True(t, httpClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
	})
	t.Run("test with custom http.Client", func(t *testing.T) {
		debug := true
		insecure := false
		httpClient := &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        7,
				MaxIdleConnsPerHost: 13,
				MaxConnsPerHost:     17,
			},
		}
		s3Agent, err := NewS3Agent(accessKey, secretKey, endpoint, debug, nil, insecure, httpClient)
		assert.NoError(t, err)
		assert.NotNil(t, s3Agent.Client)
		assert.Equal(t, "http://endpoint", *s3Agent.Client.Options().BaseEndpoint)
		resolvedClient := s3Agent.Client.Options().HTTPClient.(*http.Client)
		transport := resolvedClient.Transport.(*http.Transport)
		assert.Equal(t, 7, transport.MaxIdleConns)
		assert.Equal(t, 13, transport.MaxIdleConnsPerHost)
		assert.Equal(t, 17, transport.MaxConnsPerHost)
	})
	t.Run("endpoint with host:port for TLS", func(t *testing.T) {
		ep := "rook-ceph-rgw-store.test-ns.svc:443"
		s3Agent, err := NewS3Agent(accessKey, secretKey, ep, false, nil, true, nil)
		assert.NoError(t, err)
		assert.Equal(t, "https://rook-ceph-rgw-store.test-ns.svc:443", *s3Agent.Client.Options().BaseEndpoint)
	})
	t.Run("endpoint with host:port without TLS", func(t *testing.T) {
		ep := "rook-ceph-rgw-store.test-ns.svc:80"
		s3Agent, err := NewS3Agent(accessKey, secretKey, ep, false, nil, false, nil)
		assert.NoError(t, err)
		assert.Equal(t, "http://rook-ceph-rgw-store.test-ns.svc:80", *s3Agent.Client.Options().BaseEndpoint)
	})
	t.Run("endpoint with full URL", func(t *testing.T) {
		ep := "https://rook-ceph-rgw-store.test-ns.svc:443"
		s3Agent, err := NewS3Agent(accessKey, secretKey, ep, false, nil, true, nil)
		assert.NoError(t, err)
		assert.Equal(t, "https://rook-ceph-rgw-store.test-ns.svc:443", *s3Agent.Client.Options().BaseEndpoint)
	})
}

// capturingRoundTripper records the request it is handed and answers with
// status and respBody, or 200 and an empty body when unset.
type capturingRoundTripper struct {
	status   int
	respBody string

	req  *http.Request
	body []byte
}

func (rt *capturingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.req = req
	if req.Body != nil {
		rt.body, _ = io.ReadAll(req.Body)
	}
	status := rt.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(rt.respBody)),
	}, nil
}

const (
	bucketAlreadyExistsBody     = `<Error><Code>BucketAlreadyExists</Code><Message>bucket placement differs</Message><BucketName>bkt</BucketName></Error>`
	bucketAlreadyOwnedByYouBody = `<Error><Code>BucketAlreadyOwnedByYou</Code><BucketName>bkt</BucketName></Error>`
)

func TestS3AgentCreateBucket(t *testing.T) {
	newAgent := func(t *testing.T, rt http.RoundTripper) *S3Agent {
		s3Agent, err := NewS3Agent("accessKey", "secretKey", "endpoint", false, nil, false, &http.Client{Transport: rt})
		require.NoError(t, err)
		return s3Agent
	}

	t.Run("no placement sends no CreateBucketConfiguration", func(t *testing.T) {
		rt := &capturingRoundTripper{}
		err := newAgent(t, rt).CreateBucket(context.TODO(), "bkt", "", "")
		assert.NoError(t, err)
		assert.Equal(t, http.MethodPut, rt.req.Method)
		assert.Contains(t, rt.req.URL.Path, "/bkt")
		assert.NotContains(t, string(rt.body), "CreateBucketConfiguration")
		assert.Empty(t, rt.req.Header.Get("X-Amz-Storage-Class"))
	})

	t.Run("placement is sent as a location constraint with an empty zonegroup", func(t *testing.T) {
		rt := &capturingRoundTripper{}
		err := newAgent(t, rt).CreateBucket(context.TODO(), "bkt", "loc-a", "")
		assert.NoError(t, err)
		assert.Equal(t, http.MethodPut, rt.req.Method)
		assert.Contains(t, rt.req.URL.Path, "/bkt")
		assert.Contains(t, string(rt.body), "<LocationConstraint>:loc-a</LocationConstraint>")
		assert.Empty(t, rt.req.Header.Get("X-Amz-Storage-Class"))
	})

	t.Run("storage class is sent as a signed header", func(t *testing.T) {
		rt := &capturingRoundTripper{}
		err := newAgent(t, rt).CreateBucket(context.TODO(), "bkt", "loc-a", "FOO")
		assert.NoError(t, err)
		assert.Contains(t, string(rt.body), "<LocationConstraint>:loc-a</LocationConstraint>")
		assert.Equal(t, "FOO", rt.req.Header.Get("X-Amz-Storage-Class"))
		// the header must be part of the SigV4 signature or RGW rejects it
		assert.Contains(t, rt.req.Header.Get("Authorization"), "x-amz-storage-class")
	})

	t.Run("storage class without a placement", func(t *testing.T) {
		rt := &capturingRoundTripper{}
		err := newAgent(t, rt).CreateBucket(context.TODO(), "bkt", "", "FOO")
		assert.NoError(t, err)
		assert.NotContains(t, string(rt.body), "CreateBucketConfiguration")
		assert.Equal(t, "FOO", rt.req.Header.Get("X-Amz-Storage-Class"))
	})

	t.Run("empty placement and storage class behave like CreateBucket", func(t *testing.T) {
		rt := &capturingRoundTripper{}
		err := newAgent(t, rt).CreateBucket(context.TODO(), "bkt", "", "")
		assert.NoError(t, err)
		assert.NotContains(t, string(rt.body), "CreateBucketConfiguration")
		assert.Empty(t, rt.req.Header.Get("X-Amz-Storage-Class"))
	})

	t.Run("BucketAlreadyExists is success without a placement request", func(t *testing.T) {
		rt := &capturingRoundTripper{status: http.StatusConflict, respBody: bucketAlreadyExistsBody}
		assert.NoError(t, newAgent(t, rt).CreateBucket(context.TODO(), "bkt", "", ""))
	})

	t.Run("BucketAlreadyExists is an error carrying RGW's message when a placement is requested", func(t *testing.T) {
		for name, request := range map[string][2]string{
			"placement":     {"loc-a", ""},
			"storage class": {"", "FOO"},
			"both":          {"loc-a", "FOO"},
		} {
			t.Run(name, func(t *testing.T) {
				rt := &capturingRoundTripper{status: http.StatusConflict, respBody: bucketAlreadyExistsBody}
				err := newAgent(t, rt).CreateBucket(context.TODO(), "bkt", request[0], request[1])
				require.Error(t, err)
				var alreadyExists *s3types.BucketAlreadyExists
				assert.ErrorAs(t, err, &alreadyExists)
				assert.Contains(t, err.Error(), "bucket placement differs")
				if request[0] != "" {
					assert.Contains(t, err.Error(), `placement "loc-a"`)
				}
				if request[1] != "" {
					assert.Contains(t, err.Error(), `storage class "FOO"`)
				}
			})
		}
	})

	t.Run("BucketAlreadyOwnedByYou is success even with a placement request", func(t *testing.T) {
		rt := &capturingRoundTripper{status: http.StatusConflict, respBody: bucketAlreadyOwnedByYouBody}
		assert.NoError(t, newAgent(t, rt).CreateBucket(context.TODO(), "bkt", "loc-a", "FOO"))
	})

	t.Run("other RGW errors name the requested placement", func(t *testing.T) {
		rt := &capturingRoundTripper{
			status:   http.StatusBadRequest,
			respBody: `<Error><Code>InvalidLocationConstraint</Code><Message>The specified location-constraint is not valid</Message></Error>`,
		}
		err := newAgent(t, rt).CreateBucket(context.TODO(), "bkt", "nowhere", "FOO")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `failed to create bucket "bkt" with placement "nowhere" and storage class "FOO"`)
		assert.Contains(t, err.Error(), "InvalidLocationConstraint")
		assert.Contains(t, err.Error(), "The specified location-constraint is not valid")
	})
}

func TestDescribePlacement(t *testing.T) {
	tests := []struct {
		placement, storageClass, want string
	}{
		{"", "", ""},
		{"loc-a", "", ` with placement "loc-a"`},
		{"", "FOO", ` with storage class "FOO"`},
		{"loc-a", "FOO", ` with placement "loc-a" and storage class "FOO"`},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, describePlacement(tt.placement, tt.storageClass))
	}
}
