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
	"strconv"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
)

const (
	CrushRootConfigKey = "crushRoot"
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
	Rules    []ruleSpec `json:"rules"`
	Tunables struct {
		// Add if necessary
	} `json:"tunables"`
}

type ruleSpec struct {
	ID      int        `json:"rule_id"`
	Name    string     `json:"rule_name"`
	Ruleset int        `json:"ruleset"`
	Type    int        `json:"type"`
	MinSize int        `json:"min_size"`
	MaxSize int        `json:"max_size"`
	Steps   []stepSpec `json:"steps"`
}

type stepSpec struct {
	Operation string `json:"op"`
	Number    uint   `json:"num"`
	Item      int    `json:"item"`
	ItemName  string `json:"item_name"`
	Type      string `json:"type"`
}

// CrushFindResult is go representation of the Ceph osd find command output
type CrushFindResult struct {
	ID       int               `json:"osd"`
	IP       string            `json:"ip"`
	Host     string            `json:"host,omitempty"`
	Location map[string]string `json:"crush_location"`
}

// GetCrushMap fetches the Ceph CRUSH map
func GetCrushMap(context *clusterd.Context, clusterInfo *ClusterInfo) (CrushMap, error) {
	var c CrushMap
	args := []string{"osd", "crush", "dump"}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return c, errors.Wrapf(err, "failed to get crush map. %s", string(buf))
	}

	err = json.Unmarshal(buf, &c)
	if err != nil {
		return c, errors.Wrap(err, "failed to unmarshal crush map")
	}

	return c, nil
}

// GetCompiledCrushMap fetches the Ceph compiled version of the CRUSH map
func GetCompiledCrushMap(context *clusterd.Context, clusterInfo *ClusterInfo) (string, error) {
	compiledCrushMapFile, err := ioutil.TempFile("", "")
	if err != nil {
		return "", errors.Wrap(err, "failed to generate temporarily file")
	}

	args := []string{"osd", "getcrushmap", "--out-file", compiledCrushMapFile.Name()}
	exec := NewCephCommand(context, clusterInfo, args)
	exec.JsonOutput = false
	buf, err := exec.Run()
	if err != nil {
		return "", errors.Wrapf(err, "failed to get compiled crush map. %s", string(buf))
	}

	return compiledCrushMapFile.Name(), nil
}

// FindOSDInCrushMap finds an OSD in the CRUSH map
func FindOSDInCrushMap(context *clusterd.Context, clusterInfo *ClusterInfo, osdID int) (*CrushFindResult, error) {
	args := []string{"osd", "find", strconv.Itoa(osdID)}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
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
func GetCrushHostName(context *clusterd.Context, clusterInfo *ClusterInfo, osdID int) (string, error) {
	result, err := FindOSDInCrushMap(context, clusterInfo, osdID)
	if err != nil {
		return "", err
	}

	return result.Location["host"], nil
}

// NormalizeCrushName replaces . with -
func NormalizeCrushName(name string) string {
	return strings.Replace(name, ".", "-", -1)
}

// Obtain the cluster-wide default crush root from the cluster spec
func GetCrushRootFromSpec(c *cephv1.ClusterSpec) string {
	if c.Storage.Config == nil {
		return cephv1.DefaultCRUSHRoot
	}
	if v, ok := c.Storage.Config[CrushRootConfigKey]; ok {
		return v
	}
	return cephv1.DefaultCRUSHRoot
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

func formatProperty(name, value string) string {
	return fmt.Sprintf("%s=%s", name, value)
}

// GetOSDOnHost returns the list of osds running on a given host
func GetOSDOnHost(context *clusterd.Context, clusterInfo *ClusterInfo, node string) (string, error) {
	node = NormalizeCrushName(node)
	args := []string{"osd", "crush", "ls", node}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return "", errors.Wrapf(err, "failed to get osd list on host. %s", string(buf))
	}

	return string(buf), nil
}

func compileCRUSHMap(context *clusterd.Context, crushMapPath string) error {
	mapFile := buildCompileCRUSHFileName(crushMapPath)
	args := []string{"--compile", crushMapPath, "--outfn", mapFile}
	output, err := context.Executor.ExecuteCommandWithOutput("crushtool", args...)
	if err != nil {
		return errors.Wrapf(err, "failed to compile crush map %q. %s", mapFile, output)
	}

	return nil
}

func decompileCRUSHMap(context *clusterd.Context, crushMapPath string) error {
	mapFile := buildDecompileCRUSHFileName(crushMapPath)
	args := []string{"--decompile", crushMapPath, "--outfn", mapFile}
	output, err := context.Executor.ExecuteCommandWithOutput("crushtool", args...)
	if err != nil {
		return errors.Wrapf(err, "failed to decompile crush map %q. %s", mapFile, output)
	}

	return nil
}

func injectCRUSHMap(context *clusterd.Context, clusterInfo *ClusterInfo, crushMapPath string) error {
	args := []string{"osd", "setcrushmap", "--in-file", crushMapPath}
	exec := NewCephCommand(context, clusterInfo, args)
	exec.JsonOutput = false
	buf, err := exec.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to inject crush map %q. %s", crushMapPath, string(buf))
	}

	return nil
}

func setCRUSHMap(context *clusterd.Context, clusterInfo *ClusterInfo, crushMapPath string) error {
	args := []string{"osd", "crush", "set", crushMapPath}
	exec := NewCephCommand(context, clusterInfo, args)
	exec.JsonOutput = false
	buf, err := exec.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to set crush map %q. %s", crushMapPath, string(buf))
	}

	return nil
}

func buildDecompileCRUSHFileName(crushMapPath string) string {
	return fmt.Sprintf("%s.decompiled", crushMapPath)
}

func buildCompileCRUSHFileName(crushMapPath string) string {
	return fmt.Sprintf("%s.compiled", crushMapPath)
}
