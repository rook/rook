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
	"net/http"

	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
)

type DeleteBucketNotificationRequestInput struct {
	_ struct{} `locationName:"DeleteBucketNotificationRequestInput" type:"structure"`

	// The name of the bucket for which to get the notification configuration.
	//
	// Bucket is a required field
	Bucket *string `location:"uri" locationName:"Bucket" type:"string" required:"true"`

	// The account id of the expected bucket owner. If the bucket is owned by a
	// different account, the request will fail with an HTTP 403 (Access Denied)
	// error.
	ExpectedBucketOwner *string `location:"header" locationName:"x-amz-expected-bucket-owner" type:"string"`
}

// String returns the string representation
func (s DeleteBucketNotificationRequestInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteBucketNotificationRequestInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DeleteBucketNotificationRequestInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DeleteBucketNotificationRequest"}
	if s.Bucket == nil {
		invalidParams.Add(request.NewErrParamRequired("Bucket"))
	}
	if s.Bucket != nil && len(*s.Bucket) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("Bucket", 1))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

const opDeleteBucketNotification = "DeleteBucketNotification"

func DeleteBucketNotificationRequest(c *s3.S3, input *DeleteBucketNotificationRequestInput, notificationId string) *request.Request {
	op := &request.Operation{
		Name:       opDeleteBucketNotification,
		HTTPMethod: http.MethodDelete,
		HTTPPath:   "/{Bucket}?notification",
	}

	if len(notificationId) > 0 {
		op.HTTPPath = "/{Bucket}?notification=" + notificationId
	}
	if input == nil {
		input = &DeleteBucketNotificationRequestInput{}
	}

	return c.NewRequest(op, input, nil)
}

func DeleteBucketNotification(c *s3.S3, input *DeleteBucketNotificationRequestInput, notificationId string) error {
	req := DeleteBucketNotificationRequest(c, input, notificationId)
	return req.Send()
}
