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

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	claimClient "github.com/kube-object-storage/lib-bucket-provisioner/pkg/client/clientset/versioned"
	apibkt "github.com/kube-object-storage/lib-bucket-provisioner/pkg/provisioner/api"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephutil "github.com/rook/rook/pkg/daemon/ceph/util"
	cephObject "github.com/rook/rook/pkg/operator/ceph/object"
)

type Provisioner struct {
	context         *clusterd.Context
	objectContext   *cephObject.Context
	bucketName      string
	claimClientset  claimClient.Interface
	storeDomainName string
	storePort       int32
	region          string
	// the namespace where the rook cluster should respond to events
	namespace string
	// access keys for acct for the bucket *owner*
	cephUserName    string
	accessKeyID     string
	secretAccessKey string

	// RunAsUser cephx user to use on radosgw-admin to create s3 users
	RunRgwCmdAsUser string

	s3Svc *s3.S3

	objectStoreName      string
	objectStoreNamespace string
	endpoint             string
	secretName           string
	secretNamespace      string
	additionalConfigData map[string]string
}

var _ apibkt.Provisioner = &Provisioner{}

func NewProvisioner(context *clusterd.Context, namespace, runRgwCmdAsUser string) *Provisioner {
	return &Provisioner{context: context, namespace: namespace, RunRgwCmdAsUser: runRgwCmdAsUser}
}

const maxBuckets = 1

// Provision creates an s3 bucket and returns a connection info
// representing the bucket's endpoint and user access credentials.
func (p Provisioner) Provision(options *apibkt.BucketOptions) (*bktv1alpha1.ObjectBucket, error) {

	err := p.initializeCreateOrGrant(options)
	if err != nil {
		return nil, err
	}
	logger.Infof("Provision: creating bucket %q for OBC %q", p.bucketName, options.ObjectBucketClaim.Name)

	// dynamically create a new ceph user
	p.accessKeyID, p.secretAccessKey, err = p.createCephUser("")
	if err != nil {
		return nil, errors.Wrapf(err, "Provision: can't create ceph user")
	}

	s3svc, err := NewS3Agent(p.accessKeyID, p.secretAccessKey, p.getObjectStoreEndpoint())
	if err != nil {
		return nil, err
	}

	// create the bucket
	err = s3svc.CreateBucket(p.bucketName)
	if err != nil {
		err = errors.Wrapf(err, "error creating bucket %q", p.bucketName)
		logger.Errorf(err.Error())
		p.deleteOBCResource("")
		return nil, err
	}

	_, errCode, err := cephObject.SetQuotaUserBucketMax(p.objectContext, p.cephUserName, maxBuckets)
	if errCode > 0 {
		p.deleteOBCResource(p.bucketName)
		return nil, err
	}
	logger.Infof("set user %q bucket max to %d", p.cephUserName, maxBuckets)

	return p.composeObjectBucket(), nil
}

// Grant attaches to an existing rgw bucket and returns a connection info
// representing the bucket's endpoint and user access credentials.
func (p Provisioner) Grant(options *apibkt.BucketOptions) (*bktv1alpha1.ObjectBucket, error) {

	// initialize and set the AWS services and commonly used variables
	err := p.initializeCreateOrGrant(options)
	if err != nil {
		return nil, err
	}
	logger.Infof("Grant: allowing access to bucket %q for OBC %q", p.bucketName, options.ObjectBucketClaim.Name)

	// check and make sure the bucket exists
	logger.Infof("Checking for existing bucket %q", p.bucketName)
	if exists, err := p.bucketExists(p.bucketName); !exists {
		return nil, errors.Wrapf(err, "bucket %s does not exist", p.bucketName)
	}

	p.accessKeyID, p.secretAccessKey, err = p.createCephUser("")
	if err != nil {
		return nil, err
	}

	// need to quota into -1 for restricting creation of new buckets in rgw
	_, _, err = cephObject.SetQuotaUserBucketMax(p.objectContext, p.cephUserName, -1)
	if err != nil {
		p.deleteOBCResource("")
		return nil, err
	}

	// get the bucket's owner via the bucket metadata
	stats, _, err := cephObject.GetBucket(p.objectContext, p.bucketName)
	if err != nil {
		p.deleteOBCResource("")
		return nil, errors.Wrapf(err, "could not get bucket stats (bucket: %s)", p.bucketName)
	}
	objectUser, _, err := cephObject.GetUser(p.objectContext, stats.Owner)
	if err != nil {
		p.deleteOBCResource("")
		return nil, errors.Wrapf(err, "could not get user (user: %s)", stats.Owner)
	}

	s3svc, err := NewS3Agent(*objectUser.AccessKey, *objectUser.SecretKey, p.getObjectStoreEndpoint())
	if err != nil {
		p.deleteOBCResource("")
		return nil, err
	}

	// if the policy does not exist, we'll create a new and append the statement to it
	policy, err := s3svc.GetBucketPolicy(p.bucketName)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() != "NoSuchBucketPolicy" {
				p.deleteOBCResource("")
				return nil, err
			}
		}
	}

	statement := NewPolicyStatement().
		WithSID(p.cephUserName).
		ForPrincipals(p.cephUserName).
		ForResources(p.bucketName).
		ForSubResources(p.bucketName).
		Allows().
		Actions(AllowedActions...)
	if policy == nil {
		policy = NewBucketPolicy(*statement)
	} else {
		policy = policy.ModifyBucketPolicy(*statement)
	}
	out, err := s3svc.PutBucketPolicy(p.bucketName, *policy)

	logger.Infof("PutBucketPolicy output: %v", out)
	if err != nil {
		p.deleteOBCResource("")
		return nil, err
	}
	// returned ob with connection info
	return p.composeObjectBucket(), nil
}

// Delete is called when the ObjectBucketClaim (OBC) is deleted and the associated
// storage class' reclaimPolicy is "Delete". Or, if a Provision() error occurs and
// the bucket controller needs to clean up before retrying.
func (p Provisioner) Delete(ob *bktv1alpha1.ObjectBucket) error {

	err := p.initializeDeleteOrRevoke(ob)
	if err != nil {
		return err
	}
	logger.Infof("Delete: deleting bucket %q for OB %q", p.bucketName, ob.Name)

	_, _, err = cephObject.UnlinkUser(p.objectContext, p.cephUserName, p.bucketName)
	if err != nil {
		return err
	}

	err = p.deleteOBCResource(p.bucketName)
	if err != nil {
		return err
	}
	return nil
}

// Revoke removes a user and creds from an existing bucket.
// Note: cleanup order below matters.
func (p Provisioner) Revoke(ob *bktv1alpha1.ObjectBucket) error {

	err := p.initializeDeleteOrRevoke(ob)
	if err != nil {
		return err
	}
	logger.Infof("Revoke: denying access to bucket %q for OB %q", p.bucketName, ob.Name)

	bucket, _, err := cephObject.GetBucket(p.objectContext, p.bucketName)
	if err != nil {
		logger.Errorf("%v", err)
	} else {
		if bucket.Owner == "" {
			return errors.New("cannot find bucket owner")
		}

		user, code, err := cephObject.GetUser(p.objectContext, bucket.Owner)
		// The user may not exist.  Ignore this in order to ensure the PolicyStatement does not contain the
		// stale user.
		if err != nil && code != cephObject.RGWErrorNotFound {
			return err
		} else if user == nil {
			logger.Errorf("querying user %q returned nil", p.cephUserName)
			return nil
		}

		s3svc, err := NewS3Agent(*user.AccessKey, *user.SecretKey, p.getObjectStoreEndpoint())
		if err != nil {
			return err
		}

		// Ignore cases where there is no bucket policy. This may have occurred if an error ended a Grant()
		// call before the policy was attached to the bucket
		policy, err := s3svc.GetBucketPolicy(p.bucketName)
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
			statement := NewPolicyStatement().
				WithSID(p.cephUserName).
				ForPrincipals(p.cephUserName).
				ForResources(p.bucketName).
				ForSubResources(p.bucketName).
				Denies().
				Actions(AllowedActions...)
			if policy == nil {
				policy = NewBucketPolicy(*statement)
			} else {
				policy = policy.ModifyBucketPolicy(*statement)
			}
			out, err := s3svc.PutBucketPolicy(p.bucketName, *policy)
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
			output, err := s3svc.PutBucketPolicy(p.bucketName, *policy)
			if err != nil {
				return err
			}
			logger.Infof("principal %q ejected from bucket %q policy. Output: %v", p.cephUserName, p.bucketName, output)
		}
	}

	// finally, delete the user
	p.deleteOBCResource("")
	return nil
}

// Return the OB struct with minimal fields filled in.
// initializeCreateOrGrant sets common provisioner receiver fields and
// the services and sessions needed to provision.
func (p *Provisioner) initializeCreateOrGrant(options *apibkt.BucketOptions) error {
	logger.Infof("initializing and setting CreateOrGrant services")

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
	p.setObjectStoreNamespace(sc)
	p.setRegion(sc)
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
	p.objectStoreNamespace = getObjectStoreNameSpace(sc)
	p.setEndpoint(sc)
	err = p.setObjectContext()
	if err != nil {
		return err
	}

	err = p.populateDomainAndPort(sc)
	if err != nil {
		return err
	}

	return nil
}

// Return the OB struct with minimal fields filled in.
func (p *Provisioner) composeObjectBucket() *bktv1alpha1.ObjectBucket {

	conn := &bktv1alpha1.Connection{
		Endpoint: &bktv1alpha1.Endpoint{
			BucketHost:           p.storeDomainName,
			BucketPort:           int(p.storePort),
			BucketName:           p.bucketName,
			Region:               p.region,
			AdditionalConfigData: p.additionalConfigData,
		},
		Authentication: &bktv1alpha1.Authentication{
			AccessKeys: &bktv1alpha1.AccessKeys{
				AccessKeyID:     p.accessKeyID,
				SecretAccessKey: p.secretAccessKey,
			},
		},
		AdditionalState: map[string]string{
			cephUser: p.cephUserName,
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
	} else if p.objectStoreNamespace == "" {
		return errors.Errorf(msg, "namespace")
	}
	p.objectContext = cephObject.NewContext(p.context, p.objectStoreName, p.objectStoreNamespace)
	p.objectContext.RunAsUser = p.RunRgwCmdAsUser

	return nil
}

// setObjectStoreDomainName sets the provisioner.storeDomainName and provisioner.port
// must be called after setObjectStoreName and setObjectStoreNamespace
func (p *Provisioner) setObjectStoreDomainName(sc *storagev1.StorageClass) error {

	name := getObjectStoreName(sc)
	namespace := getObjectStoreNameSpace(sc)
	// make sure the object store actually exists
	_, err := getObjectStore(p.context.RookClientset.CephV1(), p.objectStoreNamespace, p.objectStoreName)
	if err != nil {
		return err
	}
	p.storeDomainName = fmt.Sprintf("%s-%s.%s.%s", prefixObjectStoreSvc, name, namespace, suffixObjectStoreSvc)
	return nil
}

func (p *Provisioner) setObjectStorePort(sc *storagev1.StorageClass) error {
	name := getObjectStoreName(sc)
	name = fmt.Sprintf("%s-%s", prefixObjectStoreSvc, name)
	namespace := getObjectStoreNameSpace(sc)
	// also ensure the service exists and get the appropriate clusterIP port
	svc, err := getService(p.context.Clientset, namespace, name)
	if err != nil {
		return err
	}
	p.storePort = svc.Spec.Ports[0].Port
	return nil
}

func (p *Provisioner) setObjectStoreName(sc *storagev1.StorageClass) {
	p.objectStoreName = sc.Parameters[objectStoreName]
}

func (p *Provisioner) setObjectStoreNamespace(sc *storagev1.StorageClass) {
	p.objectStoreNamespace = sc.Parameters[objectStoreNamespace]
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

func (p *Provisioner) setRegion(sc *storagev1.StorageClass) {
	const key = "region"
	p.region = sc.Parameters[key]
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
		if err := p.setObjectStoreDomainName(sc); err != nil {
			return errors.Wrap(err, "failed to set object store domain name")
		}
		if err := p.setObjectStorePort(sc); err != nil {
			return errors.Wrap(err, "failed to set object store port")
		}
	}

	return nil
}
