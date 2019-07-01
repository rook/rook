/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package cluster

import (
	"encoding/json"

	edgefsv1beta1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1beta1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultServerIfName            = "eth0"
	defaultBrokerIfName            = "eth0"
	defaultTrlogProcessingInterval = 10
	defaultTrlogKeepDays           = 7
)

// As we relying on StatefulSet, we want to build global ConfigMap shared
// to all the nodes in the cluster. This way configuration is simplified and
// available to all subcomponents at any point it time.
func (c *cluster) createClusterConfigMap(nodes []rookalpha.Node, deploymentConfig edgefsv1beta1.ClusterDeploymentConfig, resurrect bool) error {

	cm := make(map[string]edgefsv1beta1.SetupNode)

	dnsRecords := make([]string, len(nodes))
	for i := 0; i < len(nodes); i++ {
		dnsRecords[i] = target.CreateQualifiedHeadlessServiceName(i, c.Namespace)
	}

	serverIfName := defaultServerIfName
	brokerIfName := defaultBrokerIfName
	if isHostNetworkDefined(c.Spec.Network) {

		if len(c.Spec.Network.ServerIfName) > 0 && len(c.Spec.Network.BrokerIfName) > 0 {
			serverIfName = c.Spec.Network.ServerIfName
			brokerIfName = c.Spec.Network.BrokerIfName
		} else if len(c.Spec.Network.ServerIfName) > 0 {
			serverIfName = c.Spec.Network.ServerIfName
			brokerIfName = c.Spec.Network.ServerIfName
		} else if len(c.Spec.Network.BrokerIfName) > 0 {
			serverIfName = c.Spec.Network.BrokerIfName
			brokerIfName = c.Spec.Network.BrokerIfName
		}
	}

	// Fully resolve the storage config and resources for all nodes
	for _, node := range nodes {
		devConfig := deploymentConfig.DevConfig[node.Name]
		rtDevices := devConfig.Rtrd.Devices
		rtSlaveDevices := devConfig.RtrdSlaves
		rtlfsDevices := devConfig.Rtlfs.Devices

		rtlfsAutoDetectPath := ""
		if deploymentConfig.DeploymentType == edgefsv1beta1.DeploymentAutoRtlfs &&
			!devConfig.IsGatewayNode {
			rtlfsAutoDetectPath = "/data"
		}

		nodeType := "target"
		if devConfig.IsGatewayNode {
			nodeType = "gateway"
		}

		if resurrect || devConfig.IsGatewayNode {
			// In resurrection case we only need to adjust networking selections
			// in ccow.json, ccowd.json and corosync.conf. And keep device transport
			// same as before. Resurrection is "best effort" feature, we cannot
			// guarnatee that cluster can be reconfigured, but at least we do try.

			rtDevices = make([]edgefsv1beta1.RTDevice, 0)
			rtSlaveDevices = make([]edgefsv1beta1.RTDevices, 0)
			rtlfsDevices = make([]edgefsv1beta1.RtlfsDevice, 0)
		}
		// Set failureDomain to 2 if current node's zone > 0
		failureDomain := 1
		if devConfig.Zone > 0 {
			failureDomain = 2
		}

		nodeConfig := edgefsv1beta1.SetupNode{
			Ccow: edgefsv1beta1.CcowConf{
				Trlog: edgefsv1beta1.CcowTrlog{
					Interval: defaultTrlogProcessingInterval,
				},
				Tenant: edgefsv1beta1.CcowTenant{
					FailureDomain: failureDomain,
				},
				Network: edgefsv1beta1.CcowNetwork{
					BrokerInterfaces: brokerIfName,
					ServerUnixSocket: "/opt/nedge/var/run/sock/ccowd.sock",
				},
			},
			Ccowd: edgefsv1beta1.CcowdConf{
				BgConfig: edgefsv1beta1.CcowdBgConfig{
					TrlogDeleteAfterHours: defaultTrlogKeepDays * 24,
				},
				Zone: devConfig.Zone,
				Network: edgefsv1beta1.CcowdNetwork{
					ServerInterfaces: serverIfName,
					ServerUnixSocket: "/opt/nedge/var/run/sock/ccowd.sock",
				},
				Transport: []string{deploymentConfig.TransportKey},
			},
			Auditd: edgefsv1beta1.AuditdConf{
				IsAggregator: 0,
			},
			Rtrd: edgefsv1beta1.RTDevices{
				Devices: rtDevices,
			},
			RtrdSlaves: rtSlaveDevices,
			Rtlfs: edgefsv1beta1.RtlfsDevices{
				Devices: rtlfsDevices,
			},
			Ipv4Autodetect:  1,
			RtlfsAutodetect: rtlfsAutoDetectPath,
			ClusterNodes:    dnsRecords,
			NodeType:        nodeType,
		}

		if c.Spec.TrlogProcessingInterval > 0 {
			nodeConfig.Ccow.Trlog.Interval = c.Spec.TrlogProcessingInterval
		}

		if c.Spec.TrlogKeepDays > 0 {
			nodeConfig.Ccowd.BgConfig.TrlogDeleteAfterHours = c.Spec.TrlogKeepDays * 24
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

	k8sutil.SetOwnerRef(&configMap.ObjectMeta, &c.ownerRef)
	if _, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(configMap); err != nil {
		if errors.IsAlreadyExists(err) {
			if _, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Update(configMap); err != nil {
				return nil
			}
		} else {
			return err
		}
	}

	// Success. Do the labeling so that StatefulSet scheduler will
	// select the right nodes.
	for _, node := range nodes {
		k := c.Namespace
		err = c.AddLabelsToNode(c.context.Clientset, node.Name, map[string]string{k: "cluster"})
		logger.Debugf("added label %s from %s: %+v", k, node.Name, err)
	}

	return nil
}
