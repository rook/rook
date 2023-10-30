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
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rook/rook/pkg/operator/k8sutil"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metavalidation "k8s.io/apimachinery/pkg/util/validation"
)

var (
	//go:embed config.yaml
	ConfigYaml string
)

var (
	DefaultValidationNamespace = "rook-ceph"

	DefaultValidationOSDsPerNode = 3

	DefaultValidationOtherDaemonsPerNode = 16

	DefaultValidationNginxImage = "nginxinc/nginx-unprivileged:stable-alpine"

	DefaultValidationResourceTimeout = 3 * time.Minute

	DefaultValidationFlakyThreshold = 30 * time.Second

	DefaultStorageNodeLabelKey   = "storage-node"
	DefaultStorageNodeLabelValue = "true"

	DefaultArbiterNodeLabelKey   = "topology.kubernetes.io/zone"
	DefaultArbiterNodeLabelValue = "arbiter"
	DefaultArbiterTolerationKey  = "node-role.kubernetes.io/control-plane"
)

const DefaultValidationNodeType = "shared-storage-and-worker-nodes"

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
	Namespace       string                `yaml:"namespace"`
	PublicNetwork   string                `yaml:"publicNetwork"`
	ClusterNetwork  string                `yaml:"clusterNetwork"`
	ResourceTimeout time.Duration         `yaml:"resourceTimeout"`
	FlakyThreshold  time.Duration         `yaml:"flakyThreshold"`
	NginxImage      string                `yaml:"nginxImage"`
	NodeTypes       map[string]NodeConfig `yaml:"nodeTypes"`
}

type NodeConfig struct {
	// OSD daemons per node
	OSDsPerNode int `yaml:"osdsPerNode"`
	// Non-OSD daemons per node.
	OtherDaemonsPerNode int             `yaml:"otherDaemonsPerNode"`
	Placement           PlacementConfig `yaml:"placement"`
}

// NodeSelector and Tolerations are intentionally the only configurable parameters here.
// Affinity/Anti-Affinity is too relaxed of a specification to ensure the validation test runs the
// exact number of daemons per node that it should be running. Only allow the minimum selection
// configs that can be used to define nodes this test can run on.
type PlacementConfig struct {
	NodeSelector map[string]string `yaml:"nodeSelector"`
	Tolerations  []TolerationType  `yaml:"tolerations"`
}

type TolerationType corev1.Toleration

// ToJSON renders a toleration as a single-line JSON string. The JSON rendering is just as easy to
// read as the YAML rendering and is easier to format in the config.yaml template using Golang
// text templating compared to YAML.
// Need to define our own because corev1.Toleration.Marshal() does not render as expected.
func (t *TolerationType) ToJSON() (string, error) {
	j, err := json.Marshal(*t)
	if err != nil {
		return "", fmt.Errorf("failed to convert toleration into JSON: %w", err)
	}
	return string(j), nil
}

// NewDefaultValidationTestConfig returns a new ValidationTestConfig with default values.
// The default test is a converged-node test with no placement.
func NewDefaultValidationTestConfig() *ValidationTestConfig {
	return &ValidationTestConfig{
		Namespace:       DefaultValidationNamespace,
		ResourceTimeout: DefaultValidationResourceTimeout,
		FlakyThreshold:  DefaultValidationFlakyThreshold,
		NginxImage:      DefaultValidationNginxImage,
		NodeTypes: map[string]NodeConfig{
			DefaultValidationNodeType: {
				OSDsPerNode:         DefaultValidationOSDsPerNode,
				OtherDaemonsPerNode: DefaultValidationOtherDaemonsPerNode,
				// Placement empty
			},
		},
	}
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

func (c *ValidationTestConfig) TotalDaemonsPerNode() int {
	return c.TotalOSDsPerNode() + c.TotalOtherDaemonsPerNode()
}

func (c *ValidationTestConfig) TotalOSDsPerNode() int {
	t := 0
	for _, config := range c.NodeTypes {
		t += config.OSDsPerNode
	}
	return t
}

func (c *ValidationTestConfig) TotalOtherDaemonsPerNode() int {
	t := 0
	for _, config := range c.NodeTypes {
		t += config.OtherDaemonsPerNode
	}
	return t
}

func (c *ValidationTestConfig) BestNodePlacementForServer() (PlacementConfig, error) {
	// the web server MUST be placed on a node with both public and cluster networks available
	// since OSDs must have both, picking a node type with OSDs is a GOOD guess
	// BEST can't be determined easily, but a good approximation of BEST is the node likely to have
	// the most system resources available
	// the node type with the most OSDs will have high overall resource needs in production, which
	// is a good approximation of most overall system resources
	// in the case of a tie for num OSDs, more overall daemons means more resources
	best := NodeConfig{}
	for _, config := range c.NodeTypes {
		if (config.OSDsPerNode > best.OSDsPerNode) ||
			(config.OSDsPerNode == best.OSDsPerNode && config.OtherDaemonsPerNode > best.OtherDaemonsPerNode) {
			best = config
		}
	}
	if best.OSDsPerNode == 0 {
		return PlacementConfig{}, fmt.Errorf("cannot place web server in cluster with no OSDs")
	}
	return best.Placement, nil
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
	if c.ResourceTimeout < 1*time.Minute {
		errs = append(errs, "resourceTimeout must be at least one minute (two or more are recommended)")
	}
	if c.FlakyThreshold < 5*time.Second {
		errs = append(errs, "flaky threshold must be at least 5 seconds")
	}
	if c.NginxImage == "" {
		errs = append(errs, "nginxImage must be specified")
	}
	if c.TotalOSDsPerNode() == 0 {
		errs = append(errs, "osdsPerNode must be set in at least one config")
	}
	// Do not care if the total number of OtherDaemonsPerNode is zero. OSDs run on both public and
	// cluster network, so OSDsPerNode can test all daemon types, but not vice-versa.
	for nodeType := range c.NodeTypes {
		mvErrs := metavalidation.IsDNS1123Subdomain(nodeType)
		if len(mvErrs) > 0 {
			errs = append(errs, fmt.Sprintf("nodeType identifier %q must meet RFC 1123 requirements: %v", nodeType, mvErrs))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("validation test config is invalid: %s", strings.Join(errs, ", "))
	}
	return nil
}

func NewSharedStorageAndWorkerNodesValidationTestConfig() *ValidationTestConfig {
	return NewDefaultValidationTestConfig()
}

const (
	DedicatedStorageNodeType = "storage-nodes"
	DedicatedWorkerNodeType  = "worker-nodes"
)

var dedicatedStorageNodeConfig = NodeConfig{
	OSDsPerNode:         DefaultValidationOSDsPerNode,
	OtherDaemonsPerNode: DefaultValidationOtherDaemonsPerNode,
	Placement: PlacementConfig{
		NodeSelector: map[string]string{
			DefaultStorageNodeLabelKey: DefaultStorageNodeLabelValue,
		},
		Tolerations: []TolerationType{
			{Key: DefaultStorageNodeLabelKey, Value: DefaultStorageNodeLabelValue},
		},
	},
}

var dedicatedWorkerNodeConfig = NodeConfig{
	OSDsPerNode:         0,
	OtherDaemonsPerNode: 6, // CSI plugins only
	// Placement empty
}

func NewDedicatedStorageNodesValidationTestConfig() *ValidationTestConfig {
	return &ValidationTestConfig{
		Namespace:       DefaultValidationNamespace,
		ResourceTimeout: DefaultValidationResourceTimeout,
		FlakyThreshold:  DefaultValidationFlakyThreshold,
		NginxImage:      DefaultValidationNginxImage,
		NodeTypes: map[string]NodeConfig{
			DedicatedStorageNodeType: dedicatedStorageNodeConfig,
			DedicatedWorkerNodeType:  dedicatedWorkerNodeConfig,
		},
	}
}

const (
	DedicatedArbiterNodeType = "arbiter-node"
)

func NewArbiterValidationTestConfig() *ValidationTestConfig {
	return &ValidationTestConfig{
		Namespace:       DefaultValidationNamespace,
		ResourceTimeout: DefaultValidationResourceTimeout,
		FlakyThreshold:  DefaultValidationFlakyThreshold,
		NginxImage:      DefaultValidationNginxImage,
		NodeTypes: map[string]NodeConfig{
			DedicatedStorageNodeType: dedicatedStorageNodeConfig,
			DedicatedWorkerNodeType:  dedicatedWorkerNodeConfig,
			DedicatedArbiterNodeType: {
				OSDsPerNode:         0,
				OtherDaemonsPerNode: 10, // 1 mon, plus all 9 CSI provisioners and plugins (optional)
				Placement: PlacementConfig{
					NodeSelector: map[string]string{
						DefaultArbiterNodeLabelKey: DefaultArbiterNodeLabelValue,
					},
					Tolerations: []TolerationType{
						{Key: DefaultArbiterTolerationKey, Operator: corev1.TolerationOpExists},
					},
				},
			},
		},
	}
}
