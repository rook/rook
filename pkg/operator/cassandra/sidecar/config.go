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

package sidecar

import (
	"fmt"
	"github.com/ghodss/yaml"
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/cassandra/constants"
	"github.com/rook/rook/pkg/operator/cassandra/controller/util"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// Cassandra-Specific
	configDirCassandra            = "/etc/cassandra"
	cassandraYAMLPath             = configDirCassandra + "/" + "cassandra.yaml"
	cassandraEnvPath              = configDirCassandra + "/" + "cassandra-env.sh"
	cassandraRackDCPropertiesPath = configDirCassandra + "/" + "cassandra-rackdc.properties"

	// Scylla-Specific
	configDirScylla            = "/etc/scylla"
	scyllaYAMLPath             = configDirScylla + "/" + "scylla.yaml"
	scyllaRackDCPropertiesPath = configDirScylla + "/" + "cassandra-rackdc.properties"
	scyllaJMXPath              = "/usr/lib/scylla/jmx/scylla-jmx"

	// Common
	jolokiaPath            = constants.PluginDirName + "/" + "jolokia.jar"
	entrypointPath         = "/entrypoint.sh"
	rackDCPropertiesFormat = "dc=%s" + "\n" + "rack=%s" + "\n" + "prefer_local=false" + "\n"
)

// generateConfigFiles injects the default configuration files
// with our custom values.
func (m *MemberController) generateConfigFiles() error {

	var err error
	m.logger.Info("Generating config files")

	if m.mode == cassandrav1alpha1.ClusterModeScylla {
		err = m.generateScyllaConfigFiles()
	} else {
		err = m.generateCassandraConfigFiles()
	}

	return err
}

// generateCassandraConfigFiles generates the necessary config files for Cassandra.
// Currently, those are:
// - cassandra.yaml
// - cassandra-env.sh
// - cassandra-rackdc.properties
// - entrypoint-sh
func (m *MemberController) generateCassandraConfigFiles() error {

	/////////////////////////////
	// Generate cassandra.yaml //
	/////////////////////////////

	// Read default cassandra.yaml
	cassandraYAML, err := ioutil.ReadFile(cassandraYAMLPath)
	if err != nil {
		return fmt.Errorf("unexpected error trying to open cassandra.yaml: %s", err.Error())
	}

	customCassandraYAML, err := m.overrideConfigValues(cassandraYAML)
	if err != nil {
		return fmt.Errorf("error trying to override config values: %s", err.Error())
	}

	// Write result to file
	if err = ioutil.WriteFile(cassandraYAMLPath, customCassandraYAML, os.ModePerm); err != nil {
		m.logger.Errorf("error trying to write cassandra.yaml: %s", err.Error())
		return err
	}

	//////////////////////////////////////////
	// Generate cassandra-rackdc.properties //
	//////////////////////////////////////////

	rackdcProperties := []byte(fmt.Sprintf(rackDCPropertiesFormat, m.datacenter, m.rack))
	if err = ioutil.WriteFile(cassandraRackDCPropertiesPath, rackdcProperties, os.ModePerm); err != nil {
		return fmt.Errorf("error trying to write cassandra-rackdc.properties: %s", err.Error())
	}

	/////////////////////////////////////////
	//       Generate cassandra-env.sh     //
	/////////////////////////////////////////

	cassandraEnv, err := ioutil.ReadFile(cassandraEnvPath)
	if err != nil {
		return fmt.Errorf("error trying to open cassandra-env.sh, %s", err.Error())
	}

	// Calculate heap sizes
	// https://github.com/apache/cassandra/blob/521542ff26f9482b733e4f0f86281f07c3af29da/conf/cassandra-env.sh
	cpu := os.Getenv(constants.ResourceLimitCPUEnvVar)
	if cpu == "" {
		return fmt.Errorf("%s env variable not found", constants.ResourceLimitCPUEnvVar)
	}
	cpuNumber, _ := strconv.ParseInt(cpu, 10, 64)
	mem := os.Getenv(constants.ResourceLimitMemoryEnvVar)
	if mem == "" {
		return fmt.Errorf("%s env variable not found", constants.ResourceLimitMemoryEnvVar)
	}
	memNumber, _ := strconv.ParseInt(mem, 10, 64)
	maxHeapSize := util.Max(util.Min(memNumber/2, 1024), util.Min(memNumber/4, 8192))
	heapNewSize := util.Min(maxHeapSize/4, 100*cpuNumber)
	if err := os.Setenv("MAX_HEAP_SIZE", fmt.Sprintf("%dM", maxHeapSize)); err != nil {
		return fmt.Errorf("error setting MAX_HEAP_SIZE: %s", err.Error())
	}
	if err := os.Setenv("HEAP_NEWSIZE", fmt.Sprintf("%dM", heapNewSize)); err != nil {
		return fmt.Errorf("error setting HEAP_NEWSIZE: %s", err.Error())
	}

	// Add jolokia javaagent
	jolokiaConfig := []byte(fmt.Sprintf(`JVM_OPTS="$JVM_OPTS %s"`,
		getJolokiaConfig()))

	err = ioutil.WriteFile(cassandraEnvPath, append(cassandraEnv, jolokiaConfig...), os.ModePerm)
	if err != nil {
		return fmt.Errorf("error trying to write cassandra-env.sh: %s", err.Error())
	}

	////////////////////////////
	// Generate entrypoint.sh //
	////////////////////////////

	entrypoint := "#!/bin/sh" + "\n" + "exec cassandra -f -R"
	if err := ioutil.WriteFile(entrypointPath, []byte(entrypoint), os.ModePerm); err != nil {
		return fmt.Errorf("error trying to write cassandra entrypoint: %s", err.Error())
	}

	return nil

}

// generateScyllaConfigFiles generates the necessary config files for Scylla.
// Currently, those are:
// - scylla.yaml
// - cassandra-rackdc.properties
// - scylla-jmx
// - entrypoint.sh
func (m *MemberController) generateScyllaConfigFiles() error {

	// TODO: remove scylla.yaml gen once the entrypoint script in scylla gets
	// the necessary options

	/////////////////////////////
	// Generate scylla.yaml    //
	/////////////////////////////

	// Read default scylla.yaml
	scyllaYAML, err := ioutil.ReadFile(scyllaYAMLPath)
	if err != nil {
		return fmt.Errorf("unexpected error trying to open scylla.yaml: %s", err.Error())
	}

	customScyllaYAML, err := m.overrideConfigValues(scyllaYAML)
	if err != nil {
		return fmt.Errorf("error trying to override config values: %s", err.Error())
	}

	// Write result to file
	if err = ioutil.WriteFile(scyllaYAMLPath, customScyllaYAML, os.ModePerm); err != nil {
		m.logger.Errorf("error trying to write scylla.yaml: %s", err.Error())
		return err
	}

	//////////////////////////////////////////
	// Generate cassandra-rackdc.properties //
	//////////////////////////////////////////

	rackdcProperties := []byte(fmt.Sprintf(rackDCPropertiesFormat, m.datacenter, m.rack))
	if err := ioutil.WriteFile(scyllaRackDCPropertiesPath, rackdcProperties, os.ModePerm); err != nil {
		return fmt.Errorf("error trying to write cassandra-rackdc.properties: %s", err.Error())
	}

	/////////////////////////////////////////
	// Edit scylla-jmx with jolokia option //
	/////////////////////////////////////////

	scyllaJMXBytes, err := ioutil.ReadFile(scyllaJMXPath)
	if err != nil {
		return fmt.Errorf("error reading scylla-jmx: %s", err.Error())
	}
	scyllaJMX := string(scyllaJMXBytes)
	splitIndex := strings.Index(scyllaJMX, `\`) + len(`\`)
	m.logger.Infof("Split index = %d", splitIndex)
	injectedLine := fmt.Sprintf("\n    %s \\", getJolokiaConfig())
	scyllaJMXCustom := scyllaJMX[:splitIndex] + injectedLine + scyllaJMX[splitIndex:]
	if err := ioutil.WriteFile(scyllaJMXPath, []byte(scyllaJMXCustom), os.ModePerm); err != nil {
		return fmt.Errorf("error writing scylla-jmx: %s", err.Error())
	}

	////////////////////////////
	// Generate entrypoint.sh //
	////////////////////////////

	entrypoint, err := m.scyllaEntrypoint()
	if err != nil {
		return fmt.Errorf("error creating scylla entrypoint: %s", err.Error())
	}

	m.logger.Infof("Scylla entrypoint script:\n %s", entrypoint)
	if err := ioutil.WriteFile(entrypointPath, []byte(entrypoint), os.ModePerm); err != nil {
		return fmt.Errorf("error trying to write scylla entrypoint: %s", err.Error())
	}

	return nil
}

// scyllaEntrypoint returns the entrypoint script for scylla
func (m *MemberController) scyllaEntrypoint() (string, error) {

	// Get seeds
	seeds, err := m.getSeeds()
	if err != nil {
		return "", fmt.Errorf("error getting seeds: %s", err.Error())
	}

	// Get local ip
	localIP := os.Getenv(constants.PodIPEnvVar)
	if localIP == "" {
		return "", fmt.Errorf("POD_IP environment variable not set")
	}

	// See if we need to run in developer mode
	devMode := "0"
	c, err := m.rookClient.CassandraV1alpha1().Clusters(m.namespace).Get(m.cluster, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("error getting cluster: %s", err.Error())
	}
	if val, ok := c.Annotations[constants.DeveloperModeAnnotation]; ok && val == constants.LabelValueTrue {
		devMode = "1"
	}

	// Get cpu cores
	cpu := os.Getenv(constants.ResourceLimitCPUEnvVar)
	if cpu == "" {
		return "", fmt.Errorf("%s env variable not found", constants.ResourceLimitCPUEnvVar)
	}

	// Get memory
	mem := os.Getenv(constants.ResourceLimitMemoryEnvVar)
	if mem == "" {
		return "", fmt.Errorf("%s env variable not found", constants.ResourceLimitMemoryEnvVar)
	}
	// Leave some memory for other stuff
	memNumber, _ := strconv.ParseInt(mem, 10, 64)
	mem = fmt.Sprintf("%dM", util.Max(memNumber-700, 0))

	opts := []struct {
		flag, value string
	}{
		{
			flag:  "listen-address",
			value: localIP,
		},
		{
			flag:  "broadcast-address",
			value: m.ip,
		},
		{
			flag:  "broadcast-rpc-address",
			value: m.ip,
		},
		{
			flag:  "seeds",
			value: seeds,
		},
		{
			flag:  "developer-mode",
			value: devMode,
		},
		{
			flag:  "smp",
			value: cpu,
		},
		{
			flag:  "memory",
			value: mem,
		},
	}

	entrypoint := "#!/bin/sh" + "\n" + "exec /docker-entrypoint.py"
	for _, opt := range opts {
		entrypoint = fmt.Sprintf("%s --%s %s", entrypoint, opt.flag, opt.value)
	}
	return entrypoint, nil
}

// overrideConfigValues overrides the default config values with
// our custom values, for the fields that are of interest to us
func (m *MemberController) overrideConfigValues(configText []byte) ([]byte, error) {

	var config map[string]interface{}

	if err := yaml.Unmarshal(configText, &config); err != nil {
		return nil, fmt.Errorf("error unmarshalling cassandra.yaml: %s", err.Error())
	}

	seeds, err := m.getSeeds()
	if err != nil {
		return nil, fmt.Errorf("error getting seeds: %s", err.Error())
	}

	localIP := os.Getenv(constants.PodIPEnvVar)
	if localIP == "" {
		return nil, fmt.Errorf("POD_IP environment variable not set")
	}

	seedProvider := []map[string]interface{}{
		{
			"class_name": "org.apache.cassandra.locator.SimpleSeedProvider",
			"parameters": []map[string]interface{}{
				{
					"seeds": seeds,
				},
			},
		},
	}

	config["cluster_name"] = m.cluster
	config["listen_address"] = localIP
	config["broadcast_address"] = m.ip
	config["rpc_address"] = "0.0.0.0"
	config["broadcast_rpc_address"] = m.ip
	config["endpoint_snitch"] = "GossipingPropertyFileSnitch"
	config["seed_provider"] = seedProvider

	return yaml.Marshal(config)
}

// getSeeds gets the IPs of the instances acting as Seeds
// in the Cluster. It does that by getting all ClusterIP services
// of the current Cluster with the cassandra.rook.io/seed label
func (m *MemberController) getSeeds() (string, error) {

	var services *corev1.ServiceList
	var err error

	m.logger.Infof("Attempting to find seeds.")
	sel := fmt.Sprintf("%s,%s=%s", constants.SeedLabel, constants.ClusterNameLabel, m.cluster)

	for {

		services, err = m.kubeClient.CoreV1().Services(m.namespace).List(metav1.ListOptions{LabelSelector: sel})
		if err != nil {
			return "", err
		}
		if len(services.Items) > 0 {
			break
		}
		time.Sleep(1000 * time.Millisecond)
	}

	seeds := []string{}
	for _, svc := range services.Items {
		seeds = append(seeds, svc.Spec.ClusterIP)
	}
	return strings.Join(seeds, ","), nil
}

func getJolokiaConfig() string {

	opts := []struct {
		flag, value string
	}{
		{
			flag:  "host",
			value: "localhost",
		},
		{
			flag:  "port",
			value: fmt.Sprintf("%d", constants.JolokiaPort),
		},
		{
			flag:  "executor",
			value: "fixed",
		},
		{
			flag:  "threadNr",
			value: "2",
		},
	}

	cmd := []string{}
	for _, opt := range opts {
		cmd = append(cmd, fmt.Sprintf("%s=%s", opt.flag, opt.value))
	}
	return fmt.Sprintf("-javaagent:%s=%s", jolokiaPath, strings.Join(cmd, ","))
}

// Merge YAMLs merges two arbitrary YAML structures on the top level.
func mergeYAMLs(initialYAML, overrideYAML []byte) ([]byte, error) {

	var initial, override map[string]interface{}
	yaml.Unmarshal(initialYAML, &initial)
	yaml.Unmarshal(overrideYAML, &override)

	if initial == nil {
		initial = make(map[string]interface{})
	}
	// Overwrite the values onto initial
	for k, v := range override {
		initial[k] = v
	}
	return yaml.Marshal(initial)

}
