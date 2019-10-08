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
	"fmt"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	ceph "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/controller"
)

const (
	attacherImageKey              = "attacherImage"
	storageClassBetaAnnotationKey = "volume.beta.kubernetes.io/storage-class"
	sizeMB                        = 1048576 // 1 MB
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-provisioner")

// RookVolumeProvisioner is used to provision Rook volumes on Kubernetes
type RookVolumeProvisioner struct {
	context *clusterd.Context

	// The flex driver vendor dir to use
	flexDriverVendor string
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

// Provision creates a storage asset and returns a PV object representing it.
func (p *RookVolumeProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {

	var err error
	if options.PVC.Spec.Selector != nil {
		return nil, errors.New("claim Selector is not supported")
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

	// the size of the PV needs to be at least as large as the size in the PVC
	// or binding won't be successful. createVolume uses the requestBytes
	// parameter as a target, and guarantees that the size created as at least
	// that large. the adjusted value is placed in blockImage.Size and it is
	// suitable to be converted into Mi.
	//
	// note that the rounding error that can occur if the original non-adjusted
	// request is used in the original formulation here:
	//
	//    s := fmt.Sprintf("%dMi", blockImage.Size/sizeMB)
	//    Size = 500M = 500,000,000 bytes
	//    500M / 2**20 = 476
	//    476Mi = 476 * 2**20 = 499122176 < 500M
	//
	s := fmt.Sprintf("%dMi", blockImage.Size/sizeMB)
	quantity, err := resource.ParseQuantity(s)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot parse %q", s)
	}

	driverName, err := flexvolume.RookDriverName(p.context)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get driver name")
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
		return nil, errors.Errorf("image missing required fields (image=%s, pool=%s, clusterNamespace=%s, size=%d)", image, pool, clusterNamespace, size)
	}

	createdImage, err := ceph.CreateImage(p.context, clusterNamespace, image, pool, dataPool, uint64(size))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create rook block image %s/%s", pool, image)
	}
	logger.Infof("Rook block image created: %s, size = %d", createdImage.Name, createdImage.Size)

	return createdImage, nil
}

// Delete removes the storage asset that was created by Provision represented
// by the given PV.
func (p *RookVolumeProvisioner) Delete(volume *v1.PersistentVolume) error {
	logger.Infof("Deleting volume %s", volume.Name)
	if volume.Spec.PersistentVolumeSource.FlexVolume == nil {
		return errors.Errorf("Failed to delete rook block image %s: %s", volume.Name, "PersistentVolume is not a FlexVolume")
	}
	if volume.Spec.PersistentVolumeSource.FlexVolume.Options == nil {
		return errors.Errorf("Failed to delete rook block image %s: %s", volume.Name, "PersistentVolume has no image defined for the FlexVolume")
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
		return errors.Errorf("failed to delete rook block image %s/%s: no clusterNamespace or (deprecated) clusterName option given", pool, volume.Name)
	}
	err := ceph.DeleteImage(p.context, clusterns, name, pool)
	if err != nil {
		return errors.Wrapf(err, "failed to delete rook block image %s/%s", pool, volume.Name)
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

	return "", errors.Errorf("failed to get storageclass from PVC %s/%s", options.PVC.Namespace, options.PVC.Name)
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
			return nil, errors.Errorf("invalid option %q for volume plugin %s", k, "rookVolumeProvisioner")
		}
	}

	if len(cfg.blockPool) == 0 {
		return nil, errors.Errorf("StorageClass for provisioner %s must contain 'blockPool' parameter", "rookVolumeProvisioner")
	}

	if len(cfg.clusterNamespace) == 0 {
		cfg.clusterNamespace = cluster.DefaultClusterName
	}

	return &cfg, nil
}
