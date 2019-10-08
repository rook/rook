package bucket

import (
	"fmt"

	"github.com/pkg/errors"
	cephObject "github.com/rook/rook/pkg/operator/ceph/object"
)

// The bucket (and objects) could be deleted via an s3 iterator, but since radosgw
// supports the `buket rm` command, this is the approch we'll use.
func (p *Provisioner) deleteBucket(bktName string) error {

	// delete bucket with purge option to remove all objects
	errCode, err := cephObject.DeleteBucket(p.objectContext, p.bucketName, true)
	if errCode == cephObject.RGWErrorNone {
		logger.Infof("Bucket %s successfully deleted", bktName)
		return nil
	}

	// opinion: "not found" is not an error
	if errCode == cephObject.RGWErrorNotFound {
		logger.Infof("Bucket %s does not exist", bktName)
		return nil
	}

	return errors.Wrapf(err, "failed to delete bucket %q: errCode: %d", bktName, errCode)
}

func (p *Provisioner) bucketExists(name string) (bool, error) {
	_, errCode, err := cephObject.GetBucket(p.objectContext, name)
	if errCode != 0 {
		return false, errors.Wrapf(err, "error getting ceph bucket %q", name)
	}
	return true, nil
}

func (p *Provisioner) bucketIsEmpty(name string) (bool, error) {
	bucketStat, _, err := cephObject.GetBucketStats(p.objectContext, name)
	if err != nil {
		return false, errors.Wrapf(err, "error getting ceph bucket %q", name)
	}
	if bucketStat.NumberOfObjects == 0 {
		return true, nil
	}
	return false, nil
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
			return "", "", errors.Wrapf(err, "no user name provided and unable to generate a unique name")
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

// Delete the Ceph user associated to the passed-in bucket.
func (p *Provisioner) deleteCephUser(username string) error {

	logger.Infof("deleting Ceph user %s for bucket %q", username, p.bucketName)
	_, errCode, err := cephObject.DeleteUser(p.objectContext, username)

	if errCode == cephObject.RGWErrorNone {
		logger.Infof("User %s successfully deleted", username)
		return nil
	}

	// opinion: "not found" is not an error
	if errCode == cephObject.RGWErrorNotFound {
		logger.Infof("User %s does not exist", username)
		return nil
	}

	return errors.Wrapf(err, "failed to delete user %q: errCode: %d", username, errCode)
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
