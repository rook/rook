/*
Copyright 2018 The Rook Authors. All rights reserved.

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
package client

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
)

type PGDumpBrief struct {
	PgStats []PgStats `json:"pg_stats"`
}

type PgStats struct {
	ID              string `json:"pgid"`
	State           string `json:"state"`
	UpOsdIDs        []int  `json:"up"`
	UpPrimaryID     int    `json:"up_primary"`
	ActingOsdIDs    []int  `json:"acting"`
	ActingPrimaryID int    `json:"acting_primary"`
}

func GetPGDumpBrief(context *clusterd.Context, clusterName string, isNautilusOrNewer bool) (*PGDumpBrief, error) {
	var pgDump PGDumpBrief
	var pgStats []PgStats
	args := []string{"pg", "dump", "pgs_brief"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get pg dump")
	}

	if !isNautilusOrNewer {
		if err := json.Unmarshal(buf, &pgStats); err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal pg dump response")
		}
		pgDump = PGDumpBrief{
			PgStats: pgStats,
		}
		return &pgDump, nil
	}

	if err := json.Unmarshal(buf, &pgDump); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal pg dump response")
	}

	return &pgDump, nil
}
