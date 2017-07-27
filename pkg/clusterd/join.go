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
package clusterd

import (
	"fmt"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/etcd/bootstrap"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/proc"
)

// StartJoinCluster initializes the cluster services to enable joining the cluster and listening for orchestration.
func StartJoinCluster(services []*ClusterService, configDir, nodeID, discoveryURL,
	etcdMembers string, networkInfo NetworkInfo, configFileOverride string, logLevel capnslog.LogLevel) (*Context, error) {

	logger.Infof("Starting cluster. configDir=%s, nodeID=%s, url=%s, members=%s, networkInfo=%+v, configFileOverride=%s, logLevel=%s",
		configDir, nodeID, discoveryURL, etcdMembers, networkInfo, configFileOverride, logLevel)

	if err := VerifyNetworkInfo(networkInfo); err != nil {
		return nil, fmt.Errorf("invalid network info: %+v. %+v", networkInfo, err)
	}

	etcdClients := []string{}
	if etcdMembers != "" {
		// use the etcd members provided by the caller
		etcdClients = strings.Split(etcdMembers, ",")
	} else {

		// use the discovery URL to query the etcd mgr what the etcd client endpoints should be
		var err error
		etcdClients, err = bootstrap.GetEtcdClients(configDir, discoveryURL, networkInfo.ClusterAddrIPv4, nodeID)
		if err != nil {
			return nil, err
		}
	}

	etcdClient, err := util.GetEtcdClient(etcdClients)
	if err != nil {
		return nil, err
	}

	// set the basic node config in etcd
	key := inventory.GetNodeConfigKey(nodeID)
	if err := util.CreateEtcdDir(etcdClient, key); err != nil {
		return nil, err
	}
	if err := inventory.SetIPAddress(etcdClient, nodeID, networkInfo.PublicAddrIPv4, networkInfo.ClusterAddrIPv4); err != nil {
		return nil, err
	}

	// initialize a leadership lease manager
	leaseManager, err := initLeaseManager(etcdClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize lease manager: %+v", err)
	}

	executor := &exec.CommandExecutor{}
	context := &Context{
		DirectContext: DirectContext{
			EtcdClient: etcdClient,
			NodeID:     nodeID,
			Inventory:  &inventory.Config{},
			Services:   services,
		},
		Executor:           executor,
		ProcMan:            proc.New(executor),
		ConfigDir:          configDir,
		LogLevel:           logLevel,
		ConfigFileOverride: configFileOverride,
		NetworkInfo:        networkInfo,
	}

	// initialize the device inventory
	context.Inventory.Local, err = inventory.DiscoverHardwareAndStore(context.EtcdClient, context.Executor, context.NodeID)
	if err != nil {
		return nil, fmt.Errorf("failed initial hardware discovery. %+v", err)
	}

	clusterLeader := newServicesLeader(context)
	clusterMember := newClusterMember(context, leaseManager, clusterLeader)
	clusterLeader.parent = clusterMember

	if err := clusterMember.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize local cluster: %+v", err)
	}

	// initialize the agents
	for _, service := range services {
		for _, agent := range service.Agents {
			if err := agent.Initialize(context); err != nil {
				return nil, fmt.Errorf("failed to initialize service %s. %v", service.Name, err)
			}
		}
	}

	go func() {
		// Watch for commands from the leader
		watchForAgentServiceConfig(context)
	}()

	return context, nil
}
