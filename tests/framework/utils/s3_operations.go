/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package utils

import (
	"bytes"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// S3Helper contains pointer to s3 client and wrappers for basic object store operations
type S3Helper struct {
	s3client *s3.S3
}

// CreateNewS3Helper creates a s3 client for specified endpoint and creds
func CreateNewS3Helper(endpoint string, keyID string, keySecret string) *S3Helper {

	creds := credentials.NewStaticCredentials(keyID, keySecret, "")

	// create aws s3 config, must use 'us-east-1' default aws region for ceph object store
	awsConfig := aws.NewConfig().
		WithRegion("us-east-1").
		WithCredentials(creds).
		WithEndpoint(endpoint).
		WithS3ForcePathStyle(true).
		WithDisableSSL(true).
		WithMaxRetries(20)

	// create new session
	ses := session.New()

	// create new s3 client connection
	c := s3.New(ses, awsConfig)

	return &S3Helper{c}
}

// CreateBucket function creates  bucket using s3 client
func (h *S3Helper) CreateBucket(name string) (bool, error) {
	_, err := h.s3client.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(name),
	})
	if err != nil {
		logger.Errorf("error encountered while creating bucket : %v", err)
		return false, err

	}
	return true, nil
}

// DeleteBucket function deletes given bucket using s3 client
func (h *S3Helper) DeleteBucket(name string) (bool, error) {
	_, err := h.s3client.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(name),
	})
	if err != nil {
		logger.Errorf("error encountered while deleting bucket : %v", err)
		return false, err

	}
	return true, nil
}

// PutObjectInBucket function puts an object in a bucket using s3 client
func (h *S3Helper) PutObjectInBucket(bucketname string, body string, key string,
	contentType string) (bool, error) {
	_, err := h.s3client.PutObject(&s3.PutObjectInput{
		Body:        strings.NewReader(body),
		Bucket:      &bucketname,
		Key:         &key,
		ContentType: &contentType,
	})
	if err != nil {
		logger.Errorf("error encountered while putting object in bucket : %v", err)
		return false, err

	}
	return true, nil
}

// GetObjectInBucket function retrieves an object from a bucket using s3 client
func (h *S3Helper) GetObjectInBucket(bucketname string, key string) (string, error) {
	result, err := h.s3client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucketname),
		Key:    aws.String(key),
	})

	if err != nil {
		logger.Errorf("error encountered while retrieving object from bucket : %v", err)
		return "ERROR_ OBJECT NOT FOUND", err

	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(result.Body)

	return buf.String(), nil
}

// DeleteObjectInBucket function deletes given bucket using s3 client
func (h *S3Helper) DeleteObjectInBucket(bucketname string, key string) (bool, error) {
	_, err := h.s3client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(bucketname),
		Key:    aws.String(key),
	})
	if err != nil {
		logger.Errorf("error encountered while deleting object from bucket : %v", err)
		return false, err

	}
	return true, nil
}

// IsBucketPresent function returns true if a bucket is present and false if it's not present
func (h *S3Helper) IsBucketPresent(bucketname string) (bool, error) {
	_, err := h.s3client.HeadBucket(&s3.HeadBucketInput{
		Bucket: aws.String(bucketname),
	})
	if err != nil && strings.Contains(err.Error(), "NotFound") {
		return false, nil
	} else if err == nil {
		return true, nil
	}
	return false, err
}
