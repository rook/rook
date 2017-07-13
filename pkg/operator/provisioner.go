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
package operator

import (
	"fmt"
	"strings"

	"github.com/kubernetes-incubator/external-storage/lib/controller"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/operator/k8sutil"
	rookclient "github.com/rook/rook/pkg/rook/client"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
)

const (
	imageNameMaxLen = 100 // image name should be under 100 chars to support kernels older than 4.7
	imageNamePrefix = "k8s-dynamic"
	rbdIDPrefix     = "rbd_id."
)

type rookVolumeProvisioner struct {
	clusterManager *clusterManager

	// Configuration of rook volume provisioner
	provConfig provisionerConfig
}

type provisionerConfig struct {
	// Required:  the pool name to provision volumes from.
	pool string

	// Optional: Namespace of the cluster. Default is `rook`
	clusterNamespace string

	// Optional: Name of the cluster. Default is `rook`
	clusterName string
}

func newRookVolumeProvisioner(clusterManager *clusterManager) controller.Provisioner {
	return &rookVolumeProvisioner{
		clusterManager: clusterManager,
	}
}

// Provision creates a storage asset and returns a PV object representing it.
func (p *rookVolumeProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {

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

	rookClient, err := p.clusterManager.getRookClient(p.provConfig.clusterNamespace)
	if err != nil {
		return nil, fmt.Errorf("Failed to get rook client: %v", err)
	}

	res, err := createVolume(imageName, p.provConfig.pool, requestBytes, rookClient)
	if err != nil {
		return nil, err
	}
	logger.Infof("Rook block image created: %s", res)

	rookClientInfo, err := rookClient.GetClientAccessInfo()
	if err != nil {
		return nil, fmt.Errorf("Failed to get rook client information: %v", err)
	}
	monitors := processMonAddresses(rookClientInfo.MonAddresses)
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
					FSType:       "ext4",
					ReadOnly:     false,
				},
			},
		},
	}
	logger.Infof("successfully created Rook Block volume %+v", pv.Spec.PersistentVolumeSource.RBD)
	return pv, nil
}

// createVolume creates a rook block volume.
func createVolume(image, pool string, size int64, client rookclient.RookRestClient) (string, error) {
	newImage := model.BlockImage{
		Name:     image,
		PoolName: pool,
		Size:     uint64(size),
	}

	res, err := client.CreateBlockImage(newImage)
	if err != nil {
		return "", fmt.Errorf("Failed to create rook block image %s/%s: %v", pool, image, err)
	}

	return res, nil
}

// Delete removes the storage asset that was created by Provision represented
// by the given PV.
func (p *rookVolumeProvisioner) Delete(volume *v1.PersistentVolume) error {
	rookClient, err := p.clusterManager.getRookClient(p.provConfig.clusterNamespace)
	if err != nil {
		return fmt.Errorf("Failed to get rook client: %v", err)
	}

	image := model.BlockImage{
		Name:     volume.Spec.PersistentVolumeSource.RBD.RBDImage,
		PoolName: p.provConfig.pool,
	}

	_, err = rookClient.DeleteBlockImage(image)
	if err != nil {
		return fmt.Errorf("Failed to delete rook block image %s/%s: %v", p.provConfig.pool, volume.Name, err)
	}
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

func processMonAddresses(monAddresses []string) []string {
	monAddrs := make([]string, len(monAddresses))
	for i, addr := range monAddresses {
		mon := strings.Split(addr, "/")
		monAddrs[i] = mon[0]
	}
	return monAddrs
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
