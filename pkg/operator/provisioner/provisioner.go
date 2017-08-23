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
	"github.com/kubernetes-incubator/external-storage/lib/controller"
	ceph "github.com/rook/rook/pkg/ceph/client"
	cephmon "github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/mon"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
)

const (
	imageNameMaxLen = 100 // image name should be under 100 chars to support kernels older than 4.7
	imageNamePrefix = "k8s-dynamic"
	rbdIDPrefix     = "rbd_id."
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-provisioner")

// RookVolumeProvisioner is used to provision Rook volumes on Kubernetes
type RookVolumeProvisioner struct {
	context *clusterd.Context

	// Configuration of rook volume provisioner
	provConfig provisionerConfig

	Namespace           string
}

type provisionerConfig struct {
	// Required: The pool name to provision volumes from.
	pool string

	// Optional: Namespace of the cluster. Default is `rook`
	clusterNamespace string

	// Optional: Name of the cluster. Default is `rook`
	clusterName string

	// Optional: File system type used for mounting the image. Default is `ext4`
	fstype string
}

// New creates RookVolumeProvisioner
func New(context *clusterd.Context, namespace string) controller.Provisioner {
	return &RookVolumeProvisioner{
		context: context,
		Namespace: namespace,
	}
}

// Provision creates a storage asset and returns a PV object representing it.
func (p *RookVolumeProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {

	var err error
	if options.PVC.Spec.Selector != nil {
		return nil, fmt.Errorf("claim Selector is not supported")
	}

	logger.Infof("VolumeOptions %v", options)

	cfg, err := parseClassParameters(options.Parameters)
	if err != nil {
		return nil, err
	}
	p.provConfig = *cfg

	logger.Infof("creating volume with configuration %+v", p.provConfig)

	capacity := options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)]
	requestBytes := capacity.Value()

	imageName := createImageName(options.PVName)

	err = p.createVolume(imageName, p.provConfig.pool, requestBytes)
	if err != nil {
		return nil, err
	}

	monitors, err := p.getMonitorEndpoints()
	if err != nil {
		return nil, err
	}

	radosUser := fmt.Sprintf("%s-rook-user", p.provConfig.clusterName)
	secretRef := new(v1.LocalObjectReference)
	secretRef.Name = fmt.Sprintf("%s-rook-user", p.provConfig.clusterName)

	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: options.PVName,
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: options.PersistentVolumeReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): capacity,
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				RBD: &v1.RBDVolumeSource{
					RBDImage:     imageName,
					RBDPool:      p.provConfig.pool,
					CephMonitors: monitors,
					RadosUser:    radosUser,
					SecretRef:    secretRef,
					FSType:       p.provConfig.fstype,
					ReadOnly:     false,
				},
			},
		},
	}
	logger.Infof("successfully created Rook Block volume %+v", pv.Spec.PersistentVolumeSource.RBD)
	return pv, nil
}

// createVolume creates a rook block volume.
func (p *RookVolumeProvisioner) createVolume(image, pool string, size int64) error {
	if image == "" || pool == "" || size == 0 {
		return fmt.Errorf("image missing required fields (image=%s, pool=%s, size=%d)", image, pool, size)
	}

	createdImage, err := ceph.CreateImage(p.context, p.provConfig.clusterName, image, pool, uint64(size))
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

	name := volume.Spec.PersistentVolumeSource.RBD.RBDImage
	err := ceph.DeleteImage(p.context, p.provConfig.clusterName, name, p.provConfig.pool)
	if err != nil {
		return fmt.Errorf("Failed to delete rook block image %s/%s: %v", p.provConfig.pool, volume.Name, err)
	}
	logger.Infof("succeeded deleting volume %+v", volume)
	return nil
}

func parseClassParameters(params map[string]string) (*provisionerConfig, error) {
	var cfg provisionerConfig

	// Namespace and cluster name have the same default name
	defaultCluster := k8sutil.Namespace

	for k, v := range params {
		switch strings.ToLower(k) {
		case "pool":
			cfg.pool = v
		case "clusternamespace":
			cfg.clusterNamespace = v
		case "clustername":
			cfg.clusterName = v
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
		cfg.clusterNamespace = defaultCluster
	}

	if len(cfg.clusterName) == 0 {
		cfg.clusterName = defaultCluster
	}

	return &cfg, nil
}

func (p *RookVolumeProvisioner) getMonitorEndpoints() ([]string, error) {
	cm, err := p.context.Clientset.CoreV1().ConfigMaps(p.Namespace).Get(mon.EndpointConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get mon endpoints. %+v", err)
	}

	// Parse the monitor List
	info, ok := cm.Data[mon.EndpointDataKey]
	if !ok {
		return nil, fmt.Errorf("failed to find mon endpoints in config map: %+v", cm.Data)
	}

	mons := cephmon.ParseMonEndpoints(info)
	var endpoints []string
	for _, mon := range mons {
		endpoints = append(endpoints, mon.Endpoint)
	}
	return endpoints, nil
}

func createImageName(pvName string) string {
	// generate a UUID for our image name
	u := string(uuid.NewUUID())

	// image name should be under 100 chars to support kernels older than 4.7
	// when the RBD kernel module converts the image name to an OID, it will use "rbd_id.<imageName>",
	// so we have to leave room for that "rbd_id." prefix too.
	pvNameMaxLen := imageNameMaxLen - len(rbdIDPrefix) - len(imageNamePrefix) - len(u) - 2 // 2 hyphens
	if len(pvName) > pvNameMaxLen {
		// the PV name is too long, truncate it before including it in the final image name
		pvName = pvName[:pvNameMaxLen]
	}

	return fmt.Sprintf("%s-%s-%s", imageNamePrefix, pvName, u)
}
