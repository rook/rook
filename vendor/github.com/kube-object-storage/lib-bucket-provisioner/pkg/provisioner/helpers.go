/*
Copyright 2019 Red Hat Inc.

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

package provisioner

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/kube-object-storage/lib-bucket-provisioner/pkg/apis/objectbucket.io/v1alpha1"
	"github.com/kube-object-storage/lib-bucket-provisioner/pkg/client/clientset/versioned"
)

func makeObjectReference(claim *v1alpha1.ObjectBucketClaim) *corev1.ObjectReference {

	return &corev1.ObjectReference{
		APIVersion: v1alpha1.SchemeGroupVersion.String(),
		Kind:       v1alpha1.ObjectBucketClaimGVK().Kind,
		Name:       claim.Name,
		Namespace:  claim.Namespace,
		UID:        claim.UID,
	}
}

func makeOwnerReference(claim *v1alpha1.ObjectBucketClaim) metav1.OwnerReference {

	blockOwnerDeletion := true
	isController := true

	return metav1.OwnerReference{
		APIVersion:         v1alpha1.SchemeGroupVersion.String(),
		Kind:               v1alpha1.ObjectBucketClaimGVK().Kind,
		Name:               claim.Name,
		UID:                claim.UID,
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}

// return true if owner refs report being owned by the obc, false otherwise
func objectIsOwnedByClaim(obc *v1alpha1.ObjectBucketClaim, ownerRefs []metav1.OwnerReference) bool {
	for _, ref := range ownerRefs {
		// UID can change if k8s rebuilds its object tree
		// Kind and Name should be enough info to determine ownership conclusively
		if ref.Kind == v1alpha1.ObjectBucketClaimGVK().Kind && ref.Name == obc.Name {
			return true
		}
	}
	return false
}

func bucketIsOwnedByClaim(obc *v1alpha1.ObjectBucketClaim, ob *v1alpha1.ObjectBucket) bool {
	ref := ob.Spec.ClaimRef
	emptyRef := corev1.ObjectReference{}
	if ref == nil || *ref == emptyRef {
		return false
	}
	// UID can change if k8s rebuilds its object tree
	// Kind, Namespace, and Name should be enough info to determine ownership conclusively
	if ref.Kind == v1alpha1.ObjectBucketClaimGVK().Kind && ref.Namespace == obc.Namespace && ref.Name == obc.Name {
		return true
	}
	return false
}

func shouldProvision(obc *v1alpha1.ObjectBucketClaim) bool {
	logD.Info("checking OBC for OB name, this indicates provisioning is complete", obc.Name)
	if obc.Spec.ObjectBucketName != "" && obc.Status.Phase == v1alpha1.ObjectBucketClaimStatusPhaseBound {
		log.Info("provisioning already completed", "ObjectBucket", obc.Spec.ObjectBucketName)
		return false
	}
	return true
}

func claimRefForKey(key string, c versioned.Interface) (*corev1.ObjectReference, error) {
	claim, err := claimForKey(key, c)
	if err != nil {
		return nil, err
	}
	return makeObjectReference(claim), nil
}

func claimForKey(key string, c versioned.Interface) (obc *v1alpha1.ObjectBucketClaim, err error) {
	logD.Info("getting claim for key")

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, err
	}
	return c.ObjectbucketV1alpha1().ObjectBucketClaims(ns).Get(context.TODO(), name, metav1.GetOptions{})
}

// Return true if this storage class is for a new bucket vs an existing bucket.
func isNewBucketByStorageClass(sc *storagev1.StorageClass) bool {
	return len(sc.Parameters[v1alpha1.StorageClassBucket]) == 0
}

// Return true if this OB is for a new bucket vs an existing bucket.
func isNewBucketByObjectBucket(c kubernetes.Interface, ob *v1alpha1.ObjectBucket) bool {
	// temp: get bucket name from OB's storage class
	class, err := storageClassForObjectBucket(ob, c)
	if err != nil || class == nil {
		log.Error(err, "unable to get StorageClass of ObjectBucket")
		return false
	}
	return len(class.Parameters[v1alpha1.StorageClassBucket]) == 0
}

func composeConfigMapName(obc *v1alpha1.ObjectBucketClaim) string {
	return obc.Name
}

func composeSecretName(obc *v1alpha1.ObjectBucketClaim) string {
	return obc.Name
}

func configMapForClaimKey(key string, c kubernetes.Interface) (*corev1.ConfigMap, error) {
	logD.Info("getting configMap for key", "key", key)
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, err
	}
	cm, err := c.CoreV1().ConfigMaps(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return cm, nil
}

func secretForClaimKey(key string, c kubernetes.Interface) (sec *corev1.Secret, err error) {
	logD.Info("getting secret for key", "key", key)
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil, err
	}
	sec, err = c.CoreV1().Secrets(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return
}

func setObjectBucketName(ob *v1alpha1.ObjectBucket, key string) {
	obName, err := objectBucketNameFromClaimKey(key)
	if err != nil {
		return
	}
	ob.Name = obName
}

func objectBucketNameFromClaimKey(key string) (string, error) {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(objectBucketNameFormat, ns, name), nil
}

func composeBucketName(obc *v1alpha1.ObjectBucketClaim) (string, error) {
	if obc.Spec.BucketName == "" && obc.Spec.GenerateBucketName == "" {
		return "", fmt.Errorf("expected either bucketName or generateBucketName defined")
	}
	bucketName := obc.Spec.BucketName
	if bucketName == "" {
		bucketName = generateBucketName(obc.Spec.GenerateBucketName)
	}
	return bucketName, nil
}

const (
	maxNameLen     = 63
	uuidSuffixLen  = 36
	maxBaseNameLen = maxNameLen - uuidSuffixLen
)

func generateBucketName(prefix string) string {
	if len(prefix) >= maxBaseNameLen {
		prefix = prefix[:maxBaseNameLen-1]
	}
	return fmt.Sprintf("%s-%s", prefix, uuid.New())
}

func storageClassForClaim(c kubernetes.Interface, obc *v1alpha1.ObjectBucketClaim) (*storagev1.StorageClass, error) {
	if obc == nil {
		return nil, fmt.Errorf("got nil ObjectBucketClaim pointer")
	}
	if obc.Spec.StorageClassName == "" {
		return nil, fmt.Errorf("no StorageClass defined for ObjectBucketClaim \"%s/%s\"", obc.Namespace, obc.Name)
	}
	logD.Info("getting ObjectBucketClaim's StorageClass")
	class, err := c.StorageV1().StorageClasses().Get(context.TODO(), obc.Spec.StorageClassName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting StorageClass %q: %v", obc.Spec.StorageClassName, err)
	}
	log.Info("got StorageClass", "name", class.Name)
	return class, nil
}

func storageClassForObjectBucket(ob *v1alpha1.ObjectBucket, c kubernetes.Interface) (*storagev1.StorageClass, error) {
	if ob == nil {
		return nil, fmt.Errorf("got nil ObjectBucket pointer")
	}
	if ob.Spec.StorageClassName == "" {
		return nil, fmt.Errorf("no StorageClass defined for ObjectBucket %q", ob.Name)
	}
	logD.Info("getting ObjectBucket's storage class", "name", ob.Spec.StorageClassName)
	class, err := c.StorageV1().StorageClasses().Get(context.TODO(), ob.Spec.StorageClassName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting StorageClass %q: %v", ob.Spec.StorageClassName, err)
	}
	log.Info("got StorageClass", "name")

	return class, nil
}

func addLabels(obj metav1.Object, newLabels map[string]string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	for k, v1 := range newLabels {
		if v2, ok := labels[k]; ok && v1 != v2 {
			log.Info("key already exists in labels", k, v2, "overwritten by", v1)
		}
		labels[k] = v1
	}
	obj.SetLabels(labels)
}

func addFinalizers(obj metav1.Object, newFilalizers []string) {
	finalizers := obj.GetFinalizers()
	finalizerMap := make(map[string]struct{})
	for _, f := range finalizers {
		finalizerMap[f] = struct{}{}
	}

	for _, f := range newFilalizers {
		if _, ok := finalizerMap[f]; !ok {
			finalizers = append(finalizers, f)
		}
	}
	obj.SetFinalizers(finalizers)
}

func removeFinalizer(obj metav1.Object) {
	finalizers := obj.GetFinalizers()
	for i, f := range finalizers {
		if f == finalizer {
			obj.SetFinalizers(append(finalizers[:i], finalizers[i+1:]...))
			break
		}
	}
}

// replace illegal label value characters with "-".
// Note: the only substitution is replacing "/" with "-". This needs improvement.
func labelValue(v string) string {
	if errs := validation.IsValidLabelValue(v); len(errs) == 0 {
		return v
	}
	if len(v) > validation.LabelValueMaxLength {
		v = v[0:validation.LabelValueMaxLength]
	}
	return strings.Replace(v, "/", "-", -1)
}
