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
	"regexp"
	"strings"

	"github.com/coreos/pkg/capnslog"
	bktv1alpha1 "github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/kube-object-storage/lib-bucket-provisioner/pkg/provisioner"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	cephObject "github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/util/log"
	storagev1 "k8s.io/api/storage/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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

	bucketPlacementKey    = "bucketPlacement"
	bucketStorageClassKey = "bucketStorageClass"

	// standardStorageClass is what RGW records for a bucket created without
	// a storage class, and omits from the bucket's placement_rule
	standardStorageClass = "STANDARD"
)

// Reasons of the Warning Events recorded on an ObjectBucketClaim whose
// bucketPlacement or bucketStorageClass request cannot be honored.
const (
	EventReasonInvalidBucketPlacement  = "InvalidBucketPlacement"
	EventReasonBucketPlacementRejected = "BucketPlacementRejected"
	EventReasonBucketPlacementMismatch = "BucketPlacementMismatch"
)

// placementValuePattern bounds both keys to a bare token: it excludes "/"
// and ":", RGW's separators in the "<placement>/<storage-class>" placement
// rule and the "<zonegroup>:<placement>" location constraint, and the
// whitespace that the storage-class header would be sent without — a value
// created as one string and compared back as another.
var placementValuePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func NewBucketController(cfg *rest.Config, p *Provisioner) (*provisioner.Provisioner, error) {
	const allNamespaces = ""
	provName, err := cephObject.GetObjectBucketProvisioner(p.clusterInfo.Namespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get provisioner name")
	}

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

func additionalConfigSpecFromMap(config map[string]string) (*additionalConfigSpec, error) {
	var err error
	spec := additionalConfigSpec{}

	if _, ok := config["maxObjects"]; ok {
		if !opcontroller.ObcAdditionalConfigKeyIsAllowed("maxObjects") {
			return nil, errors.Errorf("OBC config %q is not allowed", "maxObjects")
		}
		spec.maxObjects, err = quanityToInt64(config["maxObjects"])
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse maxObjects quota")
		}
	}

	if _, ok := config["maxSize"]; ok {
		if !opcontroller.ObcAdditionalConfigKeyIsAllowed("maxSize") {
			return nil, errors.Errorf("OBC config %q is not allowed", "maxSize")
		}
		spec.maxSize, err = quanityToInt64(config["maxSize"])
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse maxSize quota")
		}
	}

	if _, ok := config["bucketMaxObjects"]; ok {
		if !opcontroller.ObcAdditionalConfigKeyIsAllowed("bucketMaxObjects") {
			return nil, errors.Errorf("OBC config %q is not allowed", "bucketMaxObjects")
		}
		spec.bucketMaxObjects, err = quanityToInt64(config["bucketMaxObjects"])
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse bucketMaxObjects quota")
		}
	}

	if _, ok := config["bucketMaxSize"]; ok {
		if !opcontroller.ObcAdditionalConfigKeyIsAllowed("bucketMaxSize") {
			return nil, errors.Errorf("OBC config %q is not allowed", "bucketMaxSize")
		}
		spec.bucketMaxSize, err = quanityToInt64(config["bucketMaxSize"])
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse bucketMaxSize quota")
		}
	}

	if _, ok := config["bucketPolicy"]; ok {
		if !opcontroller.ObcAdditionalConfigKeyIsAllowed("bucketPolicy") {
			return nil, errors.Errorf("OBC config %q is not allowed", "bucketPolicy")
		}
		policy := config["bucketPolicy"]
		spec.bucketPolicy = &policy
	}

	if _, ok := config["bucketLifecycle"]; ok {
		if !opcontroller.ObcAdditionalConfigKeyIsAllowed("bucketLifecycle") {
			return nil, errors.Errorf("OBC config %q is not allowed", "bucketLifecycle")
		}
		lifecycle := config["bucketLifecycle"]
		spec.bucketLifecycle = &lifecycle
	}

	if _, ok := config["bucketOwner"]; ok {
		if !opcontroller.ObcAdditionalConfigKeyIsAllowed("bucketOwner") {
			return nil, errors.Errorf("OBC config %q is not allowed", "bucketOwner")
		}
		bucketOwner := config["bucketOwner"]
		spec.bucketOwner = &bucketOwner
	}

	if _, ok := config[bucketPlacementKey]; ok {
		if !opcontroller.ObcAdditionalConfigKeyIsAllowed(bucketPlacementKey) {
			return nil, errors.Errorf("OBC config %q is not allowed", bucketPlacementKey)
		}
		if err := validatePlacementValue(bucketPlacementKey, config[bucketPlacementKey]); err != nil {
			return nil, err
		}
		spec.bucketPlacement = config[bucketPlacementKey]
	}

	if _, ok := config[bucketStorageClassKey]; ok {
		if !opcontroller.ObcAdditionalConfigKeyIsAllowed(bucketStorageClassKey) {
			return nil, errors.Errorf("OBC config %q is not allowed", bucketStorageClassKey)
		}
		if err := validatePlacementValue(bucketStorageClassKey, config[bucketStorageClassKey]); err != nil {
			return nil, err
		}
		spec.bucketStorageClass = config[bucketStorageClassKey]
	}

	return &spec, nil
}

// errInvalidPlacementValue marks a bucketPlacement or bucketStorageClass
// value the provisioner rejects, as distinct from a key the allowlist
// rejects; only the former is reported on the OBC.
var errInvalidPlacementValue = errors.New("invalid placement value")

// validatePlacementValue rejects a bucketPlacement or bucketStorageClass
// value that is not a bare placement-target or storage-class name; an empty
// value requests nothing and is valid.
func validatePlacementValue(key, value string) error {
	if value == "" || placementValuePattern.MatchString(value) {
		return nil
	}
	return errors.Wrapf(errInvalidPlacementValue, "%s %q is not a valid name: must match %s", key, value, placementValuePattern)
}

// parsePlacementRule splits an RGW bucket placement_rule, reported as
// "<placement>" or "<placement>/<storage-class>", the way RGW's
// rgw_placement_rule::from_str does; an absent class is STANDARD.
func parsePlacementRule(rule string) (placement, storageClass string) {
	placement, storageClass, _ = strings.Cut(rule, "/")
	if storageClass == "" {
		storageClass = standardStorageClass
	}
	return placement, storageClass
}

// checkPlacementRule reports whether an existing bucket's placement_rule
// satisfies the requested placement and storage class. An empty request is
// unmanaged and not compared. RGW cannot change either after creation, so a
// mismatch is a claim the bucket can never satisfy.
func checkPlacementRule(rule, requestedPlacement, requestedStorageClass string) error {
	placement, storageClass := parsePlacementRule(rule)
	var mismatches []string
	if requestedPlacement != "" && requestedPlacement != placement {
		mismatches = append(mismatches, fmt.Sprintf("%s %q was requested but the bucket is on placement %q", bucketPlacementKey, requestedPlacement, placement))
	}
	if requestedStorageClass != "" && requestedStorageClass != storageClass {
		mismatches = append(mismatches, fmt.Sprintf("%s %q was requested but the bucket has storage class %q", bucketStorageClassKey, requestedStorageClass, storageClass))
	}
	if len(mismatches) == 0 {
		return nil
	}
	return errors.New(strings.Join(mismatches, "; "))
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
	log.NamedDebug(nsName, logger, "falling back to legacy method for determining OBC \"%s/%s\"'s CephObjectStore from endpoint %q",
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

func quanityToInt64(qty string) (*int64, error) {
	n, err := resource.ParseQuantity(qty)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %q as a quantity", qty)
	}

	value := n.Value()

	return &value, nil
}
