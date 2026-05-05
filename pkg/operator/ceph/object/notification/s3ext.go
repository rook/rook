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

package notification

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	v4signer "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pkg/errors"
)

type DeleteBucketNotificationRequestInput struct {
	// Bucket is a required field
	Bucket *string

	// The account id of the expected bucket owner. If the bucket is owned by a
	// different account, the request will fail with an HTTP 403 (Access Denied)
	// error.
	ExpectedBucketOwner *string
}

func (s *DeleteBucketNotificationRequestInput) validate() error {
	if s.Bucket == nil || *s.Bucket == "" {
		return errors.New("Bucket is a required field")
	}
	return nil
}

// SHA-256 hash of an empty payload, used for signing requests with no body.
const emptyPayloadSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// DeleteBucketNotification sends a Ceph-specific DELETE request to remove a
// bucket notification configuration. This is not part of the standard AWS S3
// API, so we build and sign the HTTP request manually using the v2 client's
// credentials and endpoint.
func DeleteBucketNotification(ctx context.Context, client *s3v2.Client, input *DeleteBucketNotificationRequestInput, notificationId string) error {
	if input == nil {
		input = &DeleteBucketNotificationRequestInput{}
	}
	if err := input.validate(); err != nil {
		return err
	}

	opts := client.Options()

	baseEndpoint := ""
	if opts.BaseEndpoint != nil {
		baseEndpoint = strings.TrimRight(*opts.BaseEndpoint, "/")
	}

	reqURL := fmt.Sprintf("%s/%s?notification", baseEndpoint, *input.Bucket)
	if notificationId != "" {
		reqURL = fmt.Sprintf("%s/%s?notification=%s", baseEndpoint, *input.Bucket, notificationId)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return errors.Wrap(err, "failed to build HTTP request for DeleteBucketNotification")
	}

	req.Header.Set("x-amz-content-sha256", emptyPayloadSHA256)
	if input.ExpectedBucketOwner != nil {
		req.Header.Set("x-amz-expected-bucket-owner", *input.ExpectedBucketOwner)
	}

	creds, err := opts.Credentials.Retrieve(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve credentials for DeleteBucketNotification")
	}

	signer := v4signer.NewSigner()
	if err := signer.SignHTTP(ctx, creds, req, emptyPayloadSHA256, "s3", opts.Region, time.Now()); err != nil {
		return errors.Wrap(err, "failed to sign DeleteBucketNotification request")
	}

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to send DeleteBucketNotification request")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DeleteBucketNotification failed with HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
