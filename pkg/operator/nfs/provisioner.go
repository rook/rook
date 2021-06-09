/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package nfs

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/component-helpers/storage/volume"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/v6/controller"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	nfsServerNameSCParam      = "nfsServerName"
	nfsServerNamespaceSCParam = "nfsServerNamespace"
	exportNameSCParam         = "exportName"
	projectBlockAnnotationKey = "nfs.rook.io/project_block"
)

var (
	mountPath = "/"
)

type Provisioner struct {
	client     kubernetes.Interface
	rookClient rookclient.Interface
	quotaer    Quotaer
}

var _ controller.Provisioner = &Provisioner{}

// NewNFSProvisioner returns an instance of nfsProvisioner
func NewNFSProvisioner(clientset kubernetes.Interface, rookClientset rookclient.Interface) (*Provisioner, error) {
	quotaer, err := NewProjectQuota()
	if err != nil {
		return nil, err
	}

	return &Provisioner{
		client:     clientset,
		rookClient: rookClientset,
		quotaer:    quotaer,
	}, nil
}

// Provision(context.Context, ProvisionOptions) (*v1.PersistentVolume, ProvisioningState, error)
func (p *Provisioner) Provision(ctx context.Context, options controller.ProvisionOptions) (*v1.PersistentVolume, controller.ProvisioningState, error) {
	logger.Infof("nfs provisioner: ProvisionOptions %v", options)
	annotations := make(map[string]string)

	if options.PVC.Spec.Selector != nil {
		return nil, controller.ProvisioningFinished, fmt.Errorf("claim Selector is not supported")
	}

	sc, err := p.storageClassForPVC(ctx, options.PVC)
	if err != nil {
		return nil, controller.ProvisioningFinished, err
	}

	serverName, present := sc.Parameters[nfsServerNameSCParam]
	if !present {
		return nil, controller.ProvisioningFinished, errors.Errorf("NFS share Path not found in the storageclass: %v", sc.GetName())
	}

	serverNamespace, present := sc.Parameters[nfsServerNamespaceSCParam]
	if !present {
		return nil, controller.ProvisioningFinished, errors.Errorf("NFS share Path not found in the storageclass: %v", sc.GetName())
	}

	exportName, present := sc.Parameters[exportNameSCParam]
	if !present {
		return nil, controller.ProvisioningFinished, errors.Errorf("NFS share Path not found in the storageclass: %v", sc.GetName())
	}

	nfsserver, err := p.rookClient.NfsV1alpha1().NFSServers(serverNamespace).Get(ctx, serverName, metav1.GetOptions{})
	if err != nil {
		return nil, controller.ProvisioningFinished, err
	}

	nfsserversvc, err := p.client.CoreV1().Services(serverNamespace).Get(ctx, serverName, metav1.GetOptions{})
	if err != nil {
		return nil, controller.ProvisioningFinished, err
	}

	var (
		exportPath string
		found      bool
	)

	for _, export := range nfsserver.Spec.Exports {
		if export.Name == exportName {
			exportPath = path.Join(mountPath, export.PersistentVolumeClaim.ClaimName)
			found = true
		}
	}

	if !found {
		return nil, controller.ProvisioningFinished, fmt.Errorf("No export name from storageclass is match with NFSServer %s in namespace %s", nfsserver.Name, nfsserver.Namespace)
	}

	pvName := strings.Join([]string{options.PVC.Namespace, options.PVC.Name, options.PVName}, "-")
	fullPath := path.Join(exportPath, pvName)
	if err := os.MkdirAll(fullPath, 0700); err != nil {
		return nil, controller.ProvisioningFinished, errors.New("unable to create directory to provision new pv: " + err.Error())
	}

	capacity := options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)]
	block, err := p.createQuota(exportPath, fullPath, strconv.FormatInt(capacity.Value(), 10))
	if err != nil {
		return nil, controller.ProvisioningFinished, err
	}

	annotations[projectBlockAnnotationKey] = block

	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:        options.PVName,
			Annotations: annotations,
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: *options.StorageClass.ReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			MountOptions:                  options.StorageClass.MountOptions,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): capacity,
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				NFS: &v1.NFSVolumeSource{
					Server:   nfsserversvc.Spec.ClusterIP,
					Path:     fullPath,
					ReadOnly: false,
				},
			},
		},
	}

	return pv, controller.ProvisioningFinished, nil
}

func (p *Provisioner) Delete(ctx context.Context, volume *v1.PersistentVolume) error {
	nfsPath := volume.Spec.PersistentVolumeSource.NFS.Path
	pvName := path.Base(nfsPath)

	sc, err := p.storageClassForPV(ctx, volume)
	if err != nil {
		return err
	}

	serverName, present := sc.Parameters[nfsServerNameSCParam]
	if !present {
		return errors.Errorf("NFS share Path not found in the storageclass: %v", sc.GetName())
	}

	serverNamespace, present := sc.Parameters[nfsServerNamespaceSCParam]
	if !present {
		return errors.Errorf("NFS share Path not found in the storageclass: %v", sc.GetName())
	}

	exportName, present := sc.Parameters[exportNameSCParam]
	if !present {
		return errors.Errorf("NFS share Path not found in the storageclass: %v", sc.GetName())
	}

	nfsserver, err := p.rookClient.NfsV1alpha1().NFSServers(serverNamespace).Get(ctx, serverName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	var (
		exportPath string
		found      bool
	)

	for _, export := range nfsserver.Spec.Exports {
		if export.Name == exportName {
			exportPath = path.Join(mountPath, export.PersistentVolumeClaim.ClaimName)
			found = true
		}
	}

	if !found {
		return fmt.Errorf("No export name from storageclass is match with NFSServer %s in namespace %s", nfsserver.Name, nfsserver.Namespace)
	}

	block, ok := volume.Annotations[projectBlockAnnotationKey]
	if !ok {
		return fmt.Errorf("PV doesn't have an annotation with key %s", projectBlockAnnotationKey)
	}

	if err := p.removeQuota(exportPath, block); err != nil {
		return err
	}

	fullPath := path.Join(exportPath, pvName)
	return os.RemoveAll(fullPath)
}

func (p *Provisioner) createQuota(exportPath, directory string, limit string) (string, error) {
	projectsFile := filepath.Join(exportPath, "projects")
	if _, err := os.Stat(projectsFile); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}

		return "", fmt.Errorf("error checking projects file in directory %s: %v", exportPath, err)
	}

	return p.quotaer.CreateProjectQuota(projectsFile, directory, limit)
}

func (p *Provisioner) removeQuota(exportPath, block string) error {
	var projectID uint16
	projectsFile := filepath.Join(exportPath, "projects")
	if _, err := os.Stat(projectsFile); err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("error checking projects file in directory %s: %v", exportPath, err)
	}

	re := regexp.MustCompile("(?m:^([0-9]+):(.+):(.+)$)")
	allMatches := re.FindAllStringSubmatch(block, -1)
	for _, match := range allMatches {
		digits := match[1]
		if id, err := strconv.ParseUint(string(digits), 10, 16); err == nil {
			projectID = uint16(id)
		}
	}

	return p.quotaer.RemoveProjectQuota(projectID, projectsFile, block)
}

func (p *Provisioner) storageClassForPV(ctx context.Context, pv *v1.PersistentVolume) (*storagev1.StorageClass, error) {
	if p.client == nil {
		return nil, fmt.Errorf("Cannot get kube client")
	}
	className := volume.GetPersistentVolumeClass(pv)
	if className == "" {
		return nil, fmt.Errorf("Volume has no storage class")
	}

	return p.client.StorageV1().StorageClasses().Get(ctx, className, metav1.GetOptions{})
}

func (p *Provisioner) storageClassForPVC(ctx context.Context, pvc *v1.PersistentVolumeClaim) (*storagev1.StorageClass, error) {
	if p.client == nil {
		return nil, fmt.Errorf("Cannot get kube client")
	}
	className := volume.GetPersistentVolumeClaimClass(pvc)
	if className == "" {
		return nil, fmt.Errorf("Volume has no storage class")
	}

	return p.client.StorageV1().StorageClasses().Get(ctx, className, metav1.GetOptions{})
}
