package clusterd

import (
	"fmt"
	"log"
	"strings"

	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/etcdmgr/manager"
	"github.com/quantum/castle/pkg/util"
	"github.com/quantum/castle/pkg/util/exec"
	"github.com/quantum/castle/pkg/util/proc"
)

// StartJoinCluster initializes the cluster services to enable joining the cluster and listening for orchestration.
func StartJoinCluster(services []*ClusterService, procMan *proc.ProcManager, nodeID, discoveryURL, etcdMembers, publicIPv4, privateIPv4 string) (*Context, error) {
	log.Printf("Starting cluster. nodeID=%s, url=%s, members=%s, publicIPv4=%s, privateIPv4=%s",
		nodeID, discoveryURL, etcdMembers, publicIPv4, privateIPv4)

	etcdClients := []string{}
	if etcdMembers != "" {
		// use the etcd members provided by the caller
		etcdClients = strings.Split(etcdMembers, ",")
	} else {

		// use the discovery URL to query the etcdmgr what the etcd client endpoints should be
		var err error
		etcdClients, err = manager.GetEtcdClients(discoveryURL, privateIPv4, nodeID)
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
	if err := inventory.SetIPAddress(etcdClient, nodeID, privateIPv4); err != nil {
		return nil, err
	}

	// initialize a leadership lease manager
	leaseManager, err := initLeaseManager(etcdClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize lease manager: %+v", err)
	}

	context := &Context{
		EtcdClient: etcdClient,
		Executor:   &exec.CommandExecutor{},
		NodeID:     nodeID,
		Services:   services,
		ProcMan:    procMan,
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
