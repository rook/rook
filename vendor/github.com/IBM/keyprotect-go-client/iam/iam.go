// Copyright 2019 IBM Corp.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package iam

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	rhttp "github.com/hashicorp/go-retryablehttp"
)

// IAMTokenURL is the global endpoint URL for the IAM token service
const IAMTokenURL = "https://iam.cloud.ibm.com/oidc/token"

var (
	// RetryWaitMax is the maximum time to wait between HTTP retries
	RetryWaitMax = 30 * time.Second

	// RetryMax is the max number of attempts to retry for failed HTTP requests
	RetryMax = 4
)

type TokenSource interface {
	Token() (*Token, error)
}

// CredentialFromAPIKey returns an IAMTokenSource that requests access tokens
// from the default token endpoint using an IAM API Key as the authentication mechanism
func CredentialFromAPIKey(apiKey string) *IAMTokenSource {
	return &IAMTokenSource{
		TokenURL: IAMTokenURL,
		APIKey:   apiKey,
	}
}

// Token represents an IAM credential used to authorize requests to another service.
type Token struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	Expiry       time.Time
}

func (t *Token) Valid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}

	if t.Expiry.Before(time.Now()) {
		return false
	}

	return true
}

// jsonToken is for deserializing the token from the response body
type jsonToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int32  `json:"expires_in"`
}

// getExpireTime uses local time and the ExpiresIn offset to calculate an
// expiration time based off our local clock, which is more accurate for
// us to determine when it expires relative to our client.
// we also pad the time a bit, because long running requests can fail
// mid-request if we send a soon-to-expire token along
func (jt jsonToken) getExpireTime() time.Time {
	// set the expiration time for 1 min less than the
	// actual time to prevent timeout errors
	return time.Now().Add(time.Duration(jt.ExpiresIn-60) * time.Second)
}

// IAMTokenSource is used to retrieve access tokens from the IAM token service.
// Most will probably want to use CredentialFromAPIKey to build an IAMTokenSource type,
// but it can also be created directly if one wishes to override the default IAM
// endpoint by setting TokenURL
type IAMTokenSource struct {
	TokenURL string
	APIKey   string

	mu sync.Mutex
	t  *Token
}

// Token requests an access token from IAM using the IAMTokenSource config.
func (ts *IAMTokenSource) Token() (*Token, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.t.Valid() {
		return ts.t, nil
	}

	if ts.APIKey == "" {
		return nil, errors.New("iam: APIKey is empty")
	}

	v := url.Values{}
	v.Set("grant_type", "urn:ibm:params:oauth:grant-type:apikey")
	v.Set("apikey", ts.APIKey)
	reqBody := []byte(v.Encode())

	u, err := url.Parse(ts.TokenURL)
	if err != nil {
		return nil, err
	}

	// NewRequest will calculate Content-Length if we pass it a bytes.Buffer
	// instead of a io.Reader type
	bodyBuf := bytes.NewBuffer(reqBody)
	request, err := rhttp.NewRequest("POST", u.String(), bodyBuf)
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	// use hashicorp retryable client with max wait time and attempts from module vars
	client := rhttp.NewClient()
	client.Logger = nil
	client.RetryWaitMax = RetryWaitMax
	client.RetryMax = RetryMax
	client.ErrorHandler = rhttp.PassthroughErrorHandler

	// need to use the go http DefaultTransport for tests to override with stubs (gock HTTP stubbing)
	client.HTTPClient = &http.Client{
		Timeout: time.Duration(60) * time.Second,
	}

	// this is the DefaultRetryPolicy but with retry on 429s as well
	client.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		// do not retry on context.Canceled or context.DeadlineExceeded
		if ctx.Err() != nil {
			return false, ctx.Err()
		}

		if err != nil {
			return true, err
		}

		// retry on connection error (code == 0), all 500s except 501, and 429s
		if resp.StatusCode == 0 || (resp.StatusCode >= 500 && resp.StatusCode != 501) || resp.StatusCode == 429 {
			return true, nil
		}

		return false, nil
	}

	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var iamErr Error
		if err = json.Unmarshal(buf.Bytes(), &iamErr); err != nil {
			return nil, err
		}
		iamErr.HTTPResponse = resp
		return nil, iamErr
	}

	var jToken jsonToken
	if err = json.Unmarshal(buf.Bytes(), &jToken); err != nil {
		return nil, err
	}

	token := &Token{
		AccessToken:  jToken.AccessToken,
		RefreshToken: jToken.RefreshToken,
		TokenType:    jToken.TokenType,
		Expiry:       jToken.getExpireTime(),
	}

	ts.t = token

	return token, nil
}

// Error is a type to hold error information that the IAM services sends back
// when a request cannot be completed. ErrorCode, ErrorMessage, and Context.RequestID
// are probably the most useful fields. IAM will most likely ask you for the RequestID
// if you ask for support.
//
// Also of note is that the http.Response object is included in HTTPResponse for
// error handling at the higher application levels.
type Error struct {
	ErrorCode    string             `json:"errorCode"`
	ErrorMessage string             `json:"errorMessage"`
	Context      *iamRequestContext `json:"context"`
	HTTPResponse *http.Response
}

type iamRequestContext struct {
	ClientIP    string `json:"clientIp"`
	ClusterName string `json:"clusterName"`
	Host        string `json:"host"`
	InstanceID  string `json:"instanceId"`
	RequestID   string `json:"requestId"`
	RequestType string `json:"requestType"`
	ElapsedTime string `json:"elapsedTime"`
	StartTime   string `json:"startTime"`
	EndTime     string `json:"endTime"`
	ThreadID    string `json:"threadId"`
	URL         string `json:"url"`
	UserAgent   string `json:"userAgent"`
	Locale      string `json:"locale"`
}

func (ie Error) Error() string {

	reqId := ""
	if ie.Context != nil {
		reqId = ie.Context.RequestID
	}

	statusCode := 0
	if ie.HTTPResponse != nil {
		statusCode = ie.HTTPResponse.StatusCode
	}

	return fmt.Sprintf("iam.Error: HTTP %d requestId='%s' message='%s %s'",
		statusCode, reqId, ie.ErrorCode, ie.ErrorMessage)
}
