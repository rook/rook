package admin

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

const (
	authRegion        = "default"
	service           = "s3"
	connectionTimeout = time.Second * 3
)

var (
	errNoEndpoint  = errors.New("endpoint not set")
	errNoAccessKey = errors.New("access key not set")
	errNoSecretKey = errors.New("secret key not set")
)

// HTTPClient interface that conforms to that of the http package's Client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// API struct for New Client
type API struct {
	AccessKey  string
	SecretKey  string
	Endpoint   string
	HTTPClient HTTPClient
}

// New returns client for Ceph RGW
func New(endpoint, accessKey, secretKey string, httpClient HTTPClient) (*API, error) {
	// validate endpoint
	if endpoint == "" {
		return nil, errNoEndpoint
	}

	// validate access key
	if accessKey == "" {
		return nil, errNoAccessKey
	}

	// validate secret key
	if secretKey == "" {
		return nil, errNoSecretKey
	}

	// If no client is passed initialize it
	if httpClient == nil {
		httpClient = &http.Client{Timeout: connectionTimeout}
	}

	return &API{
		Endpoint:   endpoint,
		AccessKey:  accessKey,
		SecretKey:  secretKey,
		HTTPClient: httpClient,
	}, nil
}

// call makes request to the RGW Admin Ops API
func (api *API) call(ctx context.Context, httpMethod, path string, args url.Values) (body []byte, err error) {
	// Build request
	request, err := http.NewRequestWithContext(ctx, httpMethod, buildQueryPath(api.Endpoint, path, args.Encode()), nil)
	if err != nil {
		return nil, err
	}

	// Build S3 authentication
	credCache := aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(api.AccessKey, api.SecretKey, ""))
	creds, err := credCache.Retrieve(ctx)
	if err != nil {
		return nil, err
	}

	signer := v4.NewSigner()
	// This was present in https://github.com/IrekFasikhov/go-rgwadmin/ but it seems that the lib works without it
	// Let's keep it here just in case something shows up
	// signer.DisableRequestBodyOverwrite = true

	// Sign in S3
	const emptyPayloadHash = "UNSIGNED-PAYLOAD"
	err = signer.SignHTTP(ctx, creds, request, emptyPayloadHash, service, authRegion, time.Now())
	if err != nil {
		return nil, err
	}

	// Send HTTP request
	resp, err := api.HTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Decode HTTP response
	decodedResponse, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	resp.Body = io.NopCloser(bytes.NewBuffer(decodedResponse))

	// Handle error in response
	if resp.StatusCode >= 300 {
		return nil, handleStatusError(decodedResponse)
	}

	return decodedResponse, nil
}
