/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package installer

import (
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

const (
	OperatorChartName    = "rook-ceph"
	CephClusterChartName = "rook-ceph-cluster"
)

// CreateRookOperatorViaHelm creates rook operator via Helm chart named local/rook present in local repo
func (h *CephInstaller) CreateRookOperatorViaHelm(values map[string]interface{}) error {
	// create the operator namespace before the admission controller is created
	if err := h.k8shelper.CreateNamespace(h.settings.OperatorNamespace); err != nil {
		return errors.Errorf("failed to create namespace %s. %v", h.settings.Namespace, err)
	}
	if err := h.startAdmissionController(); err != nil {
		return errors.Errorf("Failed to start admission controllers: %v", err)
	}
	if err := h.helmHelper.InstallLocalRookHelmChart(h.settings.OperatorNamespace, OperatorChartName, values); err != nil {
		return errors.Errorf("failed to install rook operator via helm, err : %v", err)
	}

	return nil
}

// CreateRookCephClusterViaHelm creates rook cluster via Helm
func (h *CephInstaller) CreateRookCephClusterViaHelm(values map[string]interface{}) error {
	var err error
	h.settings.DataDirHostPath, err = h.initTestDir(h.settings.Namespace)
	if err != nil {
		return err
	}

	var clusterCRD map[string]interface{}
	if err := yaml.Unmarshal([]byte(h.Manifests.GetCephCluster()), &clusterCRD); err != nil {
		return err
	}

	values["operatorNamespace"] = h.settings.OperatorNamespace
	values["configOverride"] = clusterCustomSettings
	values["toolbox"] = map[string]interface{}{
		"enabled": true,
		"image":   "rook/ceph:master",
	}
	values["cephClusterSpec"] = clusterCRD["spec"]

	logger.Infof("Creating ceph cluster using Helm with values: %+v", values)
	if err := h.helmHelper.InstallLocalRookHelmChart(h.settings.Namespace, CephClusterChartName, values); err != nil {
		return err
	}

	return nil
}
