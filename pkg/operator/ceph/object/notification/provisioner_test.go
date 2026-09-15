/*
Copyright 2025 The Rook Authors. All rights reserved.

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

// Package notification to manage rook bucket notifications.
package notification

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/stretchr/testify/assert"
)

func TestCreateS3Filter(t *testing.T) {
	tests := []struct {
		name     string
		filter   *cephv1.NotificationFilterSpec
		expected string
	}{
		{
			name:     "no filter",
			filter:   nil,
			expected: "",
		},
		{
			name: "key filters only",
			filter: &cephv1.NotificationFilterSpec{
				KeyFilters: []cephv1.NotificationKeyFilterRule{
					{Name: "prefix", Value: "hello"},
					{Name: "suffix", Value: ".png"},
				},
			},
			expected: "<Filter><S3Key><FilterRule><Name>prefix</Name><Value>hello</Value></FilterRule>" +
				"<FilterRule><Name>suffix</Name><Value>.png</Value></FilterRule></S3Key></Filter>",
		},
		{
			name: "metadata filters only",
			filter: &cephv1.NotificationFilterSpec{
				MetadataFilters: []cephv1.NotificationFilterRule{
					{Name: "x-amz-meta-color", Value: "blue"},
				},
			},
			expected: "<Filter><S3Metadata><FilterRule><Name>x-amz-meta-color</Name><Value>blue</Value>" +
				"</FilterRule></S3Metadata></Filter>",
		},
		{
			name: "tag filters only",
			filter: &cephv1.NotificationFilterSpec{
				TagFilters: []cephv1.NotificationFilterRule{
					{Name: "project", Value: "rook"},
				},
			},
			expected: "<Filter><S3Tags><FilterRule><Name>project</Name><Value>rook</Value>" +
				"</FilterRule></S3Tags></Filter>",
		},
		{
			name: "key, metadata and tag filters",
			filter: &cephv1.NotificationFilterSpec{
				KeyFilters: []cephv1.NotificationKeyFilterRule{
					{Name: "regex", Value: "[a-z]+"},
				},
				MetadataFilters: []cephv1.NotificationFilterRule{
					{Name: "x-amz-meta-color", Value: "blue"},
					{Name: "x-amz-meta-size", Value: "large"},
				},
				TagFilters: []cephv1.NotificationFilterRule{
					{Name: "project", Value: "rook"},
				},
			},
			expected: "<Filter><S3Key><FilterRule><Name>regex</Name><Value>[a-z]+</Value></FilterRule></S3Key>" +
				"<S3Metadata><FilterRule><Name>x-amz-meta-color</Name><Value>blue</Value></FilterRule>" +
				"<FilterRule><Name>x-amz-meta-size</Name><Value>large</Value></FilterRule></S3Metadata>" +
				"<S3Tags><FilterRule><Name>project</Name><Value>rook</Value></FilterRule></S3Tags></Filter>",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s3Filter := createS3Filter(test.filter)
			if test.expected == "" {
				assert.Nil(t, s3Filter)
				return
			}
			marshalled, err := xml.Marshal(s3Filter)
			assert.NoError(t, err)
			assert.Equal(t, test.expected, string(marshalled))
		})
	}
}

func TestCreateS3Events(t *testing.T) {
	assert.Equal(t, []string{"s3:ObjectCreated:*", "s3:ObjectRemoved:*"}, createS3Events(nil))
	assert.Equal(t, []string{"s3:ObjectCreated:Put"}, createS3Events([]cephv1.BucketNotificationEvent{"s3:ObjectCreated:Put"}))
}

func TestPutBucketNotification(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/my-bucket", r.URL.Path)
		assert.Contains(t, r.URL.Query(), "notification")
	}))
	defer server.Close()

	s3Agent, err := object.NewS3Agent("accesskey", "secretkey", server.URL, false, nil, false, nil)
	assert.NoError(t, err)

	bucketName := "my-bucket"
	err = PutBucketNotification(context.TODO(), s3Agent.Client, &PutBucketNotificationRequestInput{
		Bucket: &bucketName,
		TopicConfigurations: []TopicConfiguration{
			{
				Id:       "notification",
				TopicArn: "arn:aws:sns:rook::topic",
				Events:   createS3Events(nil),
				Filter: createS3Filter(&cephv1.NotificationFilterSpec{
					MetadataFilters: []cephv1.NotificationFilterRule{{Name: "x-amz-meta-color", Value: "blue"}},
					TagFilters:      []cephv1.NotificationFilterRule{{Name: "project", Value: "rook"}},
				}),
			},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t,
		"<NotificationConfiguration><TopicConfiguration><Id>notification</Id>"+
			"<Topic>arn:aws:sns:rook::topic</Topic>"+
			"<Event>s3:ObjectCreated:*</Event><Event>s3:ObjectRemoved:*</Event>"+
			"<Filter><S3Metadata><FilterRule><Name>x-amz-meta-color</Name><Value>blue</Value></FilterRule></S3Metadata>"+
			"<S3Tags><FilterRule><Name>project</Name><Value>rook</Value></FilterRule></S3Tags></Filter>"+
			"</TopicConfiguration></NotificationConfiguration>",
		string(body))
}
