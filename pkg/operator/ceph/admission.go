/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	"github.com/rook/rook/pkg/clusterd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"strconv"
)

var (
	cephclusterResource   = metav1.GroupVersionResource{Group: cephv1.CustomResourceGroup, Version: cephv1.Version, Resource: opcontroller.ClusterResource.Plural}
	rookClientSet         rookclient.Interface
	universalDeserializer = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
)

func validateUpdatedCephCluster(updatedCephCluster cephv1.CephCluster, found *cephv1.CephCluster) error {
	if updatedCephCluster.Spec.DataDirHostPath != found.Spec.DataDirHostPath {
		return errors.Errorf("invalid update : DataDirHostPath change from %q to %q is not allowed", found.Spec.DataDirHostPath, updatedCephCluster.Spec.DataDirHostPath)
	}

	if updatedCephCluster.Spec.Network.HostNetwork != found.Spec.Network.HostNetwork {
		return errors.Errorf("invalid update : HostNetwork change from %q to %q is not allowed", strconv.FormatBool(found.Spec.Network.HostNetwork), strconv.FormatBool(updatedCephCluster.Spec.Network.HostNetwork))
	}

	if updatedCephCluster.Spec.Network.Provider != found.Spec.Network.Provider {
		return errors.Errorf("invalid update : Provider change from %q to %q is not allowed", found.Spec.Network.Provider, updatedCephCluster.Spec.Network.Provider)
	}

	return nil
}

func ValidateCephResource(request *v1beta1.AdmissionRequest, context *clusterd.Context) error {
	rookClientSet = context.RookClientset
	raw := request.Object.Raw
	reqCephCluster := cephv1.CephCluster{}
	// Deserializing the incoming CephCluster object
	if _, _, err := universalDeserializer.Decode(raw, nil, &reqCephCluster); err != nil {
		return errors.Wrap(err, "failed to deserialize pod object")
	}
	// Checks if CephCluster is being updated and rejects the request if the dataDirHostPath is changed from initial value
	if isCephClusterUpdate(request) {
		// Fetch the existing ceph cluster object
		found, err := fetchExistingCephCluster(request)
		if err != nil {
			return errors.Wrap(err, "failed to fetch existing cephcluster")
		}
		err = validateUpdatedCephCluster(reqCephCluster, found)
		if err != nil {
			return errors.Wrap(err, "failed to validate updated cephcluster")
		}
	}
	if isCephClusterCreate(request) {
		//If external mode enabled, then check if other fields are empty
		if reqCephCluster.Spec.External.Enable {
			if reqCephCluster.Spec.Mon != (cephv1.MonSpec{}) || reqCephCluster.Spec.Dashboard != (cephv1.DashboardSpec{}) || reqCephCluster.Spec.Monitoring != (cephv1.MonitoringSpec{}) || reqCephCluster.Spec.DisruptionManagement != (cephv1.DisruptionManagementSpec{}) || len(reqCephCluster.Spec.Mgr.Modules) > 0 || len(reqCephCluster.Spec.Network.Provider) > 0 || len(reqCephCluster.Spec.Network.Selectors) > 0 {
				return errors.New("invalid create : external mode enabled cannot have mon,dashboard,monitoring,network,disruptionManagement,storage fields in CR")
			}

		}
	}
	return nil
}

func isCephClusterCreate(request *v1beta1.AdmissionRequest) bool {
	return request.Resource == cephclusterResource && request.Operation == v1beta1.Create
}

func fetchExistingCephCluster(request *v1beta1.AdmissionRequest) (*cephv1.CephCluster, error) {
	found, err := rookClientSet.CephV1().CephClusters(request.Namespace).Get(request.Name, metav1.GetOptions{})
	if err != nil {
		return &cephv1.CephCluster{}, errors.Wrap(err, "failed to find existing cephcluster object")
	}
	return found, nil
}

func isCephClusterUpdate(request *v1beta1.AdmissionRequest) bool {
	return request.Resource == cephclusterResource && request.Operation == v1beta1.Update
}
