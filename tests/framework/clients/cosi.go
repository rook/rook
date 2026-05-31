package clients

import (
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
)

type COSIOperation struct {
	k8sh      *utils.K8sHelper
	manifests installer.CephManifests
}

func CreateCOSIOperation(k8sh *utils.K8sHelper, manifests installer.CephManifests) *COSIOperation {
	return &COSIOperation{k8sh, manifests}
}

func (c *COSIOperation) CreateCOSI() error {
	return c.k8sh.ResourceOperation("create", c.manifests.GetCOSIDriver())
}

func (c *COSIOperation) DeleteCOSI() error {
	return c.k8sh.ResourceOperation("delete", c.manifests.GetCOSIDriver())
}

func (c *COSIOperation) CreateBucketClass(name, objectStoreUserSecretName, deletionPolicy string) error {
	return c.k8sh.ResourceOperation("create", c.manifests.GetBucketClass(name, objectStoreUserSecretName, deletionPolicy))
}

func (c *COSIOperation) DeleteBucketClass(name, objectStoreUserSecretName, deletionPolicy string) error {
	return c.k8sh.ResourceOperation("delete", c.manifests.GetBucketClass(name, objectStoreUserSecretName, deletionPolicy))
}

func (c *COSIOperation) CreateBucketClaim(name, bucketClassName string) error {
	return c.k8sh.ResourceOperation("create", c.manifests.GetBucketClaim(name, bucketClassName))
}

func (c *COSIOperation) DeleteBucketClaim(name, bucketClassName string) error {
	return c.k8sh.ResourceOperation("delete", c.manifests.GetBucketClaim(name, bucketClassName))
}
