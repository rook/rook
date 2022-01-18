package bucket

import (
	"fmt"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/pkg/errors"
)

func (p *Provisioner) bucketExists(name string) (bool, error) {
	_, err := p.adminOpsClient.GetBucketInfo(p.clusterInfo.Context, admin.Bucket{Bucket: name})
	if err != nil {
		if errors.Is(err, admin.ErrNoSuchBucket) {
			return false, nil
		}
		return false, errors.Wrapf(err, "failed to get ceph bucket %q", name)
	}
	return true, nil
}

func (p *Provisioner) userExists(name string) (bool, error) {
	_, err := p.adminOpsClient.GetUser(p.clusterInfo.Context, admin.User{ID: name})
	if err != nil {
		if errors.Is(err, admin.ErrNoSuchUser) {
			return false, nil
		} else {
			return false, errors.Wrapf(err, "failed to get ceph user %q", name)
		}
	}

	return true, nil
}

// Create a Ceph user based on the passed-in name or a generated name. Return the
// accessKeys and set user name and keys in receiver.
func (p *Provisioner) createCephUser(username string) (accKey string, secKey string, err error) {
	if len(username) == 0 {
		username, err = p.genUserName()
		if len(username) == 0 || err != nil {
			return "", "", errors.Wrap(err, "no user name provided and unable to generate a unique name")
		}
	}
	p.cephUserName = username

	logger.Infof("creating Ceph user %q", username)
	userConfig := admin.User{
		ID:          username,
		DisplayName: p.cephUserName,
	}

	var u admin.User
	u, err = p.adminOpsClient.GetUser(p.clusterInfo.Context, userConfig)
	if err != nil {
		if errors.Is(err, admin.ErrNoSuchUser) {
			u, err = p.adminOpsClient.CreateUser(p.clusterInfo.Context, userConfig)
			if err != nil {
				return "", "", errors.Wrapf(err, "failed to create ceph object user %v", userConfig.ID)
			}
		} else {
			return "", "", errors.Wrapf(err, "failed to get ceph user %q", username)
		}
	}

	logger.Infof("successfully created Ceph user %q with access keys", username)
	return u.Keys[0].AccessKey, u.Keys[0].SecretKey, nil
}

// returns "" if unable to generate a unique name.
func (p *Provisioner) genUserName() (genName string, err error) {
	const (
		maxTries = 10
		prefix   = "ceph-user"
	)

	notUnique := true
	// generate names and check that the user does not already exist.  otherwise,
	// radosgw-admin will just return the existing user.
	// when notUnique == true, the loop breaks and `name` contains the latest generated name
	for i := 0; notUnique && i < maxTries; i++ {
		genName = fmt.Sprintf("%s-%s", prefix, randomString(genUserLen))
		if notUnique, err = p.userExists(genName); err != nil {
			return "", err
		}
	}
	return genName, nil
}

// Delete the user and bucket created by OBC with help of radosgw-admin commands
// If delete user failed, error is no longer returned since its permission is
// already revoked and hence user is no longer able to access the bucket
// Empty string is passed for bucketName only if user needs to be removed, ex Revoke()
func (p *Provisioner) deleteOBCResource(bucketName string) error {

	logger.Infof("deleting Ceph user %q and bucket %q", p.cephUserName, bucketName)
	if len(bucketName) > 0 {
		// delete bucket with purge option to remove all objects
		thePurge := true
		err := p.adminOpsClient.RemoveBucket(p.clusterInfo.Context, admin.Bucket{Bucket: bucketName, PurgeObject: &thePurge})
		if err == nil {
			logger.Infof("bucket %q successfully deleted", p.bucketName)
		} else if errors.Is(err, admin.ErrNoSuchBucket) {
			// opinion: "not found" is not an error
			logger.Infof("bucket %q does not exist", p.bucketName)
		} else if errors.Is(err, admin.ErrNoSuchKey) {
			// ceph might return NoSuchKey than NoSuchBucket when the target bucket does not exist.
			// then we can use GetBucketInfo() to judge the existence of the bucket.
			// see: https://github.com/ceph/ceph/pull/44413
			_, err2 := p.adminOpsClient.GetBucketInfo(p.clusterInfo.Context, admin.Bucket{Bucket: bucketName, PurgeObject: &thePurge})
			if errors.Is(err2, admin.ErrNoSuchBucket) {
				logger.Infof("bucket info %q does not exist", p.bucketName)
			} else {
				return errors.Wrapf(err, "failed to delete bucket %q (could not get bucket info)", bucketName)
			}
		} else {
			return errors.Wrapf(err, "failed to delete bucket %q", bucketName)
		}
	}
	if len(p.cephUserName) > 0 {
		err := p.adminOpsClient.RemoveUser(p.clusterInfo.Context, admin.User{ID: p.cephUserName})
		if err != nil {
			if errors.Is(err, admin.ErrNoSuchUser) {
				logger.Warningf("user %q does not exist, nothing to delete. %v", p.cephUserName, err)
			}
			logger.Warningf("failed to delete user %q. %v", p.cephUserName, err)
		} else {
			logger.Infof("user %q successfully deleted", p.cephUserName)
		}
	}
	return nil
}
