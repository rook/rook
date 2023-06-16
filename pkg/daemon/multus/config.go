/*
Copyright 2023 The Rook Authors. All rights reserved.

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

package multus

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rook/rook/pkg/operator/k8sutil"
	"gopkg.in/yaml.v2"
)

var (
	//go:embed config.yaml
	ConfigYaml string
)

var (
	DefaultValidationNamespace = "rook-ceph"

	// 1 mon, 3 osds, 1 mgr, 1 mds, 1 nfs, 1 rgw, 1 rbdmirror, 1 cephfsmirror,
	// (1 csi provisioner, 1 csi plugin) x3 for rbd, cephfs, and nfs CSI drivers
	DefaultValidationDaemonsPerNode = 16

	DefaultValidationNginxImage = "nginxinc/nginx-unprivileged:stable-alpine"

	DefaultValidationResourceTimeout = 3 * time.Minute
)

func init() {
	// the default namespace is the current namespace the operator pod is running in if possible
	ns := os.Getenv(k8sutil.PodNamespaceEnvVar)
	if ns != "" {
		DefaultValidationNamespace = ns
	}
}

// ValidationTestConfig is a configuration for a Multus validation test. To prevent documentation
// for this struct from getting out of date, see the output of ValidationTestConfig.ToYAML() for
// usage text for each field.
type ValidationTestConfig struct {
	Namespace       string        `yaml:"namespace"`
	PublicNetwork   string        `yaml:"publicNetwork"`
	ClusterNetwork  string        `yaml:"clusterNetwork"`
	DaemonsPerNode  int           `yaml:"daemonsPerNode"`
	ResourceTimeout time.Duration `yaml:"resourceTimeout"`
	NginxImage      string        `yaml:"nginxImage"`
}

// NewDefaultValidationTestConfig returns a new ValidationTestConfig with default values.
func NewDefaultValidationTestConfig() *ValidationTestConfig {
	c := &ValidationTestConfig{}
	c.Namespace = DefaultValidationNamespace
	c.DaemonsPerNode = DefaultValidationDaemonsPerNode
	c.ResourceTimeout = DefaultValidationResourceTimeout
	c.NginxImage = DefaultValidationNginxImage
	return c
}

// ToYAML converts the validation test config into a YAML representation with user-readable comments
// describing how to use the various parameters.
func (c *ValidationTestConfig) ToYAML() (string, error) {
	// No Go YAML libraries seem to support fields with default-comments attached to them. It would
	// be silly to use some super-advanced reflection techniques or to extend our own YAML library,
	// so it is at least straightforward to render the config file from a Go template.
	t, err := loadTemplate("config.yaml", ConfigYaml, c)
	if err != nil {
		return "", fmt.Errorf("failed to load config into yaml template: %w", err)
	}
	return string(t), nil
}

// String implements the Stringer interface
func (c *ValidationTestConfig) String() string {
	out, err := yaml.Marshal(c)
	if err != nil {
		return "failed quick marshal of validation test config!"
	}
	return string(out)
}

// ValidationTestConfigFromYAML loads a YAML-formatted string into a new ValidationTestConfig.
func ValidationTestConfigFromYAML(y string) (*ValidationTestConfig, error) {
	c := &ValidationTestConfig{}
	err := yaml.Unmarshal([]byte(y), c)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config from yaml: %w", err)
	}
	return c, nil
}

// Validate reports any validation test configuration problems as errors.
func (c *ValidationTestConfig) Validate() error {
	errs := []string{}
	if c.Namespace == "" {
		errs = append(errs, "namespace must be specified")
	}
	if c.PublicNetwork == "" && c.ClusterNetwork == "" {
		errs = append(errs, "at least one of publicNetwork and clusterNetwork must be specified")
	}
	if c.DaemonsPerNode == 0 {
		errs = append(errs, "daemonsPerNode must be nonzero")
	}
	if c.ResourceTimeout < 1*time.Minute {
		errs = append(errs, "resourceTimeout must be at least one minute (two or more are recommended)")
	}
	if c.NginxImage == "" {
		errs = append(errs, "nginxImage must be specified")
	}
	if len(errs) > 0 {
		return fmt.Errorf("validation test config is invalid: %s", strings.Join(errs, ", "))
	}
	return nil
}
