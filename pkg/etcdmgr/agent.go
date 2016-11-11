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
package etcdmgr

import (
	"fmt"
	"log"
	"path"

	etcd "github.com/coreos/etcd/client"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/etcdmgr/bootstrap"
	"github.com/rook/rook/pkg/util"
	ctx "golang.org/x/net/context"
)

const (
	etcdMgrAgentName = "etcd"
)

type etcdMgrAgent struct {
	embeddedEtcd *bootstrap.EmbeddedEtcd
	conf         *bootstrap.Config
	context      bootstrap.EtcdMgrContext
	etcdFactory  bootstrap.EtcdFactory
}

func (e *etcdMgrAgent) Name() string {
	return etcdMgrAgentName
}

func (e *etcdMgrAgent) Initialize(context *clusterd.Context) error {
	return nil
}

func (e *etcdMgrAgent) ConfigureLocalService(context *clusterd.Context) error {
	log.Printf("inside ConfigureLocalService")
	// check if the etcdmgr is in the desired state for this node
	desiredKey := path.Join(etcdmgrKey, clusterd.DesiredKey, context.NodeID)
	etcdmgrDesired, err := util.EtcdDirExists(context.EtcdClient, desiredKey)
	if err != nil {
		return fmt.Errorf("error in checking existence of desired key: %+v", err)
	}
	appliedKey := getEtcdMgrAppliedKey(context.NodeID)
	etcdmgrApplied, err := util.EtcdDirExists(context.EtcdClient, appliedKey)
	if err != nil {
		return fmt.Errorf("error in checking existence of applied key: %+v", err)
	}

	// Add or remote embedded etcd instance as needed
	if etcdmgrDesired {
		e.CreateLocalService(context, desiredKey, etcdmgrApplied)

	} else if !etcdmgrDesired && etcdmgrApplied {
		err := e.DestroyLocalService(context)
		if err != nil {
			return fmt.Errorf("error in removing node: %+v", err)
		}

	}

	return nil
}

// get ip address for the target agent (will be used to bootstrap embedded etcd)
func (e *etcdMgrAgent) CreateLocalService(context *clusterd.Context, desiredKey string, etcdmgrApplied bool) error {
	ipAddrKey := path.Join(desiredKey, "ipaddress")
	resp, err := context.EtcdClient.Get(ctx.Background(), ipAddrKey, nil)
	if err != nil {
		return fmt.Errorf("error in getting the ip address key: %+v. err: %+v", ipAddrKey, err)
	}
	ipAddr := resp.Node.Value
	log.Println("ipAddress: ", ipAddr)
	e.conf, err = bootstrap.GenerateConfigFromExistingCluster(e.context, context.ConfigDir, ipAddr, context.NodeID)
	log.Println("config: ", e.conf)
	if err != nil {
		return err
	}

	if !etcdmgrApplied {
		log.Println("adding the current node to the etcd cluster...")
		targetEndpoint := getPeerEndpointFromIP(ipAddr)
		err = AddMember(e.context, targetEndpoint)
		if err != nil {
			return fmt.Errorf("error in adding a member to the cluster")
		}

		ipKey := path.Join(etcdmgrKey, clusterd.AppliedKey, context.NodeID, "ipaddress")
		log.Println("ipKey for new instance: ", ipKey)
		_, err = context.EtcdClient.Set(ctx.Background(), ipKey, ipAddr, nil)
		if err != nil {
			return fmt.Errorf("error in setting applied key for ip key. %+v", err)
		}
	}
	e.embeddedEtcd, err = e.etcdFactory.NewEmbeddedEtcd("", e.conf, false)
	if err != nil {
		return fmt.Errorf("error in creating a new instance of embedded etcd. err: %+v: ", err)
	}

	return nil
}

func (e *etcdMgrAgent) DestroyLocalService(context *clusterd.Context) error {
	fmt.Println("destroying the local embedded etcd instance")
	err := e.embeddedEtcd.Destroy(e.conf)
	e.embeddedEtcd = nil
	if err != nil {
		return fmt.Errorf("error in destroying local embedded etcd. err: %+v", err)
	}
	// successful, remove the current node from desired state
	appliedKey := getEtcdMgrAppliedKey(context.NodeID)
	_, err = context.EtcdClient.Delete(ctx.Background(), appliedKey, &etcd.DeleteOptions{Dir: true, Recursive: true})
	if err != nil {
		return fmt.Errorf("error in removing etcdmgr applied key. err: %+v", err)
	}
	return nil
}

func getEtcdMgrAppliedKey(nodeID string) string {
	return path.Join(etcdmgrKey, clusterd.AppliedKey, nodeID)
}

func getPeerEndpointFromIP(ip string) string {
	return fmt.Sprintf("http://%s:%d", ip, bootstrap.DefaultPeerPort)
}
