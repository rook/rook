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
package osd

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/rook/rook/pkg/cephmgr/client"
)

// calls osd getcrushmap
func GetCrushMap(adminConn client.Connection) (string, error) {
	cmd := map[string]interface{}{"prefix": "osd crush dump"}
	buf, err := client.ExecuteMonCommand(adminConn, cmd, fmt.Sprintf("retrieving crush map"))
	if err != nil {
		return "", fmt.Errorf("failed to get crush map. %v", err)
	}

	return string(buf), nil
}

func formatLocation(location string) ([]string, error) {
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

	if !isCrushFieldSet("hostName", pairs) {
		// host name isn't set yet, attempt to set a default
		hostName, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("failed to get hostname, %+v", err)
		}

		pairs = append(pairs, formatProperty("hostName", hostName))
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
