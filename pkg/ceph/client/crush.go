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
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
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

func GetCrushMap(context *clusterd.Context, clusterName string) (string, error) {
	args := []string{"osd", "crush", "dump"}
	buf, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return "", fmt.Errorf("failed to get crush map. %v", err)
	}

	return string(buf), nil
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
	output, err = context.Executor.ExecuteCommandWithOutput("", CrushTool, args...)
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

func FormatLocation(location string) ([]string, error) {
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
