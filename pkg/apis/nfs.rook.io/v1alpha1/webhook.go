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

package v1alpha1

import (
	"strings"

	"github.com/coreos/pkg/capnslog"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/kustomize/kyaml/sets"
)

var (
	webhookName = "nfs-webhook"
	logger      = capnslog.NewPackageLogger("github.com/rook/rook", webhookName)
)

// compile-time assertions ensures NFSServer implements webhook.Defaulter so a webhook builder
// will be registered for the mutating webhook.
var _ webhook.Defaulter = &NFSServer{}

// Default implements webhook.Defaulter contains mutating webhook admission logic.
func (r *NFSServer) Default() {
	logger.Info("default", "name", r.Name)
	logger.Warning("defaulting is not supported yet")
}

// compile-time assertions ensures NFSServer implements webhook.Validator so a webhook builder
// will be registered for the validating webhook.
var _ webhook.Validator = &NFSServer{}

// ValidateCreate implements webhook.Validator contains validating webhook admission logic for CREATE operation
func (r *NFSServer) ValidateCreate() error {
	logger.Info("validate create", "name", r.Name)

	if err := r.ValidateSpec(); err != nil {
		return err
	}

	return nil
}

// ValidateUpdate implements webhook.Validator contains validating webhook admission logic for UPDATE operation
func (r *NFSServer) ValidateUpdate(old runtime.Object) error {
	logger.Info("validate update", "name", r.Name)

	if err := r.ValidateSpec(); err != nil {
		return err
	}

	return nil
}

// ValidateDelete implements webhook.Validator contains validating webhook admission logic for DELETE operation
func (r *NFSServer) ValidateDelete() error {
	logger.Info("validate delete", "name", r.Name)
	logger.Warning("validating delete event is not supported")

	return nil
}

// ValidateSpec validate NFSServer spec.
func (r *NFSServer) ValidateSpec() error {
	var allErrs field.ErrorList

	spec := r.Spec
	specPath := field.NewPath("spec")
	allErrs = append(allErrs, spec.validateExports(specPath)...)

	return allErrs.ToAggregate()
}

func (r *NFSServerSpec) validateExports(parentPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	exportsPath := parentPath.Child("exports")
	allNames := sets.String{}
	allPVCNames := sets.String{}
	for i, export := range r.Exports {
		idxPath := exportsPath.Index(i)
		namePath := idxPath.Child("name")
		errList := field.ErrorList{}
		if allNames.Has(export.Name) {
			errList = append(errList, field.Duplicate(namePath, export.Name))
		}

		pvcNamePath := idxPath.Child("persistentVolumeClaim", "claimName")
		if allPVCNames.Has(export.PersistentVolumeClaim.ClaimName) {
			errList = append(errList, field.Duplicate(pvcNamePath, export.PersistentVolumeClaim.ClaimName))
		}

		if len(errList) == 0 {
			allNames.Insert(export.Name)
			allPVCNames.Insert(export.PersistentVolumeClaim.ClaimName)
		} else {
			allErrs = append(allErrs, errList...)
		}

		allErrs = append(allErrs, export.validateServer(idxPath)...)
	}

	return allErrs
}

func (r *ExportsSpec) validateServer(parentPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	server := r.Server
	serverPath := parentPath.Child("server")
	accessModePath := serverPath.Child("accessMode")
	if err := validateAccessMode(accessModePath, server.AccessMode); err != nil {
		allErrs = append(allErrs, err)
	}

	squashPath := serverPath.Child("squash")
	if err := validateSquashMode(squashPath, server.Squash); err != nil {
		allErrs = append(allErrs, err)
	}

	allErrs = append(allErrs, server.validateAllowedClient(serverPath)...)

	return allErrs
}

func (r *ServerSpec) validateAllowedClient(parentPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	allowedClientsPath := parentPath.Child("allowedClients")
	allNames := sets.String{}
	for i, allowedClient := range r.AllowedClients {
		idxPath := allowedClientsPath.Index(i)
		namePath := idxPath.Child("name")
		errList := field.ErrorList{}
		if allNames.Has(allowedClient.Name) {
			errList = append(errList, field.Duplicate(namePath, allowedClient.Name))
		}

		if len(errList) == 0 {
			allNames.Insert(allowedClient.Name)
		} else {
			allErrs = append(allErrs, errList...)
		}

		accessModePath := idxPath.Child("accessMode")
		if err := validateAccessMode(accessModePath, allowedClient.AccessMode); err != nil {
			allErrs = append(allErrs, err)
		}

		squashPath := idxPath.Child("squash")
		if err := validateSquashMode(squashPath, allowedClient.Squash); err != nil {
			allErrs = append(allErrs, err)
		}
	}

	return allErrs
}

func validateAccessMode(path *field.Path, mode string) *field.Error {
	switch strings.ToLower(mode) {
	case "readonly":
	case "readwrite":
	case "none":
	default:
		return field.Invalid(path, mode, "valid values are (ReadOnly, ReadWrite, none)")
	}
	return nil
}

func validateSquashMode(path *field.Path, mode string) *field.Error {
	switch strings.ToLower(mode) {
	case "rootid":
	case "root":
	case "all":
	case "none":
	default:
		return field.Invalid(path, mode, "valid values are (none, rootId, root, all)")
	}
	return nil
}
