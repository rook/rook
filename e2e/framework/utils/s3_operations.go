package utils

import (
	"bytes"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type S3Helper struct {
	s3client *s3.S3
}

// create a s3 client for specfied endpoint and creds
func CreateNewS3Helper(endpoint string, keyId string, keySecret string) *S3Helper {

	creds := credentials.NewStaticCredentials(keyId, keySecret, "")

	// create aws s3 config, must use 'us-east-1' default aws region for ceph object store
	awsConfig := aws.NewConfig().
		WithRegion("us-east-1").
		WithCredentials(creds).
		WithEndpoint(endpoint).
		WithS3ForcePathStyle(true).
		WithDisableSSL(true)

	//create new session
	ses := session.New()

	//create new s3 client connection
	c := s3.New(ses, awsConfig)

	return &S3Helper{c}
}

func (h *S3Helper) CreateBucket(name string) (bool, error) {
	_, err := h.s3client.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(name),
	})
	if err != nil {
		return false, err

	}
	return true, nil
}

func (h *S3Helper) DeleteBucket(name string) (bool, error) {
	_, err := h.s3client.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(name),
	})
	if err != nil {
		return false, err

	}
	return true, nil
}

func (h *S3Helper) PutObjectInBucket(bucketname string, body string, key string,
	contentType string) (bool, error) {
	_, err := h.s3client.PutObject(&s3.PutObjectInput{
		Body:        strings.NewReader(body),
		Bucket:      &bucketname,
		Key:         &key,
		ContentType: &contentType,
	})
	if err != nil {
		return false, err

	}
	return true, nil
}

func (h *S3Helper) GetObjectInBucket(bucketname string, key string) (string, error) {
	result, err := h.s3client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucketname),
		Key:    aws.String(key),
	})

	if err != nil {
		return "ERROR_ OBJCET NOT FOUND", err

	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(result.Body)

	return buf.String(), nil
}
func (h *S3Helper) DeleteObjectInBucket(bucketname string, key string) (bool, error) {
	_, err := h.s3client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(bucketname),
		Key:    aws.String(key),
	})
	if err != nil {
		return false, err

	}
	return true, nil
}
