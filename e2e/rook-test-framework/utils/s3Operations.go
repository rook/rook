package utils

import (
	"bytes"
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
	svc := s3.New(session.New(&aws.Config{Endpoint: aws.String(endpoint),
		Credentials: credentials.NewStaticCredentials(keyId,
			keySecret, ""),
		Region: aws.String("us-west-2"),
	}))

	return &S3Helper{svc}
}

func (s3Help *S3Helper) CreateBucket(name string) (bool, error) {
	_, err := s3Help.s3client.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(name),
	})
	if err != nil {
		return false, err

	}
	return true, nil
}

func (s3Help *S3Helper) DeleteBucket(name string) (bool, error) {
	_, err := s3Help.s3client.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(name),
	})
	if err != nil {
		return false, err

	}
	return true, nil
}

func (s3Help *S3Helper) PutObjectInBucket(bucketname string, body string, key string,
	contentType string) (bool, error) {
	_, err := s3Help.s3client.PutObject(&s3.PutObjectInput{
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

func (s3Help *S3Helper) GetObjectInBucket(bucketname string, key string) (string, error) {
	result, err := s3Help.s3client.GetObject(&s3.GetObjectInput{
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
func (s3Help *S3Helper) DeleteObjectInBucket(bucketname string, key string) (bool, error) {
	_, err := s3Help.s3client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(bucketname),
		Key:    aws.String(key),
	})
	if err != nil {
		return false, err

	}
	return true, nil
}

/*func main() {
	*//*	fmt.Println("Hello World")
		os.Setenv("AWS_HOST","rook-rgw")
		os.Setenv("AWS_ENDPOINT","http://172.17.4.201:30169")
		os.Setenv("AWS_ACCESS_KEY_ID","WDHV55GA4VFELGPB2PVQ")
		os.Setenv("AWS_SECRET_ACCESS_KEY","FEbHX2CEpvBMMXYUap0LVo8SheWGAJkRMjloAYOA")*//*
	bucket := "testBkt"
	key := "mykey1"
	contentType := "text/plain"

	s3Ops := CreateNewS3Helper("http://172.17.4.201:30169", "WDHV55GA4VFELGPB2PVQ",
		"FEbHX2CEpvBMMXYUap0LVo8SheWGAJkRMjloAYOA")

	fmt.Println(s3Ops.CreateBucket(bucket))
	fmt.Println(s3Ops.PutObjectInBucket(bucket, "This is a a test", key, contentType))
	fmt.Println(s3Ops.GetObjectInBucket(bucket, key))
	//fmt.Println(s3Ops.DeleteObjectInBucket(bucket, key))
	//fmt.Println(s3Ops.DeleteBucket(bucket))

}*/
