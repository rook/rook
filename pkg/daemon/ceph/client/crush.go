/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
)

// CrushMap is the go representation of a CRUSH map
type CrushMap struct {
	Devices []struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Class string `json:"class"`
	} `json:"devices"`
	Types []struct {
		ID   int    `json:"type_id"`
		Name string `json:"name"`
	} `json:"types"`
	Buckets []struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		TypeID   int    `json:"type_id"`
		TypeName string `json:"type_name"`
		Weight   int    `json:"weight"`
		Alg      string `json:"alg"`
		Hash     string `json:"hash"`
		Items    []struct {
			ID     int `json:"id"`
			Weight int `json:"weight"`
			Pos    int `json:"pos"`
		} `json:"items"`
	} `json:"buckets"`
	Rules []struct {
		ID      int    `json:"rule_id"`
		Name    string `json:"rule_name"`
		Ruleset int    `json:"ruleset"`
		Type    int    `json:"type"`
		MinSize int    `json:"min_size"`
		MaxSize int    `json:"max_size"`
		Steps   []struct {
			Operation string `json:"op"`
			Number    int    `json:"num"`
			Item      int    `json:"item"`
			ItemName  string `json:"item_name"`
			Type      string `json:"type"`
		} `json:"steps"`
	} `json:"rules"`
	Tunables struct {
		// Add if necessary
	} `json:"tunables"`
}

// CrushFindResult is go representation of the Ceph osd find command output
type CrushFindResult struct {
	ID       int               `json:"osd"`
	IP       string            `json:"ip"`
	Host     string            `json:"host,omitempty"`
	Location map[string]string `json:"crush_location"`
}

// GetCrushMap fetches the Ceph CRUSH map
func GetCrushMap(context *clusterd.Context, clusterName string) (CrushMap, error) {
	var c CrushMap
	args := []string{"osd", "crush", "dump"}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return c, errors.Wrapf(err, "failed to get crush map")
	}

	err = json.Unmarshal(buf, &c)
	if err != nil {
		return c, errors.Wrapf(err, "failed to unmarshal crush map")
	}

	return c, nil
}

// FindOSDInCrushMap finds an OSD in the CRUSH map
func FindOSDInCrushMap(context *clusterd.Context, clusterName string, osdID int) (*CrushFindResult, error) {
	args := []string{"osd", "find", strconv.Itoa(osdID)}
	buf, err := NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find osd.%d in crush map: %s", osdID, string(buf))
	}

	var result CrushFindResult
	if err := json.Unmarshal(buf, &result); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal crush find result: %s", string(buf))
	}

	return &result, nil
}

// GetCrushHostName gets the hostname where an OSD is running on
func GetCrushHostName(context *clusterd.Context, clusterName string, osdID int) (string, error) {
	result, err := FindOSDInCrushMap(context, clusterName, osdID)
	if err != nil {
		return "", err
	}

	return result.Location["host"], nil
}

// NormalizeCrushName replaces . with -
func NormalizeCrushName(name string) string {
	return strings.Replace(name, ".", "-", -1)
}

// IsNormalizedCrushNameEqual returns true if normalized is either equal to or the normalized version of notNormalized
// a crush name is normalized if it comes from the crushmap or has passed through the NormalizeCrushName function.
func IsNormalizedCrushNameEqual(notNormalized, normalized string) bool {
	if notNormalized == normalized || NormalizeCrushName(notNormalized) == normalized {
		return true
	}
	return false
}

// UpdateCrushMapValue is for updating the location in the crush map
// this is not safe for incorrectly formatted strings
func UpdateCrushMapValue(pairs *[]string, key, value string) {
	found := false
	property := formatProperty(key, value)
	for i, pair := range *pairs {
		entry := strings.Split(pair, "=")
		if key == entry[0] {
			(*pairs)[i] = property
			found = true
		}
	}
	if !found {
		*pairs = append(*pairs, property)
	}
}

func isValidCrushFieldFormat(pair string) bool {
	matched, err := regexp.MatchString("^.+=.+$", pair)
	return matched && err == nil
}

func isCrushFieldSet(fieldName string, pairs []string) bool {
	for _, p := range pairs {
		kv := strings.Split(p, "=")
		if len(kv) == 2 && kv[0] == fieldName && kv[1] != "" {
			return true
		}
	}

	return false
}

func formatProperty(name, value string) string {
	return fmt.Sprintf("%s=%s", name, value)
}
