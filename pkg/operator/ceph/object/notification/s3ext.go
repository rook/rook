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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	v4signer "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pkg/errors"
)

// FilterRule is a single name/value rule of a bucket notification filter.
type FilterRule struct {
	Name  string `xml:"Name"`
	Value string `xml:"Value"`
}

// FilterRules is a container of notification filter rules.
type FilterRules struct {
	FilterRules []FilterRule `xml:"FilterRule"`
}

// NotificationFilter is the filter of a bucket notification. In addition to the
// S3Key filter of the S3 API it carries the S3Metadata and S3Tags filters, which
// are a Ceph extension that the AWS SDK types cannot express.
type NotificationFilter struct {
	XMLName    xml.Name     `xml:"Filter"`
	S3Key      *FilterRules `xml:"S3Key,omitempty"`
	S3Metadata *FilterRules `xml:"S3Metadata,omitempty"`
	S3Tags     *FilterRules `xml:"S3Tags,omitempty"`
}

// TopicConfiguration is a single topic configuration of a bucket notification.
type TopicConfiguration struct {
	Id       string              `xml:"Id"`
	TopicArn string              `xml:"Topic"`
	Events   []string            `xml:"Event"`
	Filter   *NotificationFilter `xml:"Filter,omitempty"`
}

type notificationConfiguration struct {
	XMLName             xml.Name             `xml:"NotificationConfiguration"`
	TopicConfigurations []TopicConfiguration `xml:"TopicConfiguration"`
}

type PutBucketNotificationRequestInput struct {
	// Bucket is a required field
	Bucket *string

	TopicConfigurations []TopicConfiguration
}

func (s *PutBucketNotificationRequestInput) validate() error {
	if s.Bucket == nil || *s.Bucket == "" {
		return errors.New("Bucket is a required field")
	}
	return nil
}

// PutBucketNotification sends a bucket notification configuration to RGW. The
// typed SDK call is not used because its filter type has no member for the Ceph
// S3Metadata and S3Tags extensions, so the request is built and signed manually.
func PutBucketNotification(ctx context.Context, client *s3.Client, input *PutBucketNotificationRequestInput) error {
	if input == nil {
		input = &PutBucketNotificationRequestInput{}
	}
	if err := input.validate(); err != nil {
		return err
	}

	body, err := xml.Marshal(&notificationConfiguration{TopicConfigurations: input.TopicConfigurations})
	if err != nil {
		return errors.Wrap(err, "failed to marshal notification configuration")
	}
	payloadSum := sha256.Sum256(body)
	payloadSHA256 := hex.EncodeToString(payloadSum[:])

	opts := client.Options()

	baseEndpoint := ""
	if opts.BaseEndpoint != nil {
		baseEndpoint = strings.TrimRight(*opts.BaseEndpoint, "/")
	}

	reqURL := fmt.Sprintf("%s/%s?notification", baseEndpoint, *input.Bucket)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "failed to build HTTP request for PutBucketNotification")
	}

	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("x-amz-content-sha256", payloadSHA256)

	creds, err := opts.Credentials.Retrieve(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve credentials for PutBucketNotification")
	}

	signer := v4signer.NewSigner()
	if err := signer.SignHTTP(ctx, creds, req, payloadSHA256, "s3", opts.Region, time.Now()); err != nil {
		return errors.Wrap(err, "failed to sign PutBucketNotification request")
	}

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to send PutBucketNotification request")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PutBucketNotification failed with HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

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
func DeleteBucketNotification(ctx context.Context, client *s3.Client, input *DeleteBucketNotificationRequestInput, notificationId string) error {
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
