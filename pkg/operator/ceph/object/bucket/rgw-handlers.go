package bucket

import (
	"fmt"

	"github.com/pkg/errors"
	cephObject "github.com/rook/rook/pkg/operator/ceph/object"
)

func (p *Provisioner) bucketExists(name string) (bool, error) {
	_, errCode, err := cephObject.GetBucket(p.objectContext, name)
	if errCode != 0 {
		return false, errors.Wrapf(err, "error getting ceph bucket %q", name)
	}
	return true, nil
}

func (p *Provisioner) userExists(name string) (bool, error) {
	_, errCode, err := cephObject.GetUser(p.objectContext, name)
	if errCode == cephObject.RGWErrorNotFound {
		return false, nil
	}
	if errCode > 0 {
		return false, errors.Wrapf(err, "error getting ceph user %q", name)
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
	userConfig := cephObject.ObjectUser{
		UserID:      username,
		DisplayName: &p.cephUserName,
	}

	u, errCode, err := cephObject.CreateUser(p.objectContext, userConfig)
	if err != nil || errCode != 0 {
		return "", "", errors.Wrapf(err, "error creating ceph user %q: %v", username, err)
	}

	logger.Infof("successfully created Ceph user %q with access keys", username)
	return *u.AccessKey, *u.SecretKey, nil
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
		errCode, err := cephObject.DeleteObjectBucket(p.objectContext, bucketName, true)

		if errCode == cephObject.RGWErrorNone {
			logger.Infof("bucket %q successfully deleted", p.bucketName)
		} else if errCode == cephObject.RGWErrorNotFound {
			// opinion: "not found" is not an error
			logger.Infof("bucket %q does not exist", p.bucketName)
		} else {
			return errors.Wrapf(err, "failed to delete bucket %q: errCode: %d", bucketName, errCode)
		}
	}
	if len(p.cephUserName) > 0 {
		output, err := cephObject.DeleteUser(p.objectContext, p.cephUserName)
		if err != nil {
			logger.Warningf("failed to delete user %q. %s. %v", p.cephUserName, output, err)
		} else {
			logger.Infof("user %q successfully deleted", p.cephUserName)
		}
	}
	return nil
}
