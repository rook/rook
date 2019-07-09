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

	"github.com/rook/rook/pkg/clusterd"
)

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

type CrushFindResult struct {
	ID       int    `json:"osd"`
	IP       string `json:"ip"`
	Location struct {
		// add more crush location fields if needed
		Root string `json:"root"`
		Host string `json:"host"`
	} `json:"crush_location"`
}

func GetCrushMap(context *clusterd.Context, clusterName string) (CrushMap, error) {
	var c CrushMap
	args := []string{"osd", "crush", "dump"}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return c, fmt.Errorf("failed to get crush map. %v", err)
	}

	err = json.Unmarshal(buf, &c)
	if err != nil {
		return c, fmt.Errorf("failed to unmarshal crush map. %+v", err)
	}

	return c, nil
}

func CrushReweight(context *clusterd.Context, clusterName string, id int, weight float64) (string, error) {
	args := []string{"osd", "crush", "reweight", fmt.Sprintf("osd.%d", id), fmt.Sprintf("%.1f", weight)}
	buf, err := ExecuteCephCommand(context, clusterName, args)

	return string(buf), err
}

func CrushRemove(context *clusterd.Context, clusterName, name string) (string, error) {
	args := []string{"osd", "crush", "rm", name}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return "", fmt.Errorf("failed to crush rm: %+v, %s", err, string(buf))
	}

	return string(buf), nil
}

func FindOSDInCrushMap(context *clusterd.Context, clusterName string, osdID int) (*CrushFindResult, error) {
	args := []string{"osd", "find", strconv.Itoa(osdID)}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to find osd.%d in crush map: %+v, %s", osdID, err, string(buf))
	}

	var result CrushFindResult
	if err := json.Unmarshal(buf, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal crush find result: %+v. raw: %s", err, string(buf))
	}

	return &result, nil
}

func GetCrushHostName(context *clusterd.Context, clusterName string, osdID int) (string, error) {
	result, err := FindOSDInCrushMap(context, clusterName, osdID)
	if err != nil {
		return "", err
	}

	return result.Location.Host, nil
}

func FormatLocation(location, hostName string) ([]string, error) {
	var pairs []string
	if location == "" {
		pairs = []string{}
	} else {
		pairs = strings.Split(location, ",")
	}

	for _, p := range pairs {
		if !isValidCrushFieldFormat(p) {
			return nil, fmt.Errorf("CRUSH location field '%s' is not in a valid format", p)
		}
	}

	// set a default root if it's not already set
	if !isCrushFieldSet("root", pairs) {
		pairs = append(pairs, formatProperty("root", "default"))
	}
	// set the host name
	if !isCrushFieldSet("host", pairs) {
		// keep the fully qualified host name in the crush map, but replace the dots with dashes to satisfy ceph
		hostName = strings.Replace(hostName, ".", "-", -1)
		pairs = append(pairs, formatProperty("host", hostName))
	}

	return pairs, nil
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
