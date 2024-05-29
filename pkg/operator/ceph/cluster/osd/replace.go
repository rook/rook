/*
Copyright 2023 The Rook Authors. All rights reserved.

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

// Package osd for the Ceph OSDs.
package osd

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	OSDReplaceConfigName       = "osd-replace-config"
	OSDReplaceConfigKey        = "config"
	OSDStoreUpdateConfirmation = "yes-really-update-store"
)

// OSDReplaceInfo represents an OSD that needs to replaced
type OSDReplaceInfo struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
	Node string `json:"node"`
}

type OSDReplaceInfoList []OSDReplaceInfo

func (o *OSDReplaceInfoList) getOSDIds() []int {
	osdIDs := []int{}
	for _, osd := range *o {
		osdIDs = append(osdIDs, osd.ID)
	}
	return osdIDs
}

func (o *OSDReplaceInfo) saveAsConfig(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo) error {
	configStr, err := o.string()
	if err != nil {
		return errors.Wrapf(err, "failed to convert osd replace config to string")
	}

	newConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OSDReplaceConfigName,
			Namespace: clusterInfo.Namespace,
		},
		Data: map[string]string{
			OSDReplaceConfigKey: configStr,
		},
	}

	err = clusterInfo.OwnerInfo.SetControllerReference(newConfigMap)
	if err != nil {
		return errors.Wrapf(err, "failed to set owner reference on %q configMap", newConfigMap.Name)
	}

	_, err = k8sutil.CreateOrUpdateConfigMap(clusterInfo.Context, context.Clientset, newConfigMap)
	if err != nil {
		return errors.Wrapf(err, "failed to create or update %q configMap", newConfigMap.Name)
	}

	return nil
}

func (o *OSDReplaceInfo) string() (string, error) {
	configInBytes, err := json.Marshal(o)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal osd replace config")
	}

	return string(configInBytes), nil
}

func (c *Cluster) replaceOSDForNewStore() (OSDReplaceInfoList, error) {
	if c.spec.Storage.Store.UpdateStore != OSDStoreUpdateConfirmation {
		logger.Info("no replacement of osds is requested")
		return nil, nil
	}

	osdsToBeReplaced, err := c.getOSDWithNonMatchingStore()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get information about the OSDs where backing store does not match the spec in namespace %q", c.clusterInfo.Namespace)
	}

	if len(osdsToBeReplaced) == 0 {
		logger.Debug("all OSDs are using the desired backing store. No replacement is required.")
		return osdsToBeReplaced, nil
	}

	// replace an OSD only if Pgs are healthy
	pgHealthMsg, pgClean, err := cephclient.IsClusterClean(c.context, c.clusterInfo, c.spec.DisruptionManagement.PGHealthyRegex)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check if the pgs are clean before replacing OSDs")
	}

	if !pgClean {
		logger.Warningf("skipping OSD replacement because pgs are not healthy. PG status: %q", pgHealthMsg)
		return osdsToBeReplaced, nil
	}

	// Check for an existing OSDs in the configmap
	osdToBeReplaced, err := GetOSDReplaceConfigMap(c.context, c.clusterInfo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get any existing OSD in replace configmap")
	}
	if osdToBeReplaced != nil {
		c.replaceOSD = osdToBeReplaced
	} else {
		c.replaceOSD = &osdsToBeReplaced[0]
	}

	logger.Infof("replacing OSD.%d to the new backing store %q", c.replaceOSD.ID, c.spec.Storage.Store.Type)
	err = c.deleteOSDDeployment(c.replaceOSD.ID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to delete OSD deployment that needs migration to new backend store in namespace %q", c.clusterInfo.Namespace)
	}

	return osdsToBeReplaced, nil
}

// getOSDWithNonMatchingStore returns OSDs with osd-store label different from expected store in cephCluster spec
func (c *Cluster) getOSDWithNonMatchingStore() (OSDReplaceInfoList, error) {
	osdReplaceList := []OSDReplaceInfo{}
	// get existing OSD deployments
	osdDeployments, err := c.getOSDDeployments()
	if err != nil {
		return osdReplaceList, errors.Wrapf(err, "failed to get existing OSD deployments in namespace %q", c.clusterInfo.Namespace)
	}
	for i := range osdDeployments.Items {
		if osdStore, ok := osdDeployments.Items[i].Labels[osdStore]; ok {
			if osdStore != string(c.spec.Storage.Store.Type) {
				osdInfo, err := c.getOSDInfo(&osdDeployments.Items[i])
				if err != nil {
					return nil, errors.Wrapf(err, "failed to details about the OSD %q", osdDeployments.Items[i].Name)
				}
				var path string
				if osdInfo.PVCName != "" {
					path = osdInfo.PVCName
				} else {
					path = osdInfo.BlockPath
				}
				osdReplaceList = append(osdReplaceList, OSDReplaceInfo{ID: osdInfo.ID, Path: path, Node: osdInfo.NodeName})
			}
		}
	}

	return osdReplaceList, nil
}

// GetOSDReplaceConfigMap returns the OSD replace config map
func GetOSDReplaceConfigMap(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo) (*OSDReplaceInfo, error) {
	cm, err := context.Clientset.CoreV1().ConfigMaps(clusterInfo.Namespace).Get(clusterInfo.Context, OSDReplaceConfigName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return nil, nil
		}
	}

	configStr, ok := cm.Data[OSDReplaceConfigKey]
	if !ok || configStr == "" {
		logger.Debugf("empty config map %q", OSDReplaceConfigName)
		return nil, nil
	}

	config := &OSDReplaceInfo{}
	err = json.Unmarshal([]byte(configStr), config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to JSON unmarshal osd replace status from the (%q)", configStr)
	}

	return config, nil
}
