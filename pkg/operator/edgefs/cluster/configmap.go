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

	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultServerIfName            = "eth0"
	defaultBrokerIfName            = "eth0"
	defaultTrlogProcessingInterval = 10
	defaultTrlogKeepDays           = 3
)

// As we relying on StatefulSet, we want to build global ConfigMap shared
// to all the nodes in the cluster. This way configuration is simplified and
// available to all subcomponents at any point it time.
func (c *cluster) createClusterConfigMap(deploymentConfig edgefsv1.ClusterDeploymentConfig, resurrect bool) error {
	var err error
	cm := make(map[string]edgefsv1.SetupNode)

	dnsRecords := make([]string, len(deploymentConfig.DevConfig))
	for i := 0; i < len(deploymentConfig.DevConfig); i++ {
		dnsRecords[i] = target.CreateQualifiedHeadlessServiceName(i, c.Namespace)
	}
	serverIfName := defaultServerIfName
	brokerIfName := defaultBrokerIfName

	serverSelector, serverDefined := c.Spec.Network.Selectors["server"]
	brokerSelector, brokerDefined := c.Spec.Network.Selectors["broker"]

	if c.Spec.Network.IsHost() {
		if serverDefined && brokerDefined {
			serverIfName = serverSelector
			brokerIfName = brokerSelector
		} else if serverDefined {
			serverIfName = serverSelector
			brokerIfName = serverSelector
		} else if brokerDefined {
			serverIfName = brokerSelector
			brokerIfName = brokerSelector
		}
	} else if c.Spec.Network.IsMultus() {
		if serverDefined && brokerDefined {
			serverIfName, err = k8sutil.GetMultusIfName(serverSelector)
			if err != nil {
				return err
			}

			brokerIfName, err = k8sutil.GetMultusIfName(brokerSelector)
			if err != nil {
				return err
			}
		} else if serverDefined {
			serverIfName, err = k8sutil.GetMultusIfName(serverSelector)
			if err != nil {
				return err
			}

			brokerIfName = serverIfName
		} else if brokerDefined {
			serverIfName, err = k8sutil.GetMultusIfName(brokerSelector)
			if err != nil {
				return err
			}

			brokerIfName = serverIfName
		}
	}

	// Fully resolve the storage config and resources for all nodes
	for nodeName := range deploymentConfig.DevConfig {
		devConfig := deploymentConfig.DevConfig[nodeName]
		rtDevices := devConfig.Rtrd.Devices
		rtSlaveDevices := devConfig.RtrdSlaves
		rtlfsDevices := devConfig.Rtlfs.Devices

		rtlfsAutoDetectPath := ""
		if deploymentConfig.DeploymentType == edgefsv1.DeploymentAutoRtlfs &&
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

			rtDevices = make([]edgefsv1.RTDevice, 0)
			rtSlaveDevices = make([]edgefsv1.RTDevices, 0)
			rtlfsDevices = make([]edgefsv1.RtlfsDevice, 0)
		}
		// Set failureDomain to 2 if current node's zone > 0
		failureDomain := 1
		if devConfig.Zone > 0 {
			failureDomain = 2
		}
		if len(c.Spec.FailureDomain) > 0 {
			switch c.Spec.FailureDomain {
			case "device":
				failureDomain = 0
			case "host":
				failureDomain = 1
			case "zone":
				failureDomain = 2
			default:
				logger.Infof("Unknow failure domain %s, skipped", c.Spec.FailureDomain)
			}
		}
		commitWait := 1
		if c.Spec.CommitNWait > 0 {
			commitWait = 0
		}
		noIpFrag := 0
		if c.Spec.NoIP4Frag {
			noIpFrag = 1
		}
		nodeConfig := edgefsv1.SetupNode{
			Ccow: edgefsv1.CcowConf{
				Trlog: edgefsv1.CcowTrlog{
					Interval: defaultTrlogProcessingInterval,
				},
				Tenant: edgefsv1.CcowTenant{
					FailureDomain: failureDomain,
					CommitWait:    commitWait,
				},
				Network: edgefsv1.CcowNetwork{
					BrokerInterfaces: brokerIfName,
					ServerUnixSocket: "/opt/nedge/var/run/sock/ccowd.sock",
					NoIP4Frag:        noIpFrag,
				},
			},
			Ccowd: edgefsv1.CcowdConf{
				BgConfig: edgefsv1.CcowdBgConfig{
					TrlogDeleteAfterHours:     defaultTrlogKeepDays * 24,
					SpeculativeBackrefTimeout: defaultTrlogKeepDays * 24 * 3600 * 1000,
				},
				Zone: devConfig.Zone,
				Network: edgefsv1.CcowdNetwork{
					ServerInterfaces: serverIfName,
					ServerUnixSocket: "/opt/nedge/var/run/sock/ccowd.sock",
					NoIP4Frag:        noIpFrag,
				},
				Transport: []string{deploymentConfig.TransportKey},
			},
			Auditd: edgefsv1.AuditdConf{
				IsAggregator: 0,
			},
			Rtrd: edgefsv1.RTDevices{
				Devices: rtDevices,
			},
			RtrdSlaves: rtSlaveDevices,
			Rtlfs: edgefsv1.RtlfsDevices{
				Devices: rtlfsDevices,
			},
			Rtkvs:           devConfig.Rtkvs,
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
			nodeConfig.Ccowd.BgConfig.SpeculativeBackrefTimeout = c.Spec.TrlogKeepDays * 24 * 3600 * 1000
		}

		if c.Spec.SystemReplicationCount > 0 {
			nodeConfig.Ccow.Tenant.ReplicationCount = c.Spec.SystemReplicationCount
			nodeConfig.Ccow.Tenant.SyncPut = c.Spec.SystemReplicationCount
			nodeConfig.Ccow.Tenant.SyncPutNamed = c.Spec.SystemReplicationCount
		}

		if c.Spec.SysChunkSize > 0 {
			nodeConfig.Ccow.Tenant.ChunkSize = c.Spec.SysChunkSize
		}

		cm[nodeName] = nodeConfig

		logger.Debugf("Resolved Node %s = %+v", nodeName, cm[nodeName])
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

	return nil
}
