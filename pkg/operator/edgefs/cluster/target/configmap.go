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

package target

import (
	"encoding/json"

	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultServerIfName = "eth0"
	defaultBrokerIfName = "eth0"
)

type RtlfsDevice struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	CheckMountpoint int    `json:"check_mountpoint"`
}

type RtlfsDevices struct {
	Devices []RtlfsDevice `json:"devices"`
}

type CcowTenant struct {
	FailureDomain int `json:"failure_domain"`
}

type CcowNetwork struct {
	BrokerInterfaces string `json:"broker_interfaces"`
	ServerUnixSocket string `json:"server_unix_socket"`
	BrokerIP4addr    string `json:"broker_ip4addr,omitempty"`
	ServerIP4addr    string `json:"server_ip4addr,omitempty"`
}

type CcowConf struct {
	Tenant  CcowTenant  `json:"tenant"`
	Network CcowNetwork `json:"network"`
}

type CcowdNetwork struct {
	ServerInterfaces string `json:"server_interfaces"`
	ServerUnixSocket string `json:"server_unix_socket"`
	ServerIP4addr    string `json:"server_ip4addr,omitempty"`
}

type CcowdConf struct {
	Network   CcowdNetwork `json:"network"`
	Transport []string     `json:"transport"`
}

type AuditdConf struct {
	IsAggregator int `json:"is_aggregator"`
}

type SetupNode struct {
	Ccow            CcowConf     `json:"ccow"`
	Ccowd           CcowdConf    `json:"ccowd"`
	Auditd          AuditdConf   `json:"auditd"`
	Ipv4Autodetect  int          `json:"ipv4_autodetect,omitempty"`
	RtlfsAutodetect string       `json:"rtlfs_autodetect,omitempty"`
	ClusterNodes    []string     `json:"cluster_nodes,omitempty"`
	Rtrd            RTDevices    `json:"rtrd"`
	Rtlfs           RtlfsDevices `json:"rtlfs"`
	NodeType        string       `json:"nodeType"`
}

// As we relying on StatefulSet, we want to build global ConfigMap shared
// to all the nodes in the cluster. This way configuration is simplified and
// available to all subcomponents at any point it time.
func (c *Cluster) createSetupConfigs(resurrect bool) error {

	cm := make(map[string]SetupNode)

	dnsRecords := make([]string, len(c.Storage.Nodes))
	for i := 0; i < len(c.Storage.Nodes); i++ {
		dnsRecords[i] = createQualifiedHeadlessServiceName(i, c.Namespace)
	}

	serverIfName := defaultServerIfName
	brokerIfName := defaultBrokerIfName
	if isHostNetworkDefined(c.HostNetworkSpec) {

		if len(c.HostNetworkSpec.ServerIfName) > 0 && len(c.HostNetworkSpec.BrokerIfName) > 0 {
			serverIfName = c.HostNetworkSpec.ServerIfName
			brokerIfName = c.HostNetworkSpec.BrokerIfName
		} else if len(c.HostNetworkSpec.ServerIfName) > 0 {
			serverIfName = c.HostNetworkSpec.ServerIfName
			brokerIfName = c.HostNetworkSpec.ServerIfName
		} else if len(c.HostNetworkSpec.BrokerIfName) > 0 {
			serverIfName = c.HostNetworkSpec.BrokerIfName
			brokerIfName = c.HostNetworkSpec.BrokerIfName
		}
	}

	// fully resolve the storage config and resources for all nodes
	for _, node := range c.Storage.Nodes {
		devConfig := c.deploymentConfig.devConfig[node.Name]
		rtDevices := devConfig.rtrd.Devices
		rtlfsDevices := devConfig.rtlfs.Devices

		rtlfsAutoDetectPath := ""
		if c.deploymentConfig.deploymentType == deploymentAutoRtlfs &&
			!devConfig.isGatewayNode {
			rtlfsAutoDetectPath = "/data"
		}

		nodeType := "target"
		if devConfig.isGatewayNode {
			nodeType = "gateway"
		}

		if resurrect || devConfig.isGatewayNode {
			// In resurrection case we only need to adjust networking selections
			// in ccow.json, ccowd.json and corosync.conf. And keep device transport
			// same as before. Resurrection is "best effort" feature, we cannot
			// guarnatee that cluster can be reconfigured, but at least we do try.

			rtDevices = make([]RTDevice, 0)
			rtlfsDevices = make([]RtlfsDevice, 0)
		}

		nodeConfig := SetupNode{
			Ccow: CcowConf{
				Tenant: CcowTenant{
					FailureDomain: 1,
				},
				Network: CcowNetwork{
					BrokerInterfaces: brokerIfName,
					ServerUnixSocket: "/opt/nedge/var/run/sock/ccowd.sock",
				},
			},
			Ccowd: CcowdConf{
				Network: CcowdNetwork{
					ServerInterfaces: serverIfName,
					ServerUnixSocket: "/opt/nedge/var/run/sock/ccowd.sock",
				},
				Transport: []string{c.deploymentConfig.transportKey},
			},
			Auditd: AuditdConf{
				IsAggregator: 0,
			},
			Rtrd: RTDevices{
				Devices: rtDevices,
			},
			Rtlfs: RtlfsDevices{
				Devices: rtlfsDevices,
			},
			Ipv4Autodetect:  1,
			RtlfsAutodetect: rtlfsAutoDetectPath,
			ClusterNodes:    dnsRecords,
			NodeType:        nodeType,
		}

		cm[node.Name] = nodeConfig

		logger.Debugf("Resolved Node %s = %+v", node.Name, cm[node.Name])
	}

	nesetupJson, err := json.Marshal(&cm)
	if err != nil {
		return err
	}

	dataMap := make(map[string]string, 1)
	dataMap["nesetup"] = string(nesetupJson)

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configName,
			Namespace: c.Namespace,
		},
		Data: dataMap,
	}

	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &configMap.ObjectMeta, &c.ownerRef)
	if _, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(configMap); err != nil {
		if errors.IsAlreadyExists(err) {
			if _, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Update(configMap); err != nil {
				return nil
			}
		} else {
			return err
		}
	}

	// success. do the labeling so that StatefulSet scheduler will
	// select the right nodes.
	for _, node := range c.Storage.Nodes {
		k := c.Namespace
		err = c.AddLabelsToNode(c.context.Clientset, node.Name, map[string]string{k: "cluster"})
		logger.Debugf("added label %s from %s: %+v", k, node.Name, err)
	}

	return nil
}
