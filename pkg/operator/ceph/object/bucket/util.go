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

	"github.com/coreos/pkg/capnslog"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/kube-object-storage/lib-bucket-provisioner/pkg/provisioner"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephObject "github.com/rook/rook/pkg/operator/ceph/object"
	storagev1 "k8s.io/api/storage/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-bucket-prov")

const (
	CephUser             = "cephUser"
	ObjectStoreName      = "objectStoreName"
	ObjectStoreNamespace = "objectStoreNamespace"
	objectStoreEndpoint  = "endpoint"
)

func NewBucketController(cfg *rest.Config, p *Provisioner, data map[string]string) (*provisioner.Provisioner, error) {
	const allNamespaces = ""
	provName := cephObject.GetObjectBucketProvisioner(data, p.clusterInfo.Namespace)

	logger.Infof("ceph bucket provisioner launched watching for provisioner %q", provName)
	return provisioner.NewProvisioner(cfg, provName, p, allNamespaces)
}

func getObjectStoreName(sc *storagev1.StorageClass) string {
	return sc.Parameters[ObjectStoreName]
}

func getObjectStoreEndpoint(sc *storagev1.StorageClass) string {
	return sc.Parameters[objectStoreEndpoint]
}

func getBucketName(ob *bktv1alpha1.ObjectBucket) string {
	return ob.Spec.Endpoint.BucketName
}

func isStaticBucket(sc *storagev1.StorageClass) (string, bool) {
	const key = "bucketName"
	val, ok := sc.Parameters[key]
	return val, ok
}

func getCephUser(ob *bktv1alpha1.ObjectBucket) string {
	return ob.Spec.AdditionalState[CephUser]
}

func (p *Provisioner) getObjectStore() (*cephv1.CephObjectStore, error) {
	ctx := p.clusterInfo.Context
	// Verify the object store API object actually exists
	store, err := p.context.RookClientset.CephV1().CephObjectStores(p.clusterInfo.Namespace).Get(ctx, p.objectStoreName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil, errors.Wrap(err, "cephObjectStore not found")
		}
		return nil, errors.Wrapf(err, "failed to get ceph object store %q", p.objectStoreName)
	}
	return store, err
}

func MaxObjectQuota(AdditionalConfig map[string]string) string {
	return AdditionalConfig["maxObjects"]
}

func MaxSizeQuota(AdditionalConfig map[string]string) string {
	return AdditionalConfig["maxSize"]
}

func GetObjectStoreNameFromBucket(ob *bktv1alpha1.ObjectBucket) (types.NamespacedName, error) {
	// Rook v1.11 OBCs have additional state labels that tell the object store namespace and name.
	// This is critical for CephObjectStores in external mode that connect to RGW endpoints directly
	// which don't have a deterministic domain structure.
	nsName, err := getNSNameFromAdditionalState(ob.Spec.AdditionalState)
	if err == nil {
		return nsName, nil
	}

	// TODO: remove after Rook v1.12
	// Older OBCs don't have the additional state labels, but they will always be configured to use
	// the legacy CephObjectStore Service which has a deterministic domain structure.
	logger.Debugf("falling back to legacy method for determining OBC \"%s/%s\"'s CephObjectStore from endpoint %q",
		ob.Namespace, ob.Name, ob.Spec.Connection.Endpoint.BucketHost)
	nsName, err = cephObject.ParseDomainName(ob.Spec.Connection.Endpoint.BucketHost)
	if err != nil {
		return types.NamespacedName{}, errors.Wrapf(err, "malformed BucketHost %q", ob.Spec.Endpoint.BucketHost)
	}

	return nsName, nil
}

func getNSNameFromAdditionalState(state map[string]string) (types.NamespacedName, error) {
	name, ok := state[ObjectStoreName]
	if !ok {
		return types.NamespacedName{}, fmt.Errorf("failed to get %q from OB additional state: %v", ObjectStoreName, state)
	}
	namespace, ok := state[ObjectStoreNamespace]
	if !ok {
		return types.NamespacedName{}, fmt.Errorf("failed to get %q from OB additional state: %v", ObjectStoreNamespace, state)
	}
	return types.NamespacedName{Name: name, Namespace: namespace}, nil
}
