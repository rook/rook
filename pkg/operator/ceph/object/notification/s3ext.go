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

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/request"
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

func DeleteBucketNotification(ctx context.Context, client *s3.Client, bucketName string, notificationId string) error {
	// Get the current configuration
	current, err := client.GetBucketNotificationConfiguration(ctx, &s3.GetBucketNotificationConfigurationInput{
		Bucket: &bucketName,
	})
	if err != nil {
		return err
	}

	// Filter out the notification to delete
	filtered := make([]s3types.TopicConfiguration, 0, len(current.TopicConfigurations))
	for _, config := range current.TopicConfigurations {
		if config.Id == nil || *config.Id != notificationId {
			filtered = append(filtered, config)
		}
	}

	// Update the bucket notification configuration
	_, err = client.PutBucketNotificationConfiguration(ctx, &s3.PutBucketNotificationConfigurationInput{
		Bucket: &bucketName,
		NotificationConfiguration: &s3types.NotificationConfiguration{
			TopicConfigurations: filtered,
		},
	})
	return err
}
