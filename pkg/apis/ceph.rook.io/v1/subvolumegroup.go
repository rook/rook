/*
Copyright 2022 The Rook Authors. All rights reserved.

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

package v1

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ webhook.Validator = &CephFilesystemSubVolumeGroup{}

func (c *CephFilesystemSubVolumeGroup) ValidateCreate() (admission.Warnings, error) {
	return nil, nil
}

func (c *CephFilesystemSubVolumeGroup) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	logger.Infof("validate update %s/%s CephFilesystemSubVolumeGroup", c.Namespace, c.Name)
	sg := old.(*CephFilesystemSubVolumeGroup)
	if sg.Spec.FilesystemName != c.Spec.FilesystemName {
		return nil, errors.New("invalid update: filesystem name cannot be changed")
	}
	return nil, nil
}

func (c *CephFilesystemSubVolumeGroup) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}
