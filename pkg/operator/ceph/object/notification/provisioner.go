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

// Package notification to manage a rook bucket notifications.
package notification

import (
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/coreos/pkg/capnslog"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/object"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type provisioner struct {
	context          *clusterd.Context
	clusterInfo      *cephclient.ClusterInfo
	clusterSpec      *cephv1.ClusterSpec
	opManagerContext context.Context
	owner            string
	objectStoreName  types.NamespacedName
}

func getUserCredentials(opManagerContext context.Context, username string, objStore *cephv1.CephObjectStore, objContext *object.Context) (accessKey string, secretKey string, err error) {
	if len(username) == 0 {
		err = errors.New("no user name provided")
		return
	}

	adminAccessKey, adminSecretKey, err := object.GetAdminOPSUserCredentials(objContext, &objStore.Spec)
	if err != nil {
		err = errors.Wrapf(err, "failed to get Ceph RGW admin ops user credentials when getting user %q", username)
		return
	}

	adminOpsClient, err := admin.New(objContext.Endpoint, adminAccessKey, adminSecretKey, &http.Client{})
	if err != nil {
		err = errors.Wrapf(err, "failed to build admin ops API connection to get user %q", username)
		return
	}

	var u admin.User
	u, err = adminOpsClient.GetUser(opManagerContext, admin.User{ID: username})
	if err != nil {
		err = errors.Wrapf(err, "failed to get ceph user %q", username)
		return
	}

	logger.Infof("successfully fetched ceph user %q", username)
	accessKey = u.Keys[0].AccessKey
	secretKey = u.Keys[0].SecretKey
	return
}

func newS3Agent(p provisioner) (*object.S3Agent, error) {
	objStore, err := p.context.RookClientset.CephV1().CephObjectStores(p.objectStoreName.Namespace).Get(p.opManagerContext, p.objectStoreName.Name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get CephObjectStore %v", p.objectStoreName)
	}

	objContext, err := object.NewMultisiteContext(p.context, p.clusterInfo, objStore)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get object context for CephObjectStore %v", p.objectStoreName)
	}
	// CephClusterSpec is needed for GetAdminOPSUserCredentials()
	objContext.CephClusterSpec = *p.clusterSpec

	accessKey, secretKey, err := getUserCredentials(p.opManagerContext, p.owner, objStore, objContext)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get owner credentials for %q", p.owner)
	}

	return object.NewS3Agent(accessKey, secretKey, objContext.Endpoint, objContext.ZoneGroup, logger.LevelAt(capnslog.DEBUG), objContext.Context.KubeConfig.CertData)
}

// TODO: convert all rules without restrictions once the AWS SDK supports that
func createS3FilterRules(filterRules []cephv1.NotificationKeyFilterRule) (s3FilterRules []*s3.FilterRule) {
	for _, rule := range filterRules {
		s3FilterRules = append(s3FilterRules, &s3.FilterRule{
			Name:  &rule.DeepCopy().Name,
			Value: &rule.DeepCopy().Value,
		})
	}
	return
}

func createS3Filter(filter *cephv1.NotificationFilterSpec) *s3.NotificationConfigurationFilter {
	if filter == nil {
		return nil
	}
	return &s3.NotificationConfigurationFilter{
		Key: &s3.KeyFilter{
			FilterRules: createS3FilterRules(filter.KeyFilters),
		},
	}
}

func createS3Events(events []cephv1.BucketNotificationEvent) []*string {
	// in the AWS S3 library "Events" is required field
	// but in our CR it is optional. indicating notifications on all events
	if len(events) == 0 {
		createdEvent := "s3:ObjectCreated:*"
		removedEvent := "s3:ObjectRemoved:*"
		return []*string{&createdEvent, &removedEvent}
	}
	s3Events := []*string{}
	for _, event := range events {
		stringEvent := string(event)
		s3Events = append(s3Events, &stringEvent)
	}
	return s3Events
}

// Allow overriding this function for unit tests
var createNotificationFunc = createNotification

var createNotification = func(p provisioner, bucket *bktv1alpha1.ObjectBucket, topicARN string, notification *cephv1.CephBucketNotification) error {
	bucketName := bucket.Spec.Endpoint.BucketName
	bnName := types.NamespacedName{Namespace: notification.Namespace, Name: notification.Name}
	s3Agent, err := newS3Agent(p)
	if err != nil {
		return errors.Wrapf(err, "failed to create S3 agent for CephBucketNotification %q provisioning for bucket %q", bnName, bucketName)
	}
	_, err = s3Agent.Client.PutBucketNotificationConfiguration(&s3.PutBucketNotificationConfigurationInput{
		Bucket: &bucketName,
		NotificationConfiguration: &s3.NotificationConfiguration{
			TopicConfigurations: []*s3.TopicConfiguration{
				{
					Events:   createS3Events(notification.Spec.Events),
					Filter:   createS3Filter(notification.Spec.Filter),
					Id:       &notification.Name,
					TopicArn: &topicARN,
				},
			},
		},
	})
	if err != nil {
		return errors.Wrapf(err, "failed to provisioning CephBucketNotification %q for bucket %q", bnName, bucketName)
	}

	logger.Infof("CephBucketNotification %q was created for bucket %q", bnName, bucketName)

	return nil
}

// Allow overriding this function for unit tests
var deleteAllNotificationsFunc = deleteAllNotifications

var deleteAllNotifications = func(p provisioner, bucket *bktv1alpha1.ObjectBucket) error {
	bucketName := types.NamespacedName{Namespace: bucket.Namespace, Name: bucket.Name}
	s3Agent, err := newS3Agent(p)
	if err != nil {
		return errors.Wrapf(err, "failed to create S3 agent for deleting all bucket notifications from bucket %q", bucketName)
	}
	if err := DeleteBucketNotification(s3Agent.Client, &DeleteBucketNotificationRequestInput{
		Bucket: &bucket.Spec.Endpoint.BucketName,
	}); err != nil {
		return errors.Wrapf(err, "failed to delete all bucket notifications from bucket %q", bucketName)
	}

	logger.Infof("all bucket notifications deleted from bucket %q", bucketName)

	return nil
}
