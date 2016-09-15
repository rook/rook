package clusterd

import (
	"log"
	"path"
	"strings"

	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/etcdmgr/manager"
	"github.com/quantum/castle/pkg/proc"
	"github.com/quantum/castle/pkg/util"
)

// Initialize the cluster services to enable joining the cluster and listening for orchestration.
func StartJoinCluster(services []*ClusterService, procMan *proc.ProcManager, discoveryURL, etcdMembers, privateIPv4 string) error {
	etcdClients := []string{}
	if etcdMembers != "" {
		// use the etcd members provided by the caller
		etcdClients = strings.Split(etcdMembers, ",")
	} else {

		// use the discovery URL to query the etcdmgr what the etcd client endpoints should be
		var err error
		etcdClients, err = manager.GetEtcdClients(discoveryURL, privateIPv4)
		if err != nil {
			return err
		}
	}

	etcdClient, err := util.GetEtcdClient(etcdClients)
	if err != nil {
		return err
	}

	nodeID, err := GetMachineID()
	if err != nil {
		return err
	}

	// set the basic node config in etcd
	key := path.Join(inventory.DiscoveredNodesKey, nodeID)
	if err := util.CreateEtcdDir(etcdClient, key); err != nil {
		return err
	}
	if err := inventory.SetIpAddress(etcdClient, nodeID, privateIPv4); err != nil {
		return err
	}

	// initialize a leadership lease manager
	leaseManager, err := initLeaseManager(etcdClient)
	if err != nil {
		log.Fatalf("failed to initialize lease manager: %s", err.Error())
		return err
	}

	context := &Context{
		EtcdClient: etcdClient,
		Executor:   &proc.CommandExecutor{},
		NodeID:     nodeID,
		Services:   services,
		ProcMan:    procMan,
	}
	clusterLeader := &servicesLeader{context: context, leaseName: LeaderElectionKey}
	clusterMember := newClusterMember(context, leaseManager, clusterLeader)
	clusterLeader.parent = clusterMember

	err = clusterMember.initialize()
	if err != nil {
		log.Fatalf("failed to initialize local cluster: %v", err)
		return err
	}

	go func() {
		// Watch for commands from the leader
		watchForAgentServiceConfig(context)
	}()

	return nil
}
