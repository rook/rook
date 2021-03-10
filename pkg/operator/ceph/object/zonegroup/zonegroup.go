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

package zonegroup

import (
	"encoding/json"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
)

type masterZoneGroupType struct {
	MasterZoneGroup string `json:"master_zonegroup"`
}

func decodeMasterZoneGroup(data string) (string, error) {
	var periodGet masterZoneGroupType
	err := json.Unmarshal([]byte(data), &periodGet)
	if err != nil {
		return "", errors.Wrap(err, "Failed to unmarshal json")
	}

	return periodGet.MasterZoneGroup, err
}

// validateZoneGroup validates the zonegroup arguments
func validateZoneGroup(u *cephv1.CephObjectZoneGroup) error {
	if u.Name == "" {
		return errors.New("missing name")
	}
	if u.Namespace == "" {
		return errors.New("missing namespace")
	}
	if u.Spec.Realm == "" {
		return errors.New("missing realm")
	}
	return nil
}
