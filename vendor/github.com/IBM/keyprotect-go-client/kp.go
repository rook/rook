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

// keyprotect-go-client is a Go client library for interacting with the IBM KeyProtect service.
package kp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	rhttp "github.com/hashicorp/go-retryablehttp"

	"github.com/IBM/keyprotect-go-client/iam"
)

const (
	// DefaultBaseURL ...
	DefaultBaseURL = "https://us-south.kms.cloud.ibm.com"
	// DefaultTokenURL ..
	DefaultTokenURL = iam.IAMTokenURL

	// VerboseNone ...
	VerboseNone = 0
	// VerboseBodyOnly ...
	VerboseBodyOnly = 1
	// VerboseAll ...
	VerboseAll = 2
	// VerboseFailOnly ...
	VerboseFailOnly = 3
	// VerboseAllNoRedact ...
	VerboseAllNoRedact = 4

	authContextKey ContextKey = 0
	defaultTimeout            = 30 // in seconds.
)

var (
	// RetryWaitMax is the maximum time to wait between HTTP retries
	RetryWaitMax = 30 * time.Second

	// RetryMax is the max number of attempts to retry for failed HTTP requests
	RetryMax = 4

	cidCtxKey = ctxKey("X-Correlation-Id")
)

type ctxKey string

// ClientConfig ...
type ClientConfig struct {
	BaseURL       string
	Authorization string      // The IBM Cloud (Bluemix) access token
	APIKey        string      // Service ID API key, can be used instead of an access token
	TokenURL      string      // The URL used to get an access token from the API key
	InstanceID    string      // The IBM Cloud (Bluemix) instance ID that identifies your Key Protect service instance.
	KeyRing       string      // The ID of the target Key Ring the key is associated with. It is optional but recommended for better performance.
	Verbose       int         // See verbose values above
	Timeout       float64     // KP request timeout in seconds.
	Headers       http.Header // Support for Custom Header
}

// DefaultTransport ...
func DefaultTransport() http.RoundTripper {
	transport := &http.Transport{
		DisableKeepAlives:   true,
		MaxIdleConnsPerHost: -1,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}
	return transport
}

// API is deprecated. Use Client instead.
type API = Client

// Client holds configuration and auth information to interact with KeyProtect.
// It is expected that one of these is created per KeyProtect service instance/credential pair.
type Client struct {
	URL        *url.URL
	HttpClient http.Client
	Dump       Dump
	Config     ClientConfig
	Logger     Logger

	tokenSource iam.TokenSource
}

// New creates and returns a Client without logging.
func New(config ClientConfig, transport http.RoundTripper) (*Client, error) {
	return NewWithLogger(config, transport, nil)
}

// NewWithLogger creates and returns a Client with logging.  The
// error value will be non-nil if the config is invalid.
func NewWithLogger(config ClientConfig, transport http.RoundTripper, logger Logger) (*Client, error) {

	if transport == nil {
		transport = DefaultTransport()
	}

	if logger == nil {
		logger = NewLogger(func(args ...interface{}) {
			fmt.Println(args...)
		})
	}

	if config.Verbose > len(dumpers)-1 || config.Verbose < 0 {
		return nil, errors.New("verbose value is out of range")
	}

	if config.Timeout == 0 {
		config.Timeout = defaultTimeout
	}
	keysURL := fmt.Sprintf("%s/api/v2/", config.BaseURL)

	u, err := url.Parse(keysURL)
	if err != nil {
		return nil, err
	}

	ts := iam.CredentialFromAPIKey(config.APIKey)

	if config.TokenURL != "" {
		ts.TokenURL = config.TokenURL
	}

	c := &Client{
		URL: u,
		HttpClient: http.Client{
			Timeout:   time.Duration(config.Timeout * float64(time.Second)),
			Transport: transport,
		},
		Dump:        dumpers[config.Verbose],
		Config:      config,
		Logger:      logger,
		tokenSource: ts,
	}
	return c, nil
}

func (c *Client) newRequest(method, path string, body interface{}) (*http.Request, error) {

	u, err := c.URL.Parse(path)
	if err != nil {
		return nil, err
	}

	var reqBody []byte
	var buf io.Reader

	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
		buf = bytes.NewBuffer(reqBody)
	}

	request, err := http.NewRequest(method, u.String(), buf)
	if err != nil {
		return nil, err
	}

	request.Header.Set("accept", "application/json")

	return request, nil
}

type reason struct {
	Code     string
	Message  string
	Status   int
	MoreInfo string
}

func (r reason) String() string {
	if r.MoreInfo != "" {
		return fmt.Sprintf("%s: %s - FOR_MORE_INFO_REFER: %s", r.Code, r.Message, r.MoreInfo)
	}
	return fmt.Sprintf("%s: %s", r.Code, r.Message)
}

type Error struct {
	URL           string   // URL of request that resulted in this error
	StatusCode    int      // HTTP error code from KeyProtect service
	Message       string   // error message from KeyProtect service
	BodyContent   []byte   // raw body content if more inspection is needed
	CorrelationID string   // string value of a UUID that uniquely identifies the request to KeyProtect
	Reasons       []reason // collection of reason types containing detailed error messages
}

// Error returns correlation id and error message string
func (e Error) Error() string {
	var extraVars string
	if e.Reasons != nil && len(e.Reasons) > 0 {
		extraVars = fmt.Sprintf(", reasons='%s'", e.Reasons)
	}

	return fmt.Sprintf("kp.Error: correlation_id='%v', msg='%s'%s", e.CorrelationID, e.Message, extraVars)
}

// URLError wraps an error from client.do() calls with a correlation ID from KeyProtect
type URLError struct {
	Err           error
	CorrelationID string
}

func (e URLError) Error() string {
	return fmt.Sprintf(
		"error during request to KeyProtect correlation_id='%s': %s", e.CorrelationID, e.Err.Error())
}

func (c *Client) do(ctx context.Context, req *http.Request, res interface{}) (*http.Response, error) {

	acccesToken, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	// retrieve the correlation id from the context. If not present, then a UUID will be
	// generated for the correlation ID and feed it into the request
	// KeyProtect will use this when it is set on a request header rather than generating its
	// own inside the service
	// if not present, we generate our own here because a connection error might actually
	// mean the request doesn't make it server side, so having a correlation ID locally helps
	// us know that when comparing with server side logs.
	corrID := c.getCorrelationID(ctx)

	req.Header.Set("bluemix-instance", c.Config.InstanceID)
	req.Header.Set("authorization", acccesToken)
	req.Header.Set("correlation-id", corrID)

	if c.Config.KeyRing != "" {
		req.Header.Set("x-kms-key-ring", c.Config.KeyRing)
	}
	// Adding check for Custom Header Input
	if c.Config.Headers != nil {
		for key, value := range c.Config.Headers {
			req.Header.Set(key, strings.Join(value, ","))
		}
	}

	// set request up to be retryable on 500-level http codes and client errors
	retryableClient := getRetryableClient(&c.HttpClient)
	retryableRequest, err := rhttp.FromRequest(req)
	if err != nil {
		return nil, err
	}

	response, err := retryableClient.Do(retryableRequest.WithContext(ctx))
	if err != nil {
		return nil, &URLError{err, corrID}
	}
	defer response.Body.Close()

	resBody, err := io.ReadAll(response.Body)
	redact := []string{c.Config.APIKey, req.Header.Get("authorization")}
	c.Dump(req, response, []byte{}, resBody, c.Logger, redact)
	if err != nil {
		return nil, err
	}

	type KPErrorMsg struct {
		Message string `json:"errorMsg,omitempty"`
		Reasons []reason
	}

	type KPError struct {
		Resources []KPErrorMsg `json:"resources,omitempty"`
	}

	switch response.StatusCode {
	case http.StatusCreated:
		if len(resBody) != 0 {
			if err := json.Unmarshal(resBody, res); err != nil {
				return nil, err
			}
		}
	case http.StatusOK:
		if err := json.Unmarshal(resBody, res); err != nil {
			return nil, err
		}
	case http.StatusNoContent:
	default:
		errMessage := string(resBody)
		var reasons []reason

		if strings.Contains(string(resBody), "errorMsg") {
			kperr := KPError{}
			json.Unmarshal(resBody, &kperr)
			if len(kperr.Resources) > 0 && len(kperr.Resources[0].Message) > 0 {
				errMessage = kperr.Resources[0].Message
				reasons = kperr.Resources[0].Reasons
			}
		}

		return nil, &Error{
			URL:           response.Request.URL.String(),
			StatusCode:    response.StatusCode,
			Message:       errMessage,
			BodyContent:   resBody,
			CorrelationID: corrID,
			Reasons:       reasons,
		}
	}

	return response, nil
}

// getRetryableClient returns a fully configured retryable HTTP client
func getRetryableClient(client *http.Client) *rhttp.Client {
	// build base client with the library defaults and override as neeeded
	rc := rhttp.NewClient()
	rc.Logger = nil
	rc.HTTPClient = client
	rc.RetryWaitMax = RetryWaitMax
	rc.RetryMax = RetryMax
	rc.CheckRetry = kpCheckRetry
	rc.ErrorHandler = rhttp.PassthroughErrorHandler
	return rc
}

// kpCheckRetry will retry on connection errors, server errors, and 429s (rate limit)
func kpCheckRetry(ctx context.Context, resp *http.Response, err error) (bool, error) {
	// do not retry on context.Canceled or context.DeadlineExceeded
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	if err != nil {
		return true, err
	}
	// Retry on connection errors, 500+ errors (except 501 - not implemented), and 429 - too many requests
	if resp.StatusCode == 0 || resp.StatusCode == 429 || (resp.StatusCode >= 500 && resp.StatusCode != 501) {
		return true, nil
	}

	return false, nil
}

// ContextKey provides a type to auth context keys.
type ContextKey int

// NewContextWithAuth ...
func NewContextWithAuth(parent context.Context, auth string) context.Context {
	return context.WithValue(parent, authContextKey, auth)
}

// getAccessToken returns the auth context from the given Context, or
// calls to the IAMTokenSource to retrieve an auth token.
func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	if ctx.Value(authContextKey) != nil {
		return ctx.Value(authContextKey).(string), nil
	}

	if len(c.Config.Authorization) > 0 {
		return c.Config.Authorization, nil
	}

	token, err := c.tokenSource.Token()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s %s", token.TokenType, token.AccessToken), nil
}

// getCorrelationId returns the correlation ID value from the given Context, or
// returns a new UUID if not present
func (c *Client) getCorrelationID(ctx context.Context) string {
	corrID := GetCorrelationID(ctx)
	if corrID == nil {
		return uuid.New().String()
	}

	return corrID.String()
}

// NewContextWithCorrelationID retuns a context containing the UUID
func NewContextWithCorrelationID(ctx context.Context, uuid *uuid.UUID) context.Context {
	return context.WithValue(ctx, cidCtxKey, uuid)
}

// GetCorrelationID returns the correlation ID from the context
func GetCorrelationID(ctx context.Context) *uuid.UUID {
	if id := ctx.Value(cidCtxKey); id != nil {
		return id.(*uuid.UUID)
	}
	return nil
}

func (c ctxKey) String() string {
	return string(c)
}

// Logger writes when called.
type Logger interface {
	Info(...interface{})
}

type logger struct {
	writer func(...interface{})
}

func (l *logger) Info(args ...interface{}) {
	l.writer(args...)
}

func NewLogger(writer func(...interface{})) Logger {
	return &logger{writer: writer}
}

var dumpers = []Dump{dumpNone, dumpBodyOnly, dumpAll, dumpFailOnly, dumpAllNoRedact}

// Dump writes various parts of an HTTP request and an HTTP response.
type Dump func(*http.Request, *http.Response, []byte, []byte, Logger, []string)

// Redact replaces various pieces of output.
type Redact func(string, []string) string

// dumpFailOnly calls dumpAll when the HTTP response isn't 200 (ok),
// 201 (created), or 204 (no content).
func dumpFailOnly(req *http.Request, rsp *http.Response, reqBody, resBody []byte, log Logger, redactStrings []string) {
	switch rsp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return
	}
	dumpAll(req, rsp, reqBody, resBody, log, redactStrings)
}

// dumpAll dumps the HTTP request and the HTTP response body.
func dumpAll(req *http.Request, rsp *http.Response, reqBody, resBody []byte, log Logger, redactStrings []string) {
	dumpRequest(req, rsp, log, redactStrings, redact)
	dumpBody(reqBody, resBody, log, redactStrings, redact)
}

// dumpAllNoRedact dumps the HTTP request and HTTP response body without redaction.
func dumpAllNoRedact(req *http.Request, rsp *http.Response, reqBody, resBody []byte, log Logger, redactStrings []string) {
	dumpRequest(req, rsp, log, redactStrings, noredact)
	dumpBody(reqBody, resBody, log, redactStrings, noredact)
}

// dumpBodyOnly dumps the HTTP response body.
func dumpBodyOnly(req *http.Request, rsp *http.Response, reqBody, resBody []byte, log Logger, redactStrings []string) {
	dumpBody(reqBody, resBody, log, redactStrings, redact)
}

// dumpNone does nothing.
func dumpNone(req *http.Request, rsp *http.Response, reqBody, resBody []byte, log Logger, redactStrings []string) {
}

// dumpRequest dumps the HTTP request.
func dumpRequest(req *http.Request, rsp *http.Response, log Logger, redactStrings []string, redact Redact) {
	// log.Info(redact(fmt.Sprint(req), redactStrings))
	// log.Info(redact(fmt.Sprint(rsp), redactStrings))
}

// dumpBody dumps the HTTP response body with redactions.
func dumpBody(reqBody, resBody []byte, log Logger, redactStrings []string, redact Redact) {
	// log.Info(string(redact(string(reqBody), redactStrings)))
	// Redact the access token and refresh token if it shows up in the reponnse body.  This will happen
	// when using an API Key
	var auth iam.Token
	if strings.Contains(string(resBody), "access_token") {
		err := json.Unmarshal(resBody, &auth)
		if err != nil {
			log.Info(err)
		}
		redactStrings = append(redactStrings, auth.AccessToken)
		redactStrings = append(redactStrings, auth.RefreshToken)
	}
	// log.Info(string(redact(string(resBody), redactStrings)))
}

// redact replaces substrings within the given string.
func redact(s string, redactStrings []string) string {
	if len(redactStrings) < 1 {
		return s
	}
	var a []string
	for _, s1 := range redactStrings {
		if s1 != "" {
			a = append(a, s1)
			a = append(a, "***Value redacted***")
		}
	}
	r := strings.NewReplacer(a...)
	return r.Replace(s)
}

// noredact does not perform redaction, and returns the given string.
func noredact(s string, redactStrings []string) string {
	return s
}

// Collection Metadata is generic and can be shared between multiple resource types
type CollectionMetadata struct {
	CollectionType  string `json:"collectionType"`
	CollectionTotal int    `json:"collectionTotal"`
	TotalCount      int    `json:"totalCount,omitempty"`
}

// ListsOptions struct to add the query parameters for list functions. Extensible.
type ListOptions struct {
	Limit      *uint32
	Offset     *uint32
	TotalCount *bool
}
