/*
Copyright 2018 The Kubernetes Authors.

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

package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"net/http"
	"reflect"

	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

// cassandraValidator validates Pods
type cassandraValidator struct {
	client  client.Client
	decoder types.Decoder
}

// Implement admission.Handler so the controller can handle admission request.
var _ admission.Handler = &cassandraValidator{}

// cassandraValidator admits a pod iff a specific annotation exists.
func (v *cassandraValidator) Handle(ctx context.Context, req types.Request) types.Response {

	old, new := &cassandrav1alpha1.Cluster{}, &cassandrav1alpha1.Cluster{}
	if err := unmarshalObjects(req.AdmissionRequest, old, new); err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	allowed, reason := v.validateValueFn(new)
	if allowed && old != nil {
		allowed, reason := v.validateTransitionsFn(old, new)
		return admission.ValidationResponse(allowed, reason)
	}
	return admission.ValidationResponse(allowed, reason)
}

func (v *cassandraValidator) validateValueFn(cluster *cassandrav1alpha1.Cluster) (bool, string) {
	rackNames := sets.NewString()
	for _, rack := range cluster.Spec.Datacenter.Racks {
		// Check that no two racks have the same name
		if rackNames.Has(rack.Name) {
			return false, fmt.Sprintf("two racks have the same name: '%s'", rack.Name)
		}
		rackNames.Insert(rack.Name)

		// Check that persistent storage is configured
		if rack.Storage.VolumeClaimTemplates == nil {
			return false, fmt.Sprintf("rack '%s' has no volumeClaimTemplates defined", rack.Name)
		}

		// Check that only one disk is present
		if len(rack.Storage.VolumeClaimTemplates) > 1 {
			return false, fmt.Sprintf("rack '%s' has more than one volumeClaimTemplates, currently only 1 is supported", rack.Name)
		}

		// Check that configMapName is not set
		if rack.ConfigMapName != nil {
			return false, fmt.Sprintf("rack '%s' has configMapName set which is currently not supported", rack.Name)
		}

	}

	return true, ""
}

func (v *cassandraValidator) validateTransitionsFn(old, new *cassandrav1alpha1.Cluster) (bool, string) {
	// Check that version remained the same
	if old.Spec.Version != new.Spec.Version {
		return false, "change of version is currently not supported"
	}

	// Check that repository remained the same
	if old.Spec.Repository != new.Spec.Repository {
		return false, "repository change is currently not supported"
	}

	// Check that mode remained the same
	if old.Spec.Mode != new.Spec.Mode {
		return false, "change of mode is currently not supported"
	}

	// Check that sidecarImage remained the same
	if !reflect.DeepEqual(old.Spec.SidecarImage, new.Spec.SidecarImage) {
		return false, "change of sidecarImage is currently not supported"
	}

	// Check that the datacenter name didn't change
	if old.Spec.Datacenter.Name != new.Spec.Datacenter.Name {
		return false, "change of datacenter name is currently not supported"
	}

	// Check that all rack names are the same as before
	oldRackNames, newRackNames := sets.NewString(), sets.NewString()
	for _, rack := range old.Spec.Datacenter.Racks {
		oldRackNames.Insert(rack.Name)
	}
	for _, rack := range new.Spec.Datacenter.Racks {
		newRackNames.Insert(rack.Name)
	}
	diff := oldRackNames.Difference(newRackNames)
	if diff.Len() != 0 {
		return false, fmt.Sprintf("racks %v not found, you cannot remove racks from the spec", diff.List())
	}

	rackMap := make(map[string]cassandrav1alpha1.RackSpec)
	for _, oldRack := range old.Spec.Datacenter.Racks {
		rackMap[oldRack.Name] = oldRack
	}
	for _, newRack := range new.Spec.Datacenter.Racks {
		oldRack := rackMap[newRack.Name]

		// Check that placement is the same as before
		if !reflect.DeepEqual(oldRack.Placement, newRack.Placement) {
			return false, fmt.Sprintf("rack %s: changes in placement are not currently supported", oldRack.Name)
		}

		// Check that storage is the same as before
		if !reflect.DeepEqual(oldRack.Storage, newRack.Storage) {
			return false, fmt.Sprintf("rack %s: changes in storage are not currently supported", oldRack.Name)
		}

		// Check that resources are the same as before
		if !reflect.DeepEqual(oldRack.Resources, newRack.Resources) {
			return false, fmt.Sprintf("rack %s: changes in resources are not currently supported", oldRack.Name)
		}
	}

	return true, ""
}

// cassandraValidator implements inject.Client.
// A client will be automatically injected.
var _ inject.Client = &cassandraValidator{}

// InjectClient injects the client.
func (v *cassandraValidator) InjectClient(c client.Client) error {
	v.client = c
	return nil
}

// cassandraValidator implements inject.Decoder.
// A decoder will be automatically injected.
var _ inject.Decoder = &cassandraValidator{}

// InjectDecoder injects the decoder.
func (v *cassandraValidator) InjectDecoder(d types.Decoder) error {
	v.decoder = d
	return nil
}

// unmarshalObjects unmarshals the old and new objects out of an AdmissionRequest
func unmarshalObjects(req *admissionv1beta1.AdmissionRequest, old, new runtime.Object) error {

	if err := json.Unmarshal(req.Object.Raw, new); err != nil {
		logger.Errorf("Could not unmarshal raw object: %v", err)
		return err
	}
	if err := json.Unmarshal(req.OldObject.Raw, old); err != nil {
		logger.Errorf("Could not unmarshal raw oldObject: %v", err)
		return err
	}
	return nil
}
