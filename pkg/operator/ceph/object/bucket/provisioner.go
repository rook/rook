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
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/coreos/pkg/capnslog"
	"github.com/google/go-cmp/cmp"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	apibkt "github.com/kube-object-storage/lib-bucket-provisioner/pkg/provisioner/api"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/object"
	storagev1 "k8s.io/api/storage/v1"

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
	logger.Debugf("Provision event for OB options: %+v", options)

	err := p.initializeCreateOrGrant(options)
	if err != nil {
		return nil, err
	}
	logger.Infof("Provision: creating bucket %q for OBC %q", p.bucketName, options.ObjectBucketClaim.Name)

	p.accessKeyID, p.secretAccessKey, err = p.createCephUser(options.UserID)
	if err != nil {
		return nil, errors.Wrap(err, "Provision: can't create ceph user")
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
		logger.Debugf("creating bucket %q", p.bucketName)
		err = p.s3Agent.CreateBucket(p.bucketName)
		if err != nil {
			return nil, errors.Wrapf(err, "error creating bucket %q", p.bucketName)
		}
	} else if owner != options.UserID {
		return nil, errors.Errorf("bucket %q already exists and is owned by %q for different OBC", p.bucketName, owner)
	} else {
		logger.Debugf("bucket %q already exists", p.bucketName)
	}

	singleBucketQuota := 1
	_, err = p.adminOpsClient.ModifyUser(p.clusterInfo.Context, admin.User{ID: p.cephUserName, MaxBuckets: &singleBucketQuota})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set user %q bucket quota to %d", p.cephUserName, singleBucketQuota)
	}
	logger.Infof("set user %q bucket max to %d", p.cephUserName, singleBucketQuota)

	additionalConfig, err := additionalConfigSpecFromMap(options.ObjectBucketClaim.Spec.AdditionalConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process additionalConfig")
	}

	err = p.setAdditionalSettings(additionalConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set additional settings for OBC %q in NS %q associated with CephObjectStore %q in NS %q", options.ObjectBucketClaim.Name, options.ObjectBucketClaim.Namespace, p.objectStoreName, p.clusterInfo.Namespace)
	}

	return p.composeObjectBucket(), nil
}

// Grant attaches to an existing rgw bucket and returns a connection info
// representing the bucket's endpoint and user access credentials.
func (p Provisioner) Grant(options *apibkt.BucketOptions) (*bktv1alpha1.ObjectBucket, error) {
	logger.Debugf("Grant event for OB options: %+v", options)

	// initialize and set the AWS services and commonly used variables
	err := p.initializeCreateOrGrant(options)
	if err != nil {
		return nil, err
	}
	logger.Infof("Grant: allowing access to bucket %q for OBC %q", p.bucketName, options.ObjectBucketClaim.Name)

	// check and make sure the bucket exists
	logger.Infof("Checking for existing bucket %q", p.bucketName)
	if exists, _, err := p.bucketExists(p.bucketName); !exists {
		return nil, errors.Wrapf(err, "bucket %s does not exist", p.bucketName)
	}

	// get or create ceph user
	p.accessKeyID, p.secretAccessKey, err = p.createCephUser(options.UserID)
	if err != nil {
		return nil, errors.Wrap(err, "Provision: can't create ceph user")
	}

	// restrict creation of new buckets in rgw
	restrictBucketCreation := 0
	_, err = p.adminOpsClient.ModifyUser(p.clusterInfo.Context, admin.User{ID: p.cephUserName, MaxBuckets: &restrictBucketCreation})
	if err != nil {
		return nil, err
	}

	err = p.setS3Agent()
	if err != nil {
		return nil, err
	}

	additionalConfig, err := additionalConfigSpecFromMap(options.ObjectBucketClaim.Spec.AdditionalConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process additionalConfig")
	}

	// setting quota limit if it is enabled
	err = p.setAdditionalSettings(additionalConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to set additional settings for OBC %q in NS %q associated with CephObjectStore %q in NS %q", options.ObjectBucketClaim.Name, options.ObjectBucketClaim.Namespace, p.objectStoreName, p.clusterInfo.Namespace)
	}

	if additionalConfig.bucketPolicy != nil {
		// if the user is managing the bucket policy, there's nothing else to do
		return p.composeObjectBucket(), nil
	}

	// generate the bucket policy if it isn't managed by the user

	// if the policy does not exist, we'll create a new and append the statement to it
	policy, err := p.s3Agent.GetBucketPolicy(p.bucketName)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() != "NoSuchBucketPolicy" {
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

	logger.Infof("PutBucketPolicy output: %v", out)
	if err != nil {
		return nil, err
	}

	// returned ob with connection info
	return p.composeObjectBucket(), nil
}

// Delete is called when the ObjectBucketClaim (OBC) is deleted and the associated
// storage class' reclaimPolicy is "Delete". Or, if a Provision() error occurs and
// the bucket controller needs to clean up before retrying.
func (p Provisioner) Delete(ob *bktv1alpha1.ObjectBucket) error {
	logger.Debugf("Delete event for OB: %+v", ob)

	err := p.initializeDeleteOrRevoke(ob)
	if err != nil {
		return err
	}
	logger.Infof("Delete: deleting bucket %q for OB %q", p.bucketName, ob.Name)

	if err := p.deleteOBCResource(p.bucketName); err != nil {
		return errors.Wrapf(err, "failed to delete OBCResource bucket %q", p.bucketName)
	}
	return nil
}

// Revoke removes a user and creds from an existing bucket.
// Note: cleanup order below matters.
func (p Provisioner) Revoke(ob *bktv1alpha1.ObjectBucket) error {
	logger.Debugf("Revoke event for OB: %+v", ob)

	err := p.initializeDeleteOrRevoke(ob)
	if err != nil {
		return err
	}
	logger.Infof("Revoke: denying access to bucket %q for OB %q", p.bucketName, ob.Name)

	bucket, err := p.adminOpsClient.GetBucketInfo(p.clusterInfo.Context, admin.Bucket{Bucket: p.bucketName})
	if err != nil {
		logger.Errorf("%v", err)
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
			if aerr, ok := err.(awserr.Error); ok && aerr.Code() == "NoSuchBucketPolicy" {
				policy = nil
				logger.Errorf("no bucket policy for bucket %q, so no need to drop policy", p.bucketName)

			} else {
				logger.Errorf("error getting policy for bucket %q. %v", p.bucketName, err)
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
			logger.Infof("PutBucketPolicy output: %v", out)
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
			logger.Infof("principal %q ejected from bucket %q policy", p.cephUserName, p.bucketName)
		}
	}

	// finally, delete the user
	err = p.deleteOBCResource("")
	if err != nil {
		return errors.Wrapf(err, "failed to delete user %q", p.cephUserName)
	}

	return nil
}

// Return the OB struct with minimal fields filled in.
// initializeCreateOrGrant sets common provisioner receiver fields and
// the services and sessions needed to provision.
func (p *Provisioner) initializeCreateOrGrant(options *apibkt.BucketOptions) error {
	logger.Info("initializing and setting CreateOrGrant services")

	// set the bucket name
	obc := options.ObjectBucketClaim
	scName := options.ObjectBucketClaim.Spec.StorageClassName
	sc, err := p.getStorageClassWithBackoff(scName)
	if err != nil {
		logger.Errorf("failed to get storage class for OBC %q in namespace %q. %v", obc.Name, obc.Namespace, err)
		return err
	}

	// In most cases we assume the bucket is to be generated dynamically.  When a storage class
	// defines the bucket in the parameters, it's assumed to be a request to connect to a statically
	// created bucket.  In these cases, we forego generating a bucket.  Instead we connect a newly generated
	// user to the existing bucket.
	p.setBucketName(options.BucketName)
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

	if len(options.UserID) == 0 {
		return errors.Errorf("user ID for OBC %q is empty", obc.Name)
	}
	p.cephUserName = options.UserID

	return nil
}

func (p *Provisioner) initializeDeleteOrRevoke(ob *bktv1alpha1.ObjectBucket) error {

	sc, err := p.getStorageClassWithBackoff(ob.Spec.StorageClassName)
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
func (p *Provisioner) composeObjectBucket() *bktv1alpha1.ObjectBucket {

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

	domainName, port, _, err := store.GetAdvertiseEndpoint()
	if err != nil {
		return errors.Wrapf(err, `failed to get advertise endpoint for CephObjectStore "%s/%s"`, p.clusterInfo.Namespace, p.objectStoreName)
	}
	p.storeDomainName = domainName
	p.storePort = port

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
	return fmt.Sprintf("%s:%d", p.storeDomainName, p.storePort)
}

func (p *Provisioner) populateDomainAndPort(sc *storagev1.StorageClass) error {
	endpoint := getObjectStoreEndpoint(sc)
	// if endpoint is present, let's introspect it
	if endpoint != "" {
		p.storeDomainName = cephutil.GetIPFromEndpoint(endpoint)
		if p.storeDomainName == "" {
			return errors.New("failed to discover endpoint IP (is empty)")
		}
		p.storePort = cephutil.GetPortFromEndpoint(endpoint)
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
func (p *Provisioner) setAdditionalSettings(additionalConfig *additionalConfigSpec) error {
	err := p.setUserQuota(additionalConfig)
	if err != nil {
		return errors.Wrap(err, "failed to set user quota")
	}

	err = p.setBucketQuota(additionalConfig)
	if err != nil {
		return errors.Wrap(err, "failed to set bucket quota")
	}

	err = p.setBucketPolicy(additionalConfig)
	if err != nil {
		return errors.Wrap(err, "failed to set bucket policy")
	}

	return nil
}

func (p *Provisioner) setUserQuota(additionalConfig *additionalConfigSpec) error {
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
		logger.Debugf("Quota for user %q has changed. diff:%s", p.cephUserName, diff)
		// UID is not set in the QuotaSpec returned by GetUser()/GetUserQuota()
		targetQuota.UID = p.cephUserName
		err = p.adminOpsClient.SetUserQuota(p.clusterInfo.Context, targetQuota)
		if err != nil {
			return errors.Wrapf(err, "failed to set user %q quota enabled=%v %+v", p.cephUserName, quotaEnabled, additionalConfig)
		}
	}

	return nil
}

func (p *Provisioner) setBucketQuota(additionalConfig *additionalConfigSpec) error {
	bucket, err := p.adminOpsClient.GetBucketInfo(p.clusterInfo.Context, admin.Bucket{Bucket: p.bucketName})
	if err != nil {
		return errors.Wrapf(err, "failed to fetch bucket %q", p.bucketName)
	}
	liveQuota := bucket.BucketQuota

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
		logger.Debugf("Quota for bucket %q has changed. diff:%s", p.bucketName, diff)
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

func (p *Provisioner) setBucketPolicy(additionalConfig *additionalConfigSpec) error {
	svc := p.s3Agent.Client
	var livePolicy *string

	policyResp, err := svc.GetBucketPolicy(&s3.GetBucketPolicyInput{
		Bucket: &p.bucketName,
	})
	if err != nil {
		// when no policy is set, an err with code "NoSuchBucketPolicy" is returned
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() != "NoSuchBucketPolicy" {
				return errors.Wrapf(err, "failed to fetch policy for bucket %q", p.bucketName)
			}
		}
	} else {
		livePolicy = policyResp.Policy
	}

	diff := cmp.Diff(&livePolicy, additionalConfig.bucketPolicy)
	if diff == "" {
		// policy is in sync
		return nil
	}

	logger.Debugf("Policy for bucket %q has changed. diff:%s", p.bucketName, diff)
	if additionalConfig.bucketPolicy == nil {
		// if policy is out of sync and the new policy is nil, we should delete the live policy
		_, err = svc.DeleteBucketPolicy(&s3.DeleteBucketPolicyInput{
			Bucket: &p.bucketName,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to delete policy for bucket %q", p.bucketName)
		}
	} else {
		// set the new policy
		_, err = svc.PutBucketPolicy(&s3.PutBucketPolicyInput{
			Bucket: &p.bucketName,
			Policy: additionalConfig.bucketPolicy,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to set policy for bucket %q", p.bucketName)
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
	if objStore.Spec.Gateway.SecurePort == p.storePort {
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
