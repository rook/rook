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

package zone

import (
	"encoding/json"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
)

type masterZoneType struct {
	MasterZone string `json:"master_zone"`
}

func decodeMasterZone(data string) (string, error) {
	var zoneGroupGet masterZoneType
	err := json.Unmarshal([]byte(data), &zoneGroupGet)
	if err != nil {
		return "", errors.Wrap(err, "Failed to unmarshal json")
	}

	return zoneGroupGet.MasterZone, err
}

// validateZoneCR validates the zone arguments
func validateZoneCR(u *cephv1.CephObjectZone) error {
	if u.Name == "" {
		return errors.New("missing name")
	}
	if u.Namespace == "" {
		return errors.New("missing namespace")
	}
	if u.Spec.ZoneGroup == "" {
		return errors.New("missing zonegroup")
	}
	return nil
}
