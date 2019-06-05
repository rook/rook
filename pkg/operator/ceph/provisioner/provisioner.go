/*
Copyright 2017 The Rook Authors. All rights reserved.

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

// Package provisioner to provision Rook volumes on Kubernetes.
package provisioner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	ceph "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/controller"
)

const (
	attacherImageKey              = "attacherImage"
	storageClassBetaAnnotationKey = "volume.beta.kubernetes.io/storage-class"
	sizeMB                        = 1048576 // 1 MB
	provisionCmd                  = "/usr/local/bin/cephfs_provisioner"
	provisionerIDAnn              = "cephFSProvisionerIdentity"
	cephShareAnn                  = "cephShare"
	provisionerNameKey            = "PROVISIONER_NAME"
	secretNamespaceKey            = "PROVISIONER_SECRET_NAMESPACE"
	disableCephNamespaceIsolation = true
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-provisioner")

type provisionOutput struct {
	Path   string `json:"path"`
	User   string `json:"user"`
	Secret string `json:"auth"`
	Mons   string `json:"mons"`
}

// RookVolumeProvisioner is used to provision Rook volumes on Kubernetes
type RookVolumeProvisioner struct {
	context *clusterd.Context

	// The flex driver vendor dir to use
	flexDriverVendor string
}

type cephFSProvisioner struct {
	// Kubernetes Client. Use to retrieve Ceph admin secret
	client   kubernetes.Interface
	identity string
	// Namespace secrets will be created in. If empty, secrets will be created in each PVC's namespace.
	secretNamespace string
	// enable PVC quota
	enableQuota bool
}

type provisionerConfig struct {
	// Required: The pool name to provision volumes from.
	blockPool string

	// Optional: Name of the cluster. Default is `rook`
	clusterNamespace string

	// Optional: File system type used for mounting the image. Default is `ext4`
	fstype string

	// Optional: For erasure coded pools the data pool must be given
	dataBlockPool string
}

// New creates RookVolumeProvisioner
func New(context *clusterd.Context, flexDriverVendor string) controller.Provisioner {
	return &RookVolumeProvisioner{
		context:          context,
		flexDriverVendor: flexDriverVendor,
	}
}

// NewCephFSProvisioner creates cephfs provisioner
func NewCephFSProvisioner(client kubernetes.Interface, id string, secretNamespace string, enableQuota bool) controller.Provisioner {
	return &cephFSProvisioner{
		client:          client,
		identity:        id,
		secretNamespace: secretNamespace,
		enableQuota:     enableQuota,
	}
}

var _ controller.Provisioner = &cephFSProvisioner{}

func generateSecretName(user string) string {
	return "ceph-" + user + "-secret"
}

func getClaimRefNamespace(pv *v1.PersistentVolume) string {
	if pv.Spec.ClaimRef != nil {
		return pv.Spec.ClaimRef.Namespace
	}
	return ""
}

// getSecretFromCephFSPersistentVolume gets secret reference from CephFS PersistentVolume.
// It fallbacks to use ClaimRef.Namespace if SecretRef.Namespace is
// empty. See https://github.com/kubernetes/kubernetes/pull/49502.
func getSecretFromCephFSPersistentVolume(pv *v1.PersistentVolume) (*v1.SecretReference, error) {
	source := &pv.Spec.PersistentVolumeSource
	if source.CephFS == nil {
		return nil, errors.New("pv.Spec.PersistentVolumeSource.CephFS is nil")
	}
	if source.CephFS.SecretRef == nil {
		return nil, errors.New("pv.Spec.PersistentVolumeSource.CephFS.SecretRef is nil")
	}
	if len(source.CephFS.SecretRef.Namespace) > 0 {
		return source.CephFS.SecretRef, nil
	}
	ns := getClaimRefNamespace(pv)
	if len(ns) <= 0 {
		return nil, errors.New("both pv.Spec.SecretRef.Namespace and pv.Spec.ClaimRef.Namespace are empty")
	}
	return &v1.SecretReference{
		Name:      source.CephFS.SecretRef.Name,
		Namespace: ns,
	}, nil
}

// Provision creates a storage asset and returns a PV object representing it.
func (p *cephFSProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {
	if options.PVC.Spec.Selector != nil {
		return nil, fmt.Errorf("claim Selector is not supported")
	}
	//cluster, adminID, adminSecret, pvcRoot, mon, deterministicNames, err := p.parseParameters(options.Parameters)
	//fsName, err := p.parseParameters(options.Parameters)
	//if err != nil {
	//        return nil, err
	//}
	var share, user string
	var (
		err error
		mon []string
	)
	// create share name
	share = fmt.Sprintf("kubernetes-dynamic-pvc-%s", options.PVName)
	// create user id
	user = fmt.Sprintf("kubernetes-dynamic-user-%s", uuid.NewUUID())

	// provision share
	// create cmd
	args := []string{"-n", share, "-u", user}
	if p.enableQuota {
		capacity := options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)]
		requestBytes := strconv.FormatInt(capacity.Value(), 10)
		args = append(args, "-s", requestBytes)
	}
	cmd := exec.Command(provisionCmd, args...)
	//if deterministicNames {
	//	cmd.Env = append(cmd.Env, "CEPH_VOLUME_GROUP="+options.PVC.Namespace)
	//}
	if disableCephNamespaceIsolation {
		cmd.Env = append(cmd.Env, "CEPH_NAMESPACE_ISOLATION_DISABLED=true")
	}
	output, cmdErr := cmd.CombinedOutput()
	if cmdErr != nil {
		return nil, cmdErr
	}
	// validate output
	res := &provisionOutput{}
	json.Unmarshal([]byte(output), &res)
	if res.User == "" || res.Secret == "" || res.Path == "" || res.Mons == "" {
		return nil, fmt.Errorf("invalid provisioner output")
	}
	mon = append(mon, res.Mons)
	nameSpace := p.secretNamespace
	if nameSpace == "" {
		// if empty, create secret in PVC's namespace
		nameSpace = options.PVC.Namespace
	}
	secretName := generateSecretName(user)
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: nameSpace,
			Name:      secretName,
		},
		Data: map[string][]byte{
			"key": []byte(res.Secret),
		},
		Type: "Opaque",
	}

	_, err = p.client.CoreV1().Secrets(nameSpace).Create(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret")
	}
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: options.PVName,
			Annotations: map[string]string{
				provisionerIDAnn: p.identity,
				cephShareAnn:     share,
			},
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: options.PersistentVolumeReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			MountOptions:                  options.MountOptions,
			Capacity: v1.ResourceList{
				// Quotas are supported by the userspace client(ceph-fuse, libcephfs), or kernel client >= 4.17 but only on mimic clusters.
				// In other cases capacity is meaningless here.
				// If quota is enabled, provisioner will set ceph.quota.max_bytes on volume path.
				v1.ResourceName(v1.ResourceStorage): options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)],
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CephFS: &v1.CephFSPersistentVolumeSource{
					Monitors: mon,
					Path:     res.Path[strings.Index(res.Path, "/"):],
					SecretRef: &v1.SecretReference{
						Name:      secretName,
						Namespace: nameSpace,
					},
					User: user,
				},
			},
		},
	}
	return pv, nil
}

func (p *cephFSProvisioner) Delete(volume *v1.PersistentVolume) error {
	ann, ok := volume.Annotations[provisionerIDAnn]
	if !ok {
		return errors.New("identity annotation not found on PV")
	}
	if ann != p.identity {
		return &controller.IgnoredError{Reason: "identity annotation on PV does not match ours"}
	}
	share, ok := volume.Annotations[cephShareAnn]
	if !ok {
		return errors.New("ceph share annotation not found on PV")
	}
	// delete CephFS
	user := volume.Spec.PersistentVolumeSource.CephFS.User
	// create cmd
	cmd := exec.Command(provisionCmd, "-r", "-n", share, "-u", user)
	if disableCephNamespaceIsolation {
		cmd.Env = append(cmd.Env, "CEPH_NAMESPACE_ISOLATION_DISABLED=true")
	}
	output, cmdErr := cmd.CombinedOutput()
	if cmdErr != nil {
		klog.Errorf("failed to delete share %q for %q, err: %v, output: %v", share, user, cmdErr, string(output))
		return cmdErr
	}
	// Remove dynamic user secret
	secretRef, err := getSecretFromCephFSPersistentVolume(volume)
	if err != nil {
		klog.Errorf("failed to get secret references, err: %v", err)
		return err
	}
	err = p.client.CoreV1().Secrets(secretRef.Namespace).Delete(secretRef.Name, &metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Cephfs Provisioner: delete secret failed, err: %v", err)
		return fmt.Errorf("failed to delete secret")
	}
	return nil
}

// Provision creates a storage asset and returns a PV object representing it.
func (p *RookVolumeProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {

	var err error
	if options.PVC.Spec.Selector != nil {
		return nil, fmt.Errorf("claim Selector is not supported")
	}

	cfg, err := parseClassParameters(options.Parameters)
	if err != nil {
		return nil, err
	}

	logger.Infof("creating volume with configuration %+v", *cfg)

	capacity := options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)]
	requestBytes := capacity.Value()

	imageName := options.PVName

	storageClass, err := parseStorageClass(options)
	if err != nil {
		return nil, err
	}

	blockImage, err := p.createVolume(imageName, cfg.blockPool, cfg.dataBlockPool, cfg.clusterNamespace, requestBytes)
	if err != nil {
		return nil, err
	}

	// since we can guarantee the size of the volume image generated have to be in `MB` boundary, so we can
	// convert it to `MB` unit safely here
	s := fmt.Sprintf("%dMi", blockImage.Size/sizeMB)
	quantity, err := resource.ParseQuantity(s)
	if err != nil {
		return nil, fmt.Errorf("cannot parse '%v': %v", s, err)
	}

	driverName, err := flexvolume.RookDriverName(p.context)
	if err != nil {
		return nil, fmt.Errorf("failed to get driver name. %+v", err)
	}

	flexdriver := fmt.Sprintf("%s/%s", p.flexDriverVendor, driverName)
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: imageName,
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: options.PersistentVolumeReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): quantity,
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				FlexVolume: &v1.FlexPersistentVolumeSource{
					Driver: flexdriver,
					FSType: cfg.fstype,
					Options: map[string]string{
						flexvolume.StorageClassKey:     storageClass,
						flexvolume.PoolKey:             cfg.blockPool,
						flexvolume.ImageKey:            imageName,
						flexvolume.ClusterNamespaceKey: cfg.clusterNamespace,
						flexvolume.DataBlockPoolKey:    cfg.dataBlockPool,
					},
				},
			},
		},
	}
	logger.Infof("successfully created Rook Block volume %+v", pv.Spec.PersistentVolumeSource.FlexVolume)
	return pv, nil
}

// createVolume creates a rook block volume.
func (p *RookVolumeProvisioner) createVolume(image, pool, dataPool string, clusterNamespace string, size int64) (*ceph.CephBlockImage, error) {
	if image == "" || pool == "" || clusterNamespace == "" || size == 0 {
		return nil, fmt.Errorf("image missing required fields (image=%s, pool=%s, clusterNamespace=%s, size=%d)", image, pool, clusterNamespace, size)
	}

	createdImage, err := ceph.CreateImage(p.context, clusterNamespace, image, pool, dataPool, uint64(size))
	if err != nil {
		return nil, fmt.Errorf("Failed to create rook block image %s/%s: %v", pool, image, err)
	}
	logger.Infof("Rook block image created: %s, size = %d", createdImage.Name, createdImage.Size)

	return createdImage, nil
}

// Delete removes the storage asset that was created by Provision represented
// by the given PV.
func (p *RookVolumeProvisioner) Delete(volume *v1.PersistentVolume) error {
	logger.Infof("Deleting volume %s", volume.Name)
	if volume.Spec.PersistentVolumeSource.FlexVolume == nil {
		return fmt.Errorf("Failed to delete rook block image %s: %v", volume.Name, "PersistentVolume is not a FlexVolume")
	}
	if volume.Spec.PersistentVolumeSource.FlexVolume.Options == nil {
		return fmt.Errorf("Failed to delete rook block image %s: %v", volume.Name, "PersistentVolume has no image defined for the FlexVolume")
	}
	name := volume.Spec.PersistentVolumeSource.FlexVolume.Options[flexvolume.ImageKey]
	pool := volume.Spec.PersistentVolumeSource.FlexVolume.Options[flexvolume.PoolKey]
	var clusterns string
	if _, ok := volume.Spec.PersistentVolumeSource.FlexVolume.Options[flexvolume.ClusterNamespaceKey]; ok {
		clusterns = volume.Spec.PersistentVolumeSource.FlexVolume.Options[flexvolume.ClusterNamespaceKey]
	} else if _, ok := volume.Spec.PersistentVolumeSource.FlexVolume.Options[flexvolume.ClusterNameKey]; ok {
		// Fallback to `clusterName` as it was used in Rook version earlier v0.8
		clusterns = volume.Spec.PersistentVolumeSource.FlexVolume.Options[flexvolume.ClusterNameKey]
	}
	if clusterns == "" {
		return fmt.Errorf("Failed to delete rook block image %s/%s: no clusterNamespace or (deprecated) clusterName option given", pool, volume.Name)
	}
	err := ceph.DeleteImage(p.context, clusterns, name, pool)
	if err != nil {
		return fmt.Errorf("Failed to delete rook block image %s/%s: %v", pool, volume.Name, err)
	}
	logger.Infof("succeeded deleting volume %+v", volume)
	return nil
}

func parseStorageClass(options controller.VolumeOptions) (string, error) {
	if options.PVC.Spec.StorageClassName != nil {
		return *options.PVC.Spec.StorageClassName, nil
	}

	// PVC manifest is from 1.5. Check annotation.
	if val, ok := options.PVC.Annotations[storageClassBetaAnnotationKey]; ok {
		return val, nil
	}

	return "", fmt.Errorf("failed to get storageclass from PVC %s/%s", options.PVC.Namespace, options.PVC.Name)
}

func parseClassParameters(params map[string]string) (*provisionerConfig, error) {
	var cfg provisionerConfig

	for k, v := range params {
		switch strings.ToLower(k) {
		case "pool":
			cfg.blockPool = v
		case "blockpool":
			cfg.blockPool = v
		case "clusternamespace":
			cfg.clusterNamespace = v
		case "clustername":
			cfg.clusterNamespace = v
		case "fstype":
			cfg.fstype = v
		case "datablockpool":
			cfg.dataBlockPool = v
		default:
			return nil, fmt.Errorf("invalid option %q for volume plugin %s", k, "rookVolumeProvisioner")
		}
	}

	if len(cfg.blockPool) == 0 {
		return nil, fmt.Errorf("StorageClass for provisioner %s must contain 'blockPool' parameter", "rookVolumeProvisioner")
	}

	if len(cfg.clusterNamespace) == 0 {
		cfg.clusterNamespace = cluster.DefaultClusterName
	}

	return &cfg, nil
}
