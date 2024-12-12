/*
Copyright 2018 The Rook Authors. All rights reserved.

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
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	awssession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
)

// Region for aws golang sdk
const CephRegion = "us-east-1"

// S3Agent wraps the s3.S3 structure to allow for wrapper methods
type S3Agent struct {
	Client *s3.S3
}

func NewS3Agent(accessKey, secretKey, endpoint string, debug bool, tlsCert []byte, insecure bool, httpClient *http.Client) (*S3Agent, error) {
	logLevel := aws.LogOff
	if debug {
		logLevel = aws.LogDebug
	}
	tlsEnabled := false
	if len(tlsCert) > 0 || insecure {
		tlsEnabled = true
	}
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: HttpTimeOut,
		}
		if tlsEnabled {
			httpClient.Transport = BuildTransportTLS(tlsCert, insecure)
		}
	}

	session, err := awssession.NewSession(
		aws.NewConfig().
			WithRegion(CephRegion).
			WithCredentials(credentials.NewStaticCredentials(accessKey, secretKey, "")).
			WithEndpoint(endpoint).
			WithS3ForcePathStyle(true).
			WithMaxRetries(5).
			WithDisableSSL(!tlsEnabled).
			WithHTTPClient(httpClient).
			WithLogLevel(logLevel),
	)
	if err != nil {
		return nil, err
	}
	svc := s3.New(session)
	return &S3Agent{
		Client: svc,
	}, nil
}

// CreateBucket creates a bucket with the given name
func (s *S3Agent) CreateBucketNoInfoLogging(name string) error {
	return s.createBucket(name, false)
}

// CreateBucket creates a bucket with the given name
func (s *S3Agent) CreateBucket(name string) error {
	return s.createBucket(name, true)
}

func (s *S3Agent) createBucket(name string, infoLogging bool) error {
	if infoLogging {
		logger.Infof("creating bucket %q", name)
	} else {
		logger.Debugf("creating bucket %q", name)
	}
	bucketInput := &s3.CreateBucketInput{
		Bucket: &name,
	}

	_, err := s.Client.CreateBucket(bucketInput)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			logger.Debugf("DEBUG: after s3 call, ok=%v, aerr=%v", ok, aerr)
			switch aerr.Code() {
			case s3.ErrCodeBucketAlreadyExists:
				logger.Debugf("bucket %q already exists", name)
				return nil
			case s3.ErrCodeBucketAlreadyOwnedByYou:
				logger.Debugf("bucket %q already owned by you", name)
				return nil
			}
		}
		return errors.Wrapf(err, "failed to create bucket %q", name)
	}

	if infoLogging {
		logger.Infof("successfully created bucket %q", name)
	} else {
		logger.Debugf("successfully created bucket %q", name)
	}
	return nil
}

// DeleteBucket function deletes given bucket using s3 client
func (s *S3Agent) DeleteBucket(name string) (bool, error) {
	_, err := s.Client.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(name),
	})
	if err != nil {
		logger.Errorf("failed to delete bucket. %v", err)
		return false, err

	}
	return true, nil
}

// PutObjectInBucket function puts an object in a bucket using s3 client
func (s *S3Agent) PutObjectInBucket(bucketname string, body string, key string,
	contentType string) (bool, error) {
	_, err := s.Client.PutObject(&s3.PutObjectInput{
		Body:        strings.NewReader(body),
		Bucket:      &bucketname,
		Key:         &key,
		ContentType: &contentType,
	})
	if err != nil {
		logger.Errorf("failed to put object in bucket. %v", err)
		return false, err

	}
	return true, nil
}

// GetObjectInBucket function retrieves an object from a bucket using s3 client
func (s *S3Agent) GetObjectInBucket(bucketname string, key string) (string, error) {
	result, err := s.Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucketname),
		Key:    aws.String(key),
	})

	if err != nil {
		logger.Errorf("failed to retrieve object from bucket. %v", err)
		return "ERROR_ OBJECT NOT FOUND", err

	}
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(result.Body)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// DeleteObjectInBucket function deletes given bucket using s3 client
func (s *S3Agent) DeleteObjectInBucket(bucketname string, key string) (bool, error) {
	_, err := s.Client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(bucketname),
		Key:    aws.String(key),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case s3.ErrCodeNoSuchBucket:
				return true, nil
			case s3.ErrCodeNoSuchKey:
				return true, nil
			}
		}
		logger.Errorf("failed to delete object from bucket. %v", err)
		return false, err

	}
	return true, nil
}

func BuildTransportTLS(tlsCert []byte, insecure bool) *http.Transport {
	//nolint:gosec // is enabled only for testing
	tlsConfig := &tls.Config{InsecureSkipVerify: insecure}
	var caCertPool *x509.CertPool
	var err error
	caCertPool, err = x509.SystemCertPool()
	if err != nil {
		logger.Warningf("failed to load system cert pool; continuing without loading system certs")
		caCertPool = x509.NewCertPool() // start with empty cert pool instead
	}
	if len(tlsCert) > 0 {
		caCertPool.AppendCertsFromPEM(tlsCert)
	}
	tlsConfig.RootCAs = caCertPool

	return &http.Transport{
		TLSClientConfig: tlsConfig,
	}
}
