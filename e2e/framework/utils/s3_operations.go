package utils

import (
	"bytes"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"strings"
)

type S3Helper struct {
	s3client *s3.S3
}

func CreateNewS3Helper(endpoint string, keyId string, keySecret string) *S3Helper {
	creds := credentials.NewStaticCredentials(keyId, keySecret, "")
	_, err := creds.Get()
	if err != nil {
		fmt.Printf("bad credentials: %s", err)
	}
	cfg := aws.NewConfig().WithEndpoint(endpoint).WithRegion("us-west-1").WithCredentials(creds).WithDisableSSL(true)

	svc := s3.New(session.New(), cfg)

	return &S3Helper{svc}
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
