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
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/rook/rook/pkg/clusterd"
)

const defaultCrushMap = `# begin crush map
tunable choose_local_tries 0
tunable choose_local_fallback_tries 0
tunable choose_total_tries 50
tunable chooseleaf_descend_once 1
tunable chooseleaf_vary_r 1
tunable chooseleaf_stable 0
tunable straw_calc_version 1
tunable allowed_bucket_algs 22

# types
type 0 osd
type 1 host
type 2 chassis
type 3 rack
type 4 row
type 5 pdu
type 6 pod
type 7 room
type 8 datacenter
type 9 region
type 10 root

# default bucket
root default {
	id -1   # do not change unnecessarily
	alg straw
	hash 0  # rjenkins1
}

# rules
rule replicated_ruleset {
	ruleset 0
	type replicated
	min_size 1
	max_size 10
	step take default
	step chooseleaf firstn 0 type host
	step emit
}

# end crush map
`

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

func SetCrushMap(context *clusterd.Context, clusterName, compiledMap string) (string, error) {
	args := []string{"osd", "setcrushmap", "-i", compiledMap}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return string(buf), fmt.Errorf("failed to set compiled crushmap. %v", err)
	}

	return string(buf), nil
}

func SetCrushTunables(context *clusterd.Context, clusterName, profile string) (string, error) {
	args := []string{"osd", "crush", "tunables", profile}
	buf, err := ExecuteCephCommandPlain(context, clusterName, args)
	if err != nil {
		return "", fmt.Errorf("%+v, %s", err, string(buf))
	}

	return string(buf), nil
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

func CreateDefaultCrushMap(context *clusterd.Context, clusterName string) (string, error) {
	// first set crush tunables to a firefly profile in order to support older clients
	// (e.g., hyperkube uses a firefly rbd tool)
	crushTunablesProfile := "firefly"
	logger.Infof("setting crush tunables to %s", crushTunablesProfile)
	output, err := SetCrushTunables(context, clusterName, crushTunablesProfile)
	if err != nil {
		return output, fmt.Errorf("failed to set crush tunables to profile %s: %+v", crushTunablesProfile, err)
	} else {
		logger.Infof("succeeded setting crush tunables to profile %s: %s", crushTunablesProfile, output)
	}

	// create a temp file that we will use to write the default decompiled crush map to
	decompiledMap, err := ioutil.TempFile("", "")
	if err != nil {
		return "", fmt.Errorf("failed to open decompiled crush map temp file: %+v", err)
	}
	defer decompiledMap.Close()
	defer os.Remove(decompiledMap.Name())

	// write the default decompiled crush map to the temp file
	_, err = decompiledMap.WriteString(defaultCrushMap)
	if err != nil {
		return "", fmt.Errorf("failed to write decompiled crush map to %s: %+v", decompiledMap.Name(), err)
	}

	// create a temp file to serve as the output file for the compilation process
	compiledMap, err := ioutil.TempFile("", "")
	if err != nil {
		return "", fmt.Errorf("failed to open compiled crush map temp file: %+v", err)
	}
	defer compiledMap.Close()
	defer os.Remove(compiledMap.Name())

	// compile the crush map to an output file
	args := []string{"-c", decompiledMap.Name(), "-o", compiledMap.Name()}
	output, err = context.Executor.ExecuteCommandWithOutput(false, "", CrushTool, args...)
	if err != nil {
		return output, fmt.Errorf("failed to compile crushmap from %s: %+v", decompiledMap.Name(), err)
	}

	// set the compiled crush map on the cluster
	output, err = SetCrushMap(context, clusterName, compiledMap.Name())
	if err != nil {
		return output, fmt.Errorf("failed to set crushmap to %s: %+v", compiledMap.Name(), err)
	}

	return "", nil
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
