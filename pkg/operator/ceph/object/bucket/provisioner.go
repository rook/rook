/*
Copyright 2018 The Kubernetes Authors.

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

package bucket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/coreos/pkg/capnslog"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	apibkt "github.com/kube-object-storage/lib-bucket-provisioner/pkg/provisioner/api"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/util/log"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephutil "github.com/rook/rook/pkg/daemon/ceph/util"
)

type Provisioner struct {
	context         *clusterd.Context
	objectContext   *object.Context
	clusterInfo     *client.ClusterInfo
	bucketName      string
	storeDomainName string
	storePort       int32
	storeUseTLS     bool
	// access keys for acct for the bucket *owner*
	cephUserName         string
	accessKeyID          string
	secretAccessKey      string
	objectStoreName      string
	endpoint             string
	additionalConfigData map[string]string
	tlsCert              []byte
	insecureTLS          bool
	adminOpsClient       *admin.API
	s3Agent              *object.S3Agent
}

type additionalConfigSpec struct {
	maxObjects       *int64
	maxSize          *int64
	bucketMaxObjects *int64
	bucketMaxSize    *int64
	bucketPolicy     *string
	bucketLifecycle  *string
	bucketOwner      *string
}

var _ apibkt.Provisioner = &Provisioner{}

func NewProvisioner(context *clusterd.Context, clusterInfo *client.ClusterInfo) *Provisioner {
	return &Provisioner{context: context, clusterInfo: clusterInfo}
}

func (p Provisioner) GenerateUserID(obc *bktv1alpha1.ObjectBucketClaim, ob *bktv1alpha1.ObjectBucket) (string, error) {
	if ob != nil {
		return getCephUser(ob), nil
	}

	username := p.genUserName(obc)

	return username, nil
}

// Provision creates an s3 bucket and returns a connection info
// representing the bucket's endpoint and user access credentials.
func (p Provisioner) Provision(options *apibkt.BucketOptions) (*bktv1alpha1.ObjectBucket, error) {
	nsName := p.objectContext.NsName()
	log.NamedDebug(nsName, logger, "Provision event for OB options: %+v", options)

	additionalConfig, err := additionalConfigSpecFromMap(options.ObjectBucketClaim.Spec.AdditionalConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process additionalConfig")
	}

	bucket := &bucket{provisioner: &p, options: options, additionalConfig: additionalConfig}

	err = p.initializeCreateOrGrant(bucket)
	if err != nil {
		return nil, err
	}
	log.NamedInfo(nsName, logger, "Provision: creating bucket %q for OBC %q", p.bucketName, options.ObjectBucketClaim.Name)

	p.accessKeyID, p.secretAccessKey, err = bucket.getUserCreds()
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get user %q creds", p.cephUserName)
	}

	err = p.setS3Agent()
	if err != nil {
		return nil, err
	}

	// create the bucket
	var bucketExists bool
	var owner string
	bucketExists, owner, err = p.bucketExists(p.bucketName)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating bucket %q. failed to check if bucket already exists", p.bucketName)
	}
	if !bucketExists {
		// if bucket already exists, this returns error: TooManyBuckets because we set the quota
		// below. If it already exists, assume we are good to go
		log.NamedDebug(nsName, logger, "creating bucket %q owned by user %q", p.bucketName, p.cephUserName)
		err = p.s3Agent.CreateBucket(p.bucketName)
		if err != nil {
			return nil, errors.Wrapf(err, "error creating bucket %q", p.bucketName)
		}
	} else if owner != p.cephUserName {
		log.NamedDebug(nsName, logger, "bucket %q already exists and is owned by user %q instead of user %q, relinking...", p.bucketName, owner, p.cephUserName)

		err = p.adminOpsClient.LinkBucket(p.clusterInfo.Context, admin.BucketLinkInput{Bucket: p.bucketName, UID: p.cephUserName})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to link bucket %q to user %q", p.bucketName, p.cephUserName)
		}
	} else {
		log.NamedDebug(nsName, logger, "bucket %q already exists", p.bucketName)
	}

	// is the bucket owner a provisioner generated user?
	if p.isObcGeneratedUser(p.cephUserName, options.ObjectBucketClaim) {
		// set user quota
		singleBucketQuota := 1
		_, err = p.adminOpsClient.ModifyUser(p.clusterInfo.Context, admin.User{ID: p.cephUserName, MaxBuckets: &singleBucketQuota})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to set user %q bucket quota to %d", p.cephUserName, singleBucketQuota)
		}
		log.NamedInfo(nsName, logger, "set user %q bucket max to %d", p.cephUserName, singleBucketQuota)
	}

	err = p.setAdditionalSettings(bucket)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set additional settings for OBC %q in NS %q associated with CephObjectStore %q in NS %q", options.ObjectBucketClaim.Name, options.ObjectBucketClaim.Namespace, p.objectStoreName, p.clusterInfo.Namespace)
	}

	return p.composeObjectBucket(bucket), nil
}

// Grant attaches to an existing rgw bucket and returns a connection info
// representing the bucket's endpoint and user access credentials.
func (p Provisioner) Grant(options *apibkt.BucketOptions) (*bktv1alpha1.ObjectBucket, error) {
	nsName := p.objectContext.NsName()
	log.NamedDebug(nsName, logger, "Grant event for OB options: %+v", options)

	additionalConfig, err := additionalConfigSpecFromMap(options.ObjectBucketClaim.Spec.AdditionalConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process additionalConfig")
	}

	bucket := &bucket{provisioner: &p, options: options, additionalConfig: additionalConfig}

	// initialize and set the AWS services and commonly used variables
	err = p.initializeCreateOrGrant(bucket)
	if err != nil {
		return nil, err
	}
	log.NamedInfo(nsName, logger, "Grant: allowing access to bucket %q for OBC %q", p.bucketName, options.ObjectBucketClaim.Name)

	// check and make sure the bucket exists
	log.NamedInfo(nsName, logger, "Checking for existing bucket %q", p.bucketName)
	if exists, _, err := p.bucketExists(p.bucketName); !exists {
		return nil, errors.Wrapf(err, "bucket %s does not exist", p.bucketName)
	}

	p.accessKeyID, p.secretAccessKey, err = bucket.getUserCreds()
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get user %q creds", p.cephUserName)
	}

	// is the bucket owner a provisioner generated user?
	if p.isObcGeneratedUser(p.cephUserName, options.ObjectBucketClaim) {
		// restrict creation of new buckets in rgw
		restrictBucketCreation := 0
		_, err = p.adminOpsClient.ModifyUser(p.clusterInfo.Context, admin.User{ID: p.cephUserName, MaxBuckets: &restrictBucketCreation})
		if err != nil {
			return nil, err
		}
	}

	err = p.setS3Agent()
	if err != nil {
		return nil, err
	}

	// setting quota limit if it is enabled
	err = p.setAdditionalSettings(bucket)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set additional settings for OBC %q in NS %q associated with CephObjectStore %q in NS %q", options.ObjectBucketClaim.Name, options.ObjectBucketClaim.Namespace, p.objectStoreName, p.clusterInfo.Namespace)
	}

	if additionalConfig.bucketPolicy != nil {
		// if the user is managing the bucket policy, there's nothing else to do
		return p.composeObjectBucket(bucket), nil
	}

	// generate the bucket policy if it isn't managed by the user

	// if the policy does not exist, we'll create a new and append the statement to it
	policy, err := p.s3Agent.GetBucketPolicy(p.bucketName)
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			if apiErr.ErrorCode() != "NoSuchBucketPolicy" {
				return nil, err
			}
		}
	}

	statement := object.NewPolicyStatement().
		WithSID(p.cephUserName).
		ForPrincipals(p.cephUserName).
		ForResources(p.bucketName).
		ForSubResources(p.bucketName).
		Allows().
		Actions(object.AllowedActions...)
	if policy == nil {
		policy = object.NewBucketPolicy(*statement)
	} else {
		policy = policy.ModifyBucketPolicy(*statement)
	}
	out, err := p.s3Agent.PutBucketPolicy(p.bucketName, *policy)

	log.NamedInfo(nsName, logger, "PutBucketPolicy output: %v", out)
	if err != nil {
		return nil, err
	}

	// returned ob with connection info
	return p.composeObjectBucket(bucket), nil
}

// Delete is called when the ObjectBucketClaim (OBC) is deleted and the associated
// storage class' reclaimPolicy is "Delete". Or, if a Provision() error occurs and
// the bucket controller needs to clean up before retrying.
func (p Provisioner) Delete(ob *bktv1alpha1.ObjectBucket) error {
	nsName := p.objectContext.NsName()
	log.NamedDebug(nsName, logger, "Delete event for OB: %+v", ob)

	err := p.initializeDeleteOrRevoke(ob)
	if err != nil {
		return err
	}
	log.NamedInfo(nsName, logger, "Delete: deleting bucket %q for OB %q", p.bucketName, ob.Name)

	if err := p.deleteBucket(p.bucketName); err != nil {
		return errors.Wrapf(err, "failed to delete bucket %q", p.bucketName)
	}

	log.NamedInfo(nsName, logger, "Delete: deleting user %q for OB %q", p.bucketName, ob.Name)
	if err := p.deleteOBUser(ob); err != nil {
		return errors.Wrapf(err, "failed to delete user %q", p.cephUserName)
	}
	return nil
}

// Revoke removes a user and creds from an existing bucket.
// Note: cleanup order below matters.
func (p Provisioner) Revoke(ob *bktv1alpha1.ObjectBucket) error {
	nsName := p.objectContext.NsName()
	log.NamedDebug(nsName, logger, "Revoke event for OB: %+v", ob)

	err := p.initializeDeleteOrRevoke(ob)
	if err != nil {
		return err
	}
	log.NamedInfo(nsName, logger, "Revoke: denying access to bucket %q for OB %q", p.bucketName, ob.Name)

	bucket, err := p.adminOpsClient.GetBucketInfo(p.clusterInfo.Context, admin.Bucket{Bucket: p.bucketName})
	if err != nil {
		log.NamedError(nsName, logger, "%v", err)
	} else {
		if bucket.Owner == "" {
			return errors.Errorf("failed to find bucket %q owner", p.bucketName)
		}

		user, err := p.adminOpsClient.GetUser(p.clusterInfo.Context, admin.User{ID: bucket.Owner})
		if err != nil {
			if errors.Is(err, admin.ErrNoSuchUser) {
				// The user may not exist. Ignore this in order to ensure the PolicyStatement does not contain the
				// stale user.
				return nil
			}

			return err
		}

		p.accessKeyID = user.Keys[0].AccessKey
		p.secretAccessKey = user.Keys[0].SecretKey

		err = p.setS3Agent()
		if err != nil {
			return err
		}

		// Ignore cases where there is no bucket policy. This may have occurred if an error ended a Grant()
		// call before the policy was attached to the bucket
		policy, err := p.s3Agent.GetBucketPolicy(p.bucketName)
		if err != nil {
			var apiErr smithy.APIError
			if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchBucketPolicy" {
				policy = nil
				log.NamedError(nsName, logger, "no bucket policy for bucket %q, so no need to drop policy", p.bucketName)

			} else {
				log.NamedError(nsName, logger, "error getting policy for bucket %q. %v", p.bucketName, err)
				return err
			}
		}

		if bucket.Owner == p.cephUserName {
			statement := object.NewPolicyStatement().
				WithSID(p.cephUserName).
				ForPrincipals(p.cephUserName).
				ForResources(p.bucketName).
				ForSubResources(p.bucketName).
				Denies().
				Actions(object.AllowedActions...)
			if policy == nil {
				policy = object.NewBucketPolicy(*statement)
			} else {
				policy = policy.ModifyBucketPolicy(*statement)
			}
			out, err := p.s3Agent.PutBucketPolicy(p.bucketName, *policy)
			log.NamedInfo(nsName, logger, "PutBucketPolicy output: %v", out)
			if err != nil {
				return errors.Wrap(err, "failed to update policy")
			} else {
				return nil
			}
		}

		// drop policy if present
		if policy != nil {
			policy = policy.DropPolicyStatements(p.cephUserName)
			_, err := p.s3Agent.PutBucketPolicy(p.bucketName, *policy)
			if err != nil {
				return err
			}
			log.NamedInfo(nsName, logger, "principal %q ejected from bucket %q policy", p.cephUserName, p.bucketName)
		}
	}

	// finally, delete the user
	err = p.deleteOBUser(ob)
	if err != nil {
		return errors.Wrapf(err, "failed to delete user %q", p.cephUserName)
	}

	return nil
}

// Return the OB struct with minimal fields filled in.
// initializeCreateOrGrant sets common provisioner receiver fields and
// the services and sessions needed to provision.
func (p *Provisioner) initializeCreateOrGrant(bucket *bucket) error {
	nsName := p.objectContext.NsName()
	log.NamedInfo(nsName, logger, "initializing and setting CreateOrGrant services")

	// set the bucket name
	obc := bucket.options.ObjectBucketClaim
	scName := bucket.options.ObjectBucketClaim.Spec.StorageClassName
	sc, err := p.context.Clientset.StorageV1().StorageClasses().Get(p.clusterInfo.Context, scName, metav1.GetOptions{})
	if err != nil {
		log.NamedError(nsName, logger, "failed to get storage class for OBC %q in namespace %q. %v", obc.Name, obc.Namespace, err)
		return err
	}

	// In most cases we assume the bucket is to be generated dynamically.  When a storage class
	// defines the bucket in the parameters, it's assumed to be a request to connect to a statically
	// created bucket.  In these cases, we forego generating a bucket.  Instead we connect a newly generated
	// user to the existing bucket.
	p.setBucketName(bucket.options.BucketName)
	if bucketName, isStatic := isStaticBucket(sc); isStatic {
		p.setBucketName(bucketName)
	}

	p.setObjectStoreName(sc)
	p.setAdditionalConfigData(obc.Spec.AdditionalConfig)
	p.setEndpoint(sc)
	err = p.setObjectContext()
	if err != nil {
		return err
	}

	// If an endpoint is declared let's use it
	err = p.populateDomainAndPort(sc)
	if err != nil {
		return errors.Wrap(err, "failed to set domain and port")
	}
	err = p.setTlsCaCert()
	if err != nil {
		return errors.Wrapf(err, "failed to set CA cert for the OBC %q to connect with object store %q via TLS", obc.Name, p.objectStoreName)
	}

	// Set admin ops api client
	err = p.setAdminOpsAPIClient()
	if err != nil {
		// Replace the error with a nicer more comprehensive one
		// If the ceph config is not initialized yet, the radosgw-admin command will fail to retrieve the user
		if strings.Contains(err.Error(), opcontroller.OperatorNotInitializedMessage) {
			return errors.New(opcontroller.OperatorNotInitializedMessage)
		}
		return errors.Wrap(err, "failed to set admin ops api client")
	}

	if len(bucket.options.UserID) == 0 {
		return errors.Errorf("user ID for OBC %q is empty", obc.Name)
	}

	// override generated bucket owner name if an explicit name is set via additionalConfig["bucketOwner"]
	if bucketOwner := bucket.additionalConfig.bucketOwner; bucketOwner != nil {
		p.cephUserName = *bucketOwner
	} else {
		p.cephUserName = bucket.options.UserID
	}
	log.NamedDebug(nsName, logger, "Using user %q for OBC %q", p.cephUserName, obc.Name)

	return nil
}

func (p *Provisioner) initializeDeleteOrRevoke(ob *bktv1alpha1.ObjectBucket) error {
	sc, err := p.context.Clientset.StorageV1().StorageClasses().Get(p.clusterInfo.Context, ob.Spec.StorageClassName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get storage class for OB %q", ob.Name)
	}

	// set receiver fields from OB data
	p.setBucketName(getBucketName(ob))
	p.cephUserName = getCephUser(ob)
	p.objectStoreName = getObjectStoreName(sc)
	p.setEndpoint(sc)
	err = p.setObjectContext()
	if err != nil {
		return err
	}

	err = p.populateDomainAndPort(sc)
	if err != nil {
		return err
	}

	err = p.setTlsCaCert()
	if err != nil {
		return errors.Wrapf(err, "failed to set CA cert for the OB %q to connect with object store %q via TLS", ob.Name, p.objectStoreName)
	}

	// Set admin ops api client
	err = p.setAdminOpsAPIClient()
	if err != nil {
		// Replace the error with a nicer more comprehensive one
		// If the ceph config is not initialized yet, the radosgw-admin command will fail to retrieve the user
		if strings.Contains(err.Error(), opcontroller.OperatorNotInitializedMessage) {
			return errors.New(opcontroller.OperatorNotInitializedMessage)
		}
		return errors.Wrap(err, "failed to set admin ops api client")
	}

	return nil
}

// Return the OB struct with minimal fields filled in.
func (p *Provisioner) composeObjectBucket(bucket *bucket) *bktv1alpha1.ObjectBucket {
	conn := &bktv1alpha1.Connection{
		Endpoint: &bktv1alpha1.Endpoint{
			// if there are multiple endpoints on the object store, the OBC will get the endpoint
			// that it used for provisioning to make sure that the configmap gets a working config
			BucketHost:           p.storeDomainName,
			BucketPort:           int(p.storePort),
			BucketName:           p.bucketName,
			AdditionalConfigData: p.additionalConfigData,
		},
		Authentication: &bktv1alpha1.Authentication{
			AccessKeys: &bktv1alpha1.AccessKeys{
				AccessKeyID:     p.accessKeyID,
				SecretAccessKey: p.secretAccessKey,
			},
		},
		AdditionalState: map[string]string{
			CephUser:             p.cephUserName,
			ObjectStoreName:      p.objectStoreName,
			ObjectStoreNamespace: p.clusterInfo.Namespace,
		},
	}

	// bucketOwner will either match CephUser, indicating that it is an
	// explicitly set name, or the key will be unset, indicating that either the
	// provisioner created the user or a grant was made on a pre-existing bucket
	// linked to a pre-existing user.  Due to the semantics of lib-bucket, it
	// isn't possible to determine if it was a pre-existing bucket.
	if bucket.additionalConfig.bucketOwner != nil {
		conn.AdditionalState["bucketOwner"] = *bucket.additionalConfig.bucketOwner
	}

	return &bktv1alpha1.ObjectBucket{
		Spec: bktv1alpha1.ObjectBucketSpec{
			Connection: conn,
		},
	}
}

func (p *Provisioner) setObjectContext() error {
	msg := "error building object.Context: store %s cannot be empty"
	// p.endpoint means we point to an external cluster
	if p.objectStoreName == "" && p.endpoint == "" {
		return errors.Errorf(msg, "name")
	}

	// We don't need the CephObjectStore if an endpoint is provided
	// In 1.3, OBC external is working with an Endpoint (from the SC param) and in 1.4 we have a CephObjectStore but we must keep backward compatibility
	// In 1.4, the Endpoint from the SC is not expected and never used so we will enter the "else" condition which gets a CephObjectStore and it is present
	if p.endpoint != "" {
		p.objectContext = object.NewContext(p.context, p.clusterInfo, p.objectStoreName)
	} else {
		// Get CephObjectStore
		store, err := p.getObjectStore()
		if err != nil {
			return errors.Wrap(err, "failed to get cephObjectStore")
		}

		// Set multisite context
		p.objectContext, err = object.NewMultisiteContext(p.context, p.clusterInfo, store)
		if err != nil {
			return errors.Wrap(err, "failed to set multisite on provisioner's objectContext")
		}
	}

	return nil
}

// setObjectStoreDomainName sets the provisioner.storeDomainName and provisioner.port
// must be called after setObjectStoreName and setObjectStoreNamespace
func (p *Provisioner) setObjectStoreDomainNameAndPort(sc *storagev1.StorageClass) error {
	// make sure the object store actually exists
	store, err := p.getObjectStore()
	if err != nil {
		return err
	}

	domainName, port, useTLS, err := store.GetAdvertiseEndpoint()
	if err != nil {
		return errors.Wrapf(err, `failed to get advertise endpoint for CephObjectStore "%s/%s"`, p.clusterInfo.Namespace, p.objectStoreName)
	}
	p.storeDomainName = domainName
	p.storePort = port
	p.storeUseTLS = useTLS

	return nil
}

func (p *Provisioner) setObjectStoreName(sc *storagev1.StorageClass) {
	p.objectStoreName = sc.Parameters[ObjectStoreName]
}

func (p *Provisioner) setBucketName(name string) {
	p.bucketName = name
}

func (p *Provisioner) setAdditionalConfigData(additionalConfigData map[string]string) {
	if len(additionalConfigData) == 0 {
		additionalConfigData = make(map[string]string)
	}
	p.additionalConfigData = additionalConfigData
}

func (p *Provisioner) setEndpoint(sc *storagev1.StorageClass) {
	p.endpoint = sc.Parameters[objectStoreEndpoint]
}

func (p Provisioner) getObjectStoreEndpoint() string {
	if p.storeUseTLS {
		return object.BuildDNSEndpoint(p.storeDomainName, p.storePort, true)
	}
	return fmt.Sprintf("%s:%d", p.storeDomainName, p.storePort)
}

func (p *Provisioner) populateDomainAndPort(sc *storagev1.StorageClass) error {
	endpoint := getObjectStoreEndpoint(sc)
	// if endpoint is present, let's introspect it
	if endpoint != "" {
		endpointHostPort := endpoint
		p.storeUseTLS = false
		if u, err := url.Parse(endpoint); err == nil && u.Scheme != "" {
			if u.Scheme == "https" {
				p.storeUseTLS = true
			}
			endpointHostPort = u.Host
		}

		p.storeDomainName = cephutil.GetIPFromEndpoint(endpointHostPort)
		if p.storeDomainName == "" {
			return errors.New("failed to discover endpoint IP (is empty)")
		}
		p.storePort = cephutil.GetPortFromEndpoint(endpointHostPort)
		if p.storePort == 0 {
			return errors.New("failed to discover endpoint port (is empty)")
		}
		// If no endpoint exists let's see if CephObjectStore exists
	} else {
		if err := p.setObjectStoreDomainNameAndPort(sc); err != nil {
			return errors.Wrap(err, "failed to set object store domain name")
		}
	}

	return nil
}

// Check for additional options mentioned in OBC and set them accordingly
func (p *Provisioner) setAdditionalSettings(bucket *bucket) error {
	err := p.setUserQuota(bucket)
	if err != nil {
		return errors.Wrap(err, "failed to set user quota")
	}

	err = p.setBucketQuota(bucket)
	if err != nil {
		return errors.Wrap(err, "failed to set bucket quota")
	}

	err = p.setBucketPolicy(bucket)
	if err != nil {
		return errors.Wrap(err, "failed to set bucket policy")
	}

	err = p.setBucketLifecycle(bucket)
	if err != nil {
		return errors.Wrap(err, "failed to set bucket lifecycle")
	}

	return nil
}

func (p *Provisioner) setUserQuota(bucket *bucket) error {
	nsName := p.objectContext.NsName()
	additionalConfig := bucket.additionalConfig

	if additionalConfig.bucketOwner != nil {
		// when an explicit bucket owner is set, we do not manage user quotas
		log.NamedDebug(nsName, logger, "Skipping user level quotas for OBC %q as bucketOwner is set", bucket.options.ObjectBucketClaim.Name)
		return nil
	}

	liveQuota, err := p.adminOpsClient.GetUserQuota(p.clusterInfo.Context, admin.QuotaSpec{UID: p.cephUserName})
	if err != nil {
		return errors.Wrapf(err, "failed to fetch user %q", p.cephUserName)
	}

	// Copy only the fields that are actively managed by the provisioner to
	// prevent passing back undesirable combinations of fields.  It is
	// known to be problematic to set both MaxSize and MaxSizeKB.
	currentQuota := admin.QuotaSpec{
		Enabled:    liveQuota.Enabled,
		MaxObjects: liveQuota.MaxObjects,
		MaxSize:    liveQuota.MaxSize,
	}
	targetQuota := currentQuota

	// enable or disable quota for user
	quotaEnabled := (additionalConfig.maxObjects != nil) || (additionalConfig.maxSize != nil)

	targetQuota.Enabled = &quotaEnabled

	if additionalConfig.maxObjects != nil {
		targetQuota.MaxObjects = additionalConfig.maxObjects
	} else if currentQuota.MaxObjects != nil && *currentQuota.MaxObjects >= 0 {
		// if the existing value is already negative, we don't want to change it
		var objects int64 = -1
		targetQuota.MaxObjects = &objects
	}

	if additionalConfig.maxSize != nil {
		targetQuota.MaxSize = additionalConfig.maxSize
	} else if currentQuota.MaxSize != nil && *currentQuota.MaxSize >= 0 {
		// if the existing value is already negative, we don't want to change it
		var size int64 = -1
		targetQuota.MaxSize = &size
	}

	diff := cmp.Diff(currentQuota, targetQuota)
	if diff != "" {
		log.NamedDebug(nsName, logger, "Quota for user %q has changed. diff:%s", p.cephUserName, diff)
		// UID is not set in the QuotaSpec returned by GetUser()/GetUserQuota()
		targetQuota.UID = p.cephUserName
		err = p.adminOpsClient.SetUserQuota(p.clusterInfo.Context, targetQuota)
		if err != nil {
			return errors.Wrapf(err, "failed to set user %q quota enabled=%v %+v", p.cephUserName, quotaEnabled, additionalConfig)
		}
	}

	return nil
}

func (p *Provisioner) setBucketQuota(bucket *bucket) error {
	additionalConfig := bucket.additionalConfig
	nsName := p.objectContext.NsName()

	bkt, err := p.adminOpsClient.GetBucketInfo(p.clusterInfo.Context, admin.Bucket{Bucket: p.bucketName})
	if err != nil {
		return errors.Wrapf(err, "failed to fetch bucket %q", p.bucketName)
	}
	liveQuota := bkt.BucketQuota

	// Copy only the fields that are actively managed by the provisioner to
	// prevent passing back undesirable combinations of fields.  It is
	// known to be problematic to set both MaxSize and MaxSizeKB.
	currentQuota := admin.QuotaSpec{
		Enabled:    liveQuota.Enabled,
		MaxObjects: liveQuota.MaxObjects,
		MaxSize:    liveQuota.MaxSize,
	}
	targetQuota := currentQuota

	// enable or disable quota for user
	quotaEnabled := (additionalConfig.bucketMaxObjects != nil) || (additionalConfig.bucketMaxSize != nil)

	targetQuota.Enabled = &quotaEnabled

	if additionalConfig.bucketMaxObjects != nil {
		targetQuota.MaxObjects = additionalConfig.bucketMaxObjects
	} else if currentQuota.MaxObjects != nil && *currentQuota.MaxObjects >= 0 {
		// if the existing value is already negative, we don't want to change it
		var objects int64 = -1
		targetQuota.MaxObjects = &objects
	}

	if additionalConfig.bucketMaxSize != nil {
		targetQuota.MaxSize = additionalConfig.bucketMaxSize
	} else if currentQuota.MaxSize != nil && *currentQuota.MaxSize >= 0 {
		// if the existing value is already negative, we don't want to change it
		var size int64 = -1
		targetQuota.MaxSize = &size
	}

	diff := cmp.Diff(currentQuota, targetQuota)
	if diff != "" {
		log.NamedDebug(nsName, logger, "Quota for bucket %q has changed. diff:%s", p.bucketName, diff)
		// UID & Bucket are not set in the QuotaSpec returned by GetBucketInfo()
		targetQuota.UID = p.cephUserName
		targetQuota.Bucket = p.bucketName
		err = p.adminOpsClient.SetIndividualBucketQuota(p.clusterInfo.Context, targetQuota)
		if err != nil {
			return errors.Wrapf(err, "failed to set bucket %q quota enabled=%v %+v", p.bucketName, quotaEnabled, additionalConfig)
		}
	}

	return nil
}

func (p *Provisioner) setBucketPolicy(bucket *bucket) error {
	nsName := p.objectContext.NsName()
	additionalConfig := bucket.additionalConfig
	ctx := context.TODO()

	svc := p.s3Agent.ClientV2
	var livePolicy *string

	policyResp, err := svc.GetBucketPolicy(ctx, &s3v2.GetBucketPolicyInput{
		Bucket: &p.bucketName,
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			if apiErr.ErrorCode() != "NoSuchBucketPolicy" {
				return errors.Wrapf(err, "failed to fetch policy for bucket %q", p.bucketName)
			}
		}
	} else {
		livePolicy = policyResp.Policy
	}

	diff := cmp.Diff(livePolicy, additionalConfig.bucketPolicy)
	if diff == "" {
		// policy is in sync
		return nil
	}

	log.NamedDebug(nsName, logger, "Policy for bucket %q has changed. diff:%s", p.bucketName, diff)
	if additionalConfig.bucketPolicy == nil {
		_, err = svc.DeleteBucketPolicy(ctx, &s3v2.DeleteBucketPolicyInput{
			Bucket: &p.bucketName,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to delete policy for bucket %q", p.bucketName)
		}
	} else {
		_, err = svc.PutBucketPolicy(ctx, &s3v2.PutBucketPolicyInput{
			Bucket: &p.bucketName,
			Policy: additionalConfig.bucketPolicy,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to set policy for bucket %q", p.bucketName)
		}
	}

	return nil
}

func (p *Provisioner) setBucketLifecycle(bucket *bucket) error {
	nsName := p.objectContext.NsName()
	additionalConfig := bucket.additionalConfig
	ctx := context.TODO()

	svc := p.s3Agent.ClientV2
	var liveLc *s3v2.GetBucketLifecycleConfigurationOutput

	liveLc, err := svc.GetBucketLifecycleConfiguration(ctx, &s3v2.GetBucketLifecycleConfigurationInput{
		Bucket: &p.bucketName,
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchLifecycleConfiguration" {
			log.NamedDebug(nsName, logger, "no lifecycle configuration set for bucket %q", p.bucketName)
		} else {
			return errors.Wrapf(err, "failed to fetch lifecycle configuration for bucket %q", p.bucketName)
		}
	}

	confLc := &s3types.BucketLifecycleConfiguration{}
	if additionalConfig.bucketLifecycle != nil {
		err = json.Unmarshal([]byte(*additionalConfig.bucketLifecycle), confLc)
		if err != nil {
			return errors.Wrapf(err, "failed to unmarshal lifecycle configuration for bucket %q", p.bucketName)
		}
	}

	// Compare go structs directly rather than JSON serialization, since SDK
	// types don't use omitempty tags and String() output isn't valid JSON.
	var liveRules []s3types.LifecycleRule
	if liveLc != nil {
		liveRules = liveLc.Rules
	}
	diffLiveLc := &s3types.BucketLifecycleConfiguration{Rules: liveRules}

	// cmpopts.IgnoreUnexported is required because AWS SDK v2 types embed
	// an unexported noSmithyDocumentSerde field that cmp.Diff cannot handle.
	// This list must be updated if new s3types structs are used in lifecycle
	// rules (e.g. when RGW adds support for additional lifecycle features).
	ignoreUnexported := cmpopts.IgnoreUnexported(
		s3types.BucketLifecycleConfiguration{},
		s3types.LifecycleRule{},
		s3types.LifecycleExpiration{},
		s3types.LifecycleRuleFilter{},
		s3types.LifecycleRuleAndOperator{},
		s3types.AbortIncompleteMultipartUpload{},
		s3types.NoncurrentVersionExpiration{},
		s3types.NoncurrentVersionTransition{},
		s3types.Transition{},
		s3types.Tag{},
	)
	diff := cmp.Diff(diffLiveLc, confLc, ignoreUnexported)
	if diff == "" {
		return nil
	}

	log.NamedDebug(nsName, logger, "Lifecycle configuration for bucket %q has changed. diff:%s", p.bucketName, diff)
	if additionalConfig.bucketLifecycle == nil {
		_, err = svc.DeleteBucketLifecycle(ctx, &s3v2.DeleteBucketLifecycleInput{
			Bucket: &p.bucketName,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to delete lifecycle configuration for bucket %q", p.bucketName)
		}
	} else {
		_, err = svc.PutBucketLifecycleConfiguration(ctx, &s3v2.PutBucketLifecycleConfigurationInput{
			Bucket:                 &p.bucketName,
			LifecycleConfiguration: confLc,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to set lifecycle configuration for bucket %q", p.bucketName)
		}
	}

	return nil
}

func (p *Provisioner) setTlsCaCert() error {
	objStore, err := p.getObjectStore()
	if err != nil {
		return err
	}
	p.tlsCert = make([]byte, 0)
	if objStore.Spec.Gateway.SecurePort == p.storePort || p.storeUseTLS {
		p.tlsCert, p.insecureTLS, err = object.GetTlsCaCert(p.objectContext, &objStore.Spec)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Provisioner) setAdminOpsAPIClient() error {
	// Build TLS transport for the HTTP client if needed
	httpClient := &http.Client{
		Timeout: object.HttpTimeOut,
	}
	if p.tlsCert != nil {
		httpClient.Transport = object.BuildTransportTLS(p.tlsCert, p.insecureTLS)
	}

	// Fetch the ceph object store
	cephObjectStore, err := p.getObjectStore()
	if err != nil {
		return errors.Wrapf(err, "failed to get ceph object store %q", p.objectStoreName)
	}

	// Fetch the object store admin ops user
	accessKey, secretKey, err := object.GetAdminOPSUserCredentials(p.objectContext, &cephObjectStore.Spec)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve rgw admin ops user")
	}

	s3endpoint, err := object.GetAdminOpsEndpoint(cephObjectStore)
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve admin ops endpoint")
	}

	// If DEBUG level is set we will mutate the HTTP client for printing request and response
	if logger.LevelAt(capnslog.DEBUG) {
		p.adminOpsClient, err = admin.New(s3endpoint, accessKey, secretKey, object.NewDebugHTTPClient(httpClient, logger))
		if err != nil {
			return errors.Wrap(err, "failed to build admin ops API connection")
		}
	} else {
		p.adminOpsClient, err = admin.New(s3endpoint, accessKey, secretKey, httpClient)
		if err != nil {
			return errors.Wrap(err, "failed to build admin ops API connection")
		}
	}

	return nil
}

func (p *Provisioner) setS3Agent() error {
	var err error
	p.s3Agent, err = object.NewS3Agent(p.accessKeyID, p.secretAccessKey, p.getObjectStoreEndpoint(), logger.LevelAt(capnslog.DEBUG), p.tlsCert, p.insecureTLS, nil)
	return err
}
