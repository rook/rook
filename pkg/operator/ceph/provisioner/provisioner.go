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
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	ceph "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	"github.com/rook/rook/pkg/operator/ceph/provisioner/controller"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	attacherImageKey              = "attacherImage"
	storageClassBetaAnnotationKey = "volume.beta.kubernetes.io/storage-class"
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
	pool string

	// Optional: Name of the cluster. Default is `rook`
	clusterNamespace string

	// Optional: File system type used for mounting the image. Default is `ext4`
	fstype string
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

	if err := p.createVolume(imageName, cfg.pool, cfg.clusterNamespace, requestBytes); err != nil {
		return nil, err
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
				v1.ResourceName(v1.ResourceStorage): capacity,
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				FlexVolume: &v1.FlexVolumeSource{
					Driver: flexdriver,
					FSType: cfg.fstype,
					Options: map[string]string{
						flexvolume.StorageClassKey:     storageClass,
						flexvolume.PoolKey:             cfg.pool,
						flexvolume.ImageKey:            imageName,
						flexvolume.ClusterNamespaceKey: cfg.clusterNamespace,
					},
				},
			},
		},
	}
	logger.Infof("successfully created Rook Block volume %+v", pv.Spec.PersistentVolumeSource.FlexVolume)
	return pv, nil
}

// createVolume creates a rook block volume.
func (p *RookVolumeProvisioner) createVolume(image, pool string, clusterNamespace string, size int64) error {
	if image == "" || pool == "" || clusterNamespace == "" || size == 0 {
		return fmt.Errorf("image missing required fields (image=%s, pool=%s, clusterNamespace=%s, size=%d)", image, pool, clusterNamespace, size)
	}

	createdImage, err := ceph.CreateImage(p.context, clusterNamespace, image, pool, uint64(size))
	if err != nil {
		return fmt.Errorf("Failed to create rook block image %s/%s: %v", pool, image, err)
	}
	logger.Infof("Rook block image created: %s", createdImage.Name)

	return nil
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
	clusterns := volume.Spec.PersistentVolumeSource.FlexVolume.Options[flexvolume.ClusterNamespaceKey]
	pool := volume.Spec.PersistentVolumeSource.FlexVolume.Options[flexvolume.PoolKey]
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
			cfg.pool = v
		case "clusternamespace":
			cfg.clusterNamespace = v
		case "clustername":
			cfg.clusterNamespace = v
		case "fstype":
			cfg.fstype = v
		default:
			return nil, fmt.Errorf("invalid option %q for volume plugin %s", k, "rookVolumeProvisioner")
		}
	}

	if len(cfg.pool) == 0 {
		return nil, fmt.Errorf("StorageClass for provisioner %s must contain 'pool' parameter", "rookVolumeProvisioner")
	}

	if len(cfg.clusterNamespace) == 0 {
		cfg.clusterNamespace = cluster.DefaultClusterName
	}

	return &cfg, nil
}
