// Copyright © 2022 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package patch

import (
	"emperror.dev/errors"
	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/util/jsonmergepatch"
	"k8s.io/apimachinery/pkg/util/mergepatch"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

type StrategicMergePatcher interface {
	StrategicMergePatch(original, patch []byte, dataStruct interface{}) ([]byte, error)
	CreateTwoWayMergePatch(original, modified []byte, dataStruct interface{}) ([]byte, error)
	CreateThreeWayMergePatch(original, modified, current []byte, dataStruct interface{}) ([]byte, error)
}

type JSONMergePatcher interface {
	MergePatch(docData, patchData []byte) ([]byte, error)
	CreateMergePatch(originalJSON, modifiedJSON []byte) ([]byte, error)
	CreateThreeWayJSONMergePatch(original, modified, current []byte) ([]byte, error)
}

type K8sStrategicMergePatcher struct {
	PreconditionFuncs []mergepatch.PreconditionFunc
}

func (p *K8sStrategicMergePatcher) StrategicMergePatch(original, patch []byte, dataStruct interface{}) ([]byte, error) {
	return strategicpatch.StrategicMergePatch(original, patch, dataStruct)
}

func (p *K8sStrategicMergePatcher) CreateTwoWayMergePatch(original, modified []byte, dataStruct interface{}) ([]byte, error) {
	return strategicpatch.CreateTwoWayMergePatch(original, modified, dataStruct, p.PreconditionFuncs...)
}

func (p *K8sStrategicMergePatcher) CreateThreeWayMergePatch(original, modified, current []byte, dataStruct interface{}) ([]byte, error) {
	lookupPatchMeta, err := strategicpatch.NewPatchMetaFromStruct(dataStruct)
	if err != nil {
		return nil, errors.WrapWithDetails(err, "Failed to lookup patch meta", "current object", dataStruct)
	}

	return strategicpatch.CreateThreeWayMergePatch(original, modified, current, lookupPatchMeta, true, p.PreconditionFuncs...)
}

type BaseJSONMergePatcher struct{}

func (p *BaseJSONMergePatcher) MergePatch(docData, patchData []byte) ([]byte, error) {
	return jsonpatch.MergePatch(docData, patchData)
}

func (p *BaseJSONMergePatcher) CreateMergePatch(originalJSON, modifiedJSON []byte) ([]byte, error) {
	return jsonpatch.CreateMergePatch(originalJSON, modifiedJSON)
}

func (p *BaseJSONMergePatcher) CreateThreeWayJSONMergePatch(original, modified, current []byte) ([]byte, error) {
	return jsonmergepatch.CreateThreeWayJSONMergePatch(original, modified, current)
}
