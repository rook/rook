package bucket

import (
	"regexp"
	"strings"

	"github.com/ceph/go-ceph/rgw/admin"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	apibkt "github.com/kube-object-storage/lib-bucket-provisioner/pkg/provisioner/api"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The Provisioner struct is a mix of "long lived" fields for the provisioner
// object instantiated by lib-bucket-provisioner and "short lived" fields used
// for each Provision/Grant/Delete/Revoke call. The intent for bucket struct is
// to incrementally migrate "short lived" request fields to the bucket struct
// and eventually to migrate methods as appropriate.
type bucket struct {
	provisioner      *Provisioner
	options          *apibkt.BucketOptions
	additionalConfig *additionalConfigSpec
}

// Retrieve the s3 access credentials for the rgw user.  The rgw user will be
// created if appropriate.
func (b *bucket) getUserCreds() (accessKeyID, secretAccessKey string, err error) {
	p := b.provisioner

	if b.additionalConfig.bucketOwner == nil {
		// get or create user
		accessKeyID, secretAccessKey, err = p.createCephUser(p.cephUserName)
		if err != nil {
			err = errors.Wrapf(err, "unable to create Ceph object user %q", p.cephUserName)
			return
		}
	} else {
		// only get an existing user
		var user admin.User
		user, err = p.adminOpsClient.GetUser(p.clusterInfo.Context, admin.User{ID: p.cephUserName})
		if err != nil {
			err = errors.Wrapf(err, "Ceph object user %q not found", p.cephUserName)
			return
		}
		accessKeyID = user.Keys[0].AccessKey
		secretAccessKey = user.Keys[0].SecretKey
	}

	return
}

func (p *Provisioner) bucketExists(name string) (bool, string, error) {
	bucket, err := p.adminOpsClient.GetBucketInfo(p.clusterInfo.Context, admin.Bucket{Bucket: name})
	if err != nil {
		if errors.Is(err, admin.ErrNoSuchBucket) {
			return false, "", nil
		}
		return false, "", errors.Wrapf(err, "failed to get ceph bucket %q", name)
	}
	return true, bucket.Owner, nil
}

// Create a Ceph user based on the passed-in name or a generated name. Return the
// accessKeys and set user name and keys in receiver.
func (p *Provisioner) createCephUser(username string) (accKey string, secKey string, err error) {
	if len(username) == 0 {
		return "", "", errors.Wrap(err, "no user name provided")
	}

	logger.Infof("creating Ceph object user %q", username)

	userConfig := admin.User{
		ID:          username,
		DisplayName: username,
	}

	var u admin.User
	u, err = p.adminOpsClient.GetUser(p.clusterInfo.Context, userConfig)
	if err != nil {
		if errors.Is(err, admin.ErrNoSuchUser) {
			u, err = p.adminOpsClient.CreateUser(p.clusterInfo.Context, userConfig)
			if err != nil {
				return "", "", errors.Wrapf(err, "failed to create Ceph object user %v", userConfig.ID)
			}
		} else {
			return "", "", errors.Wrapf(err, "failed to get Ceph object user %q", username)
		}
	} else {
		logger.Infof("Ceph object user %q already exists", username)
	}

	logger.Infof("successfully created Ceph object user %q with access keys", username)
	return u.Keys[0].AccessKey, u.Keys[0].SecretKey, nil
}

func (p *Provisioner) genUserName(obc *bktv1alpha1.ObjectBucketClaim) string {
	// A deterministic user name can be generated from the OBC's UID. We
	// cannot simply use the OBC's namespace and name, because they can be
	// reused, while the preceding bucket might be retained by reclaimPolicy.
	// But we still include namespace and name for usability. RadosGW user
	// names have a high enough length limit, so we don't need to crop the
	// "obc-namespace-name-" part.
	return "obc-" + obc.Namespace + "-" + obc.Name + "-" + string(obc.UID)
}

// Delete the user and bucket created by OBC with help of radosgw-admin commands
// If delete user failed, error is no longer returned since its permission is
// already revoked and hence user is no longer able to access the bucket
// Empty string is passed for bucketName only if user needs to be removed, ex Revoke()
func (p *Provisioner) deleteBucket(bucketName string) error {
	logger.Infof("deleting Ceph bucket %q", bucketName)
	// delete bucket with purge option to remove all objects
	thePurge := true
	err := p.adminOpsClient.RemoveBucket(p.clusterInfo.Context, admin.Bucket{Bucket: bucketName, PurgeObject: &thePurge})
	if err == nil {
		logger.Infof("bucket %q successfully deleted", bucketName)
	} else if errors.Is(err, admin.ErrNoSuchBucket) {
		// opinion: "not found" is not an error
		logger.Infof("bucket %q does not exist", bucketName)
	} else if errors.Is(err, admin.ErrNoSuchKey) {
		// ceph might return NoSuchKey than NoSuchBucket when the target bucket does not exist.
		// then we can use GetBucketInfo() to judge the existence of the bucket.
		// see: https://github.com/ceph/ceph/pull/44413
		_, err2 := p.adminOpsClient.GetBucketInfo(p.clusterInfo.Context, admin.Bucket{Bucket: bucketName, PurgeObject: &thePurge})
		if errors.Is(err2, admin.ErrNoSuchBucket) {
			logger.Infof("bucket info %q does not exist", bucketName)
		} else {
			return errors.Wrapf(err, "failed to delete bucket %q (could not get bucket info)", bucketName)
		}
	} else {
		return errors.Wrapf(err, "failed to delete bucket %q", bucketName)
	}

	return nil
}

// Delete the user *if it was created by OBC*. Will not delete externally
// managed users / users not created by obc.
func (p *Provisioner) deleteOBUser(ob *bktv1alpha1.ObjectBucket) error {
	// construct a partial obc object with only the fields set needed by
	// isObcGeneratedUser() & genUserName()
	obc := &bktv1alpha1.ObjectBucketClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ob.Spec.ClaimRef.Name,
			Namespace: ob.Spec.ClaimRef.Namespace,
			UID:       ob.Spec.ClaimRef.UID,
		},
	}

	if ob.Spec.Connection.AdditionalState != nil && ob.Spec.Connection.AdditionalState["bucketOwner"] != "" {
		obc.Spec.AdditionalConfig = map[string]string{
			"bucketOwner": ob.Spec.Connection.AdditionalState["bucketOwner"],
		}
	}

	// is the bucket owner a provisioner generated user?
	if p.isObcGeneratedUser(p.cephUserName, obc) {
		// delete the user
		logger.Infof("deleting Ceph user %q", p.cephUserName)

		err := p.adminOpsClient.RemoveUser(p.clusterInfo.Context, admin.User{ID: p.cephUserName})
		if err != nil {
			if errors.Is(err, admin.ErrNoSuchUser) {
				logger.Warningf("user %q does not exist, nothing to delete. %v", p.cephUserName, err)
			}
			logger.Warningf("failed to delete user %q. %v", p.cephUserName, err)
		} else {
			logger.Infof("user %q successfully deleted", p.cephUserName)
		}
	} else {
		// do not remove externally managed users
		logger.Infof("Ceph user %q does not look like an obc generated user and will not be removed", p.cephUserName)
	}

	return nil
}

// test a string to determine if is likely to be a user name generated by the provisioner
func (p *Provisioner) isObcGeneratedUser(userName string, obc *bktv1alpha1.ObjectBucketClaim) bool {
	// If the user name string is the same as the explicitly set bucketOwner we will
	// assume it is not a machine generated user name.
	if obc.Spec.AdditionalConfig != nil &&
		obc.Spec.AdditionalConfig["bucketOwner"] == userName {
		return false
	}

	// current format
	if userName == p.genUserName(obc) {
		return true
	}

	// historical format(s)
	if strings.HasPrefix(userName, "obc-"+obc.Namespace+"-"+obc.Name) {
		return true
	}

	matched, err := regexp.MatchString("^ceph-user-[0-9A-Za-z]{8}", userName)
	if err != nil {
		logger.Errorf("regex match failed. %v", err)
	}
	return matched
}
