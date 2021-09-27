/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"context"
	"fmt"
	"time"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	s3UserHealthCheckName   = "rook-ceph-internal-s3-user-checker"
	s3HealthCheckBucketName = "rook-ceph-bucket-checker"
	s3HealthCheckObjectBody = "Test Rook Object Data"
	s3HealthCheckObjectKey  = "rookHealthCheckTestObject"
	contentType             = "plain/text"
)

var (
	defaultHealthCheckInterval = 1 * time.Minute
)

// bucketChecker aggregates the mon/cluster info needed to check the health of the monitors
type bucketChecker struct {
	context         *clusterd.Context
	objContext      *AdminOpsContext
	interval        *time.Duration
	port            int32
	client          client.Client
	namespacedName  types.NamespacedName
	objectStoreSpec *cephv1.ObjectStoreSpec
}

// newbucketChecker creates a new HealthChecker object
func newBucketChecker(
	ctx *clusterd.Context, objContext *Context, client client.Client, namespacedName types.NamespacedName, objectStoreSpec *cephv1.ObjectStoreSpec,
) (*bucketChecker, error) {
	port, err := objectStoreSpec.GetPort()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create bucket checker for CephObjectStore %q", namespacedName.String())
	}

	opsCtx, err := NewMultisiteAdminOpsContext(objContext, objectStoreSpec)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create bucket checker for CephObjectStore %q", namespacedName.String())
	}

	c := &bucketChecker{
		context:         ctx,
		objContext:      opsCtx,
		interval:        &defaultHealthCheckInterval,
		port:            port,
		namespacedName:  namespacedName,
		client:          client,
		objectStoreSpec: objectStoreSpec,
	}

	// allow overriding the check interval
	checkInterval := objectStoreSpec.HealthCheck.Bucket.Interval
	if checkInterval != nil {
		logger.Infof("ceph rgw status check interval for object store %q is %q", namespacedName.Name, checkInterval.Duration.String())
		c.interval = &checkInterval.Duration
	}

	return c, nil
}

// checkObjectStore periodically checks the health of the cluster
func (c *bucketChecker) checkObjectStore(context context.Context) {
	// check the object store health immediately before starting the loop
	err := c.checkObjectStoreHealth()
	if err != nil {
		updateStatusBucket(c.client, c.namespacedName, cephv1.ConditionFailure, err.Error())
		logger.Debugf("failed to check rgw health for object store %q. %v", c.namespacedName.Name, err)
	}

	for {
		select {
		case <-context.Done():
			// purge bucket and s3 user
			// Needed for external mode where in converged everything goes away with the CR deletion
			c.cleanupHealthCheck()
			logger.Infof("stopping monitoring of rgw endpoints for object store %q", c.namespacedName.Name)
			return

		case <-time.After(*c.interval):
			logger.Debugf("checking rgw health of object store %q", c.namespacedName.Name)
			err := c.checkObjectStoreHealth()
			if err != nil {
				updateStatusBucket(c.client, c.namespacedName, cephv1.ConditionFailure, err.Error())
				logger.Debugf("failed to check rgw health for object store %q. %v", c.namespacedName.Name, err)
			}
		}
	}
}

func (c *bucketChecker) checkObjectStoreHealth() error {
	/*
		0. purge the s3 object by default
		1. create an S3 user
		2. always use the same user
		3. if already exists just re-hydrate the s3 credentials
		4. create a bucket with that user or use the existing one (always use the same bucket)
		5. create a check file
		6. get the hash of the file
		7. PUT the file
		8. GET the file
		9. compare hashes
		10. delete object on bucket
		11. update CR health status check

		Always keep the bucket and the user for the health check, just do PUT and GET because bucket creation is expensive
	*/

	// Keep admin ops context up-to date if there are config changes
	if err := UpdateEndpoint(&c.objContext.Context, c.objectStoreSpec); err != nil {
		return errors.Wrapf(err, "failed to parse updated CephObjectStore spec")
	}

	// Generate unique user and bucket name
	bucketName := genHealthCheckerBucketName(c.objContext.UID)
	userConfig := genUserCheckerConfig(c.objContext.UID)

	// Create checker user
	logger.Debugf("creating s3 user object %q for object store %q health check", userConfig.ID, c.namespacedName.Name)
	var user admin.User
	user, err := c.objContext.AdminOpsClient.GetUser(c.objContext.clusterInfo.Context, userConfig)
	if err != nil {
		if errors.Is(err, admin.ErrNoSuchUser) {
			user, err = c.objContext.AdminOpsClient.CreateUser(c.objContext.clusterInfo.Context, userConfig)
			if err != nil {
				return errors.Wrapf(err, "failed to create from ceph object user %v", userConfig.ID)
			}
		} else {
			return errors.Wrapf(err, "failed to get details from ceph object user %q", userConfig.ID)
		}
	}

	// Set access and secret key
	s3endpoint := c.objContext.Endpoint
	s3AccessKey := user.Keys[0].AccessKey
	s3SecretKey := user.Keys[0].SecretKey

	// Initiate s3 agent
	logger.Debugf("initializing s3 connection for object store %q", c.namespacedName.Name)
	s3client, err := NewInsecureS3Agent(s3AccessKey, s3SecretKey, s3endpoint, "", false)
	if err != nil {
		return errors.Wrap(err, "failed to initialize s3 connection")
	}

	// Force purge the s3 object before starting anything
	cleanupObjectHealthCheck(s3client, c.objContext.UID)

	// Bucket health test
	err = c.testBucketHealth(s3client, bucketName)
	if err != nil {
		return errors.Wrapf(err, "failed to run bucket health checks for object store %q", c.namespacedName.String())
	}

	logger.Debugf("successfully checked object store endpoint for object store %q", c.namespacedName.String())

	// Update the EndpointStatus in the CR to reflect the healthyness
	updateStatusBucket(c.client, c.namespacedName, cephv1.ConditionConnected, "")

	return nil
}

func cleanupObjectHealthCheck(s3client *S3Agent, objectStoreUID string) {
	bucketToDelete := genHealthCheckerBucketName(objectStoreUID)
	logger.Debugf("deleting object %q from bucket %q", s3HealthCheckObjectKey, bucketToDelete)
	_, err := s3client.DeleteObjectInBucket(bucketToDelete, s3HealthCheckObjectKey)
	if err != nil {
		logger.Errorf("failed to delete object in bucket. %v", err)
	}
}

func (c *bucketChecker) cleanupHealthCheck() {
	bucketToDelete := genHealthCheckerBucketName(c.objContext.UID)
	logger.Infof("deleting object %q from bucket %q in object store %q", s3HealthCheckObjectKey, bucketToDelete, c.namespacedName.Name)

	thePurge := true
	err := c.objContext.AdminOpsClient.RemoveBucket(c.objContext.clusterInfo.Context, admin.Bucket{Bucket: bucketToDelete, PurgeObject: &thePurge})
	if err != nil {
		if errors.Is(err, admin.ErrNoSuchBucket) {
			// opinion: "not found" is not an error
			logger.Debugf("bucket %q does not exist", bucketToDelete)
		} else {
			logger.Errorf("failed to delete bucket %q for object store %q. %v", bucketToDelete, c.namespacedName.Name, err)
		}
	}

	userToDelete := genUserCheckerConfig(c.objContext.UID)
	err = c.objContext.AdminOpsClient.RemoveUser(c.objContext.clusterInfo.Context, userToDelete)
	if err != nil && !errors.Is(err, admin.ErrNoSuchUser) {
		logger.Errorf("failed to delete object user %q for object store %q. %v", userToDelete.ID, c.namespacedName.Name, err)
	}

	logger.Debugf("successfully deleted object user %q for object store %q", userToDelete.ID, c.namespacedName.Name)
}

func toCustomResourceStatus(currentStatus *cephv1.BucketStatus, details string, health cephv1.ConditionType) *cephv1.BucketStatus {
	s := &cephv1.BucketStatus{
		Health:      health,
		LastChecked: time.Now().UTC().Format(time.RFC3339),
		Details:     details,
	}

	if currentStatus != nil {
		s.LastChanged = currentStatus.LastChanged
		if currentStatus.Details != s.Details {
			s.LastChanged = s.LastChecked
		}
	}
	return s
}

func genHealthCheckerBucketName(uuid string) string {
	return fmt.Sprintf("%s-%s", s3HealthCheckBucketName, uuid)
}

func genUserCheckerConfig(cephObjectStoreUID string) admin.User {
	userName := fmt.Sprintf("%s-%s", s3UserHealthCheckName, cephObjectStoreUID)

	return admin.User{
		ID:          userName,
		DisplayName: userName,
	}
}

func (c *bucketChecker) testBucketHealth(s3client *S3Agent, bucket string) error {
	// Purge on exit
	defer cleanupObjectHealthCheck(s3client, c.objContext.UID)

	// Create S3 bucket
	logger.Debugf("creating bucket %q", bucket)
	err := s3client.CreateBucketNoInfoLogging(bucket)
	if err != nil {
		return errors.Wrapf(err, "failed to create bucket %q for object store %q", bucket, c.namespacedName.Name)
	}

	// Put an object into the bucket
	logger.Debugf("putting object %q in bucket %q for object store %q", s3HealthCheckObjectKey, bucket, c.namespacedName.Name)
	_, err = s3client.PutObjectInBucket(bucket, string(s3HealthCheckObjectBody), s3HealthCheckObjectKey, contentType)
	if err != nil {
		return errors.Wrapf(err, "failed to put object %q in bucket %q for object store %q", s3HealthCheckObjectKey, bucket, c.namespacedName.Name)
	}

	// Get the object from the bucket
	logger.Debugf("getting object %q in bucket %q for object store %q", s3HealthCheckObjectKey, bucket, c.namespacedName.Name)
	read, err := s3client.GetObjectInBucket(bucket, s3HealthCheckObjectKey)
	if err != nil {
		return errors.Wrapf(err, "failed to get object %q in bucket %q for object store %q", s3HealthCheckObjectKey, bucket, c.namespacedName.Name)
	}

	// Compare the old and the existing object
	logger.Debugf("comparing objects hash for object store %q", c.namespacedName.Name)
	oldHash := k8sutil.Hash(s3HealthCheckObjectBody)
	currentHash := k8sutil.Hash(read)
	if currentHash != oldHash {
		return errors.Wrapf(err, "wrong file content, old file hash is %q and new one is %q for object store %q", oldHash, currentHash, c.namespacedName.Name)
	}

	return nil
}
