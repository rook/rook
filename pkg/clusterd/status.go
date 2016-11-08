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
	"errors"
	"fmt"
	"log"
	"path"
	"sync"
	"sync/atomic"

	etcd "github.com/coreos/etcd/client"
	"github.com/rook/rook/pkg/util"
	ctx "golang.org/x/net/context"
)

type NodeConfigStatus int

const (
	NodeStatusRootKey       = "/rook/_notify/%s"
	NodeStatusServiceKey    = "/rook/_notify/%s/%s" // node ID, service name
	InfiniteTimeout         = -1
	StatusValue             = "status"
	NodeConfigStatusUnknown = iota
	NodeConfigStatusNotTriggered
	NodeConfigStatusTriggered
	NodeConfigStatusRunning
	NodeConfigStatusFailed
	NodeConfigStatusSucceeded
	NodeConfigStatusTimeout
	NodeConfigStatusAbort
)

func (n NodeConfigStatus) String() string {
	if n == NodeConfigStatusNotTriggered {
		return ""
	}
	if n == NodeConfigStatusTriggered {
		return "triggered"
	}
	if n == NodeConfigStatusRunning {
		return "running"
	}
	if n == NodeConfigStatusFailed {
		return "failed"
	}
	if n == NodeConfigStatusSucceeded {
		return "succeeded"
	}
	if n == NodeConfigStatusTimeout {
		return "timeout"
	}
	if n == NodeConfigStatusAbort {
		return "abort"
	}

	return "unknown"
}

func ParseNodeConfigStatus(status string) NodeConfigStatus {
	if status == "" {
		return NodeConfigStatusNotTriggered
	}
	if status == "triggered" {
		return NodeConfigStatusTriggered
	}
	if status == "running" {
		return NodeConfigStatusRunning
	}
	if status == "failed" {
		return NodeConfigStatusFailed
	}
	if status == "succeeded" {
		return NodeConfigStatusSucceeded
	}
	if status == "timeout" {
		return NodeConfigStatusTimeout
	}
	if status == "abort" {
		return NodeConfigStatusAbort
	}

	return NodeConfigStatusUnknown
}

func WaitForNodeConfigCompletion(etcdClient etcd.KeysAPI, taskKey string, nodes []string, timeout int) (int, error) {
	if len(nodes) == 0 {
		return 0, nil
	}

	var waitgroup sync.WaitGroup
	waitgroup.Add(len(nodes))
	var nodesSuccessful int32

	// Start a go routine for each node that is expecting status updates for the configuration task
	for _, node := range nodes {
		go func(nodeID string) {
			defer waitgroup.Done()

			// Watch the status until it is failed or succeeded
			nodeStatus, statusIndex, _ := GetNodeConfigStatus(etcdClient, taskKey, nodeID)
			for {
				if nodeStatus == NodeConfigStatusSucceeded || nodeStatus == NodeConfigStatusFailed || nodeStatus == NodeConfigStatusTimeout {
					if nodeStatus == NodeConfigStatusSucceeded {
						atomic.AddInt32(&nodesSuccessful, 1)
					}

					log.Printf("Completed task %s with result %s on node %s", taskKey, nodeStatus.String(), nodeID)
					break
				}

				//util.DebugPrint("Watching for task %s status on node %s. Last=%s", taskKey, nodeID, nodeStatus.String())
				nodeStatus, _ = WatchNodeConfigStatus(etcdClient, taskKey, nodeID, timeout, &statusIndex)
			}
		}(node)
	}

	log.Printf("Waiting for %d nodes to complete task: %s", len(nodes), taskKey)
	waitgroup.Wait()

	log.Printf("%d/%d nodes successful for task: %s", nodesSuccessful, len(nodes), taskKey)
	if int(nodesSuccessful) < len(nodes) {
		return int(nodesSuccessful), errors.New("not all nodes succeeded configuration")
	}

	return int(nodesSuccessful), nil
}

// Get the general node status key, used for communication between the leader and the agents
func GetNodeProgressKey(nodeID string) string {
	return fmt.Sprintf(NodeStatusRootKey, nodeID)
}

// Get the status key for the general node or the specific service on the node.
func GetNodeStatusKey(service, nodeID string) string {
	if service == "" {
		return path.Join(GetNodeProgressKey(nodeID), StatusValue)
	}
	return path.Join(fmt.Sprintf(NodeStatusServiceKey, nodeID, service), StatusValue)
}

// SetNodeConfigStatus sets the node configuration status.
// If a taskKey is specified, set the status for a specific task.
// If the taskKey is the empty string, set the status for the node.
func SetNodeConfigStatus(etcdClient etcd.KeysAPI, nodeID, taskKey string, nodeStatus NodeConfigStatus) error {
	key := GetNodeStatusKey(taskKey, nodeID)
	_, err := etcdClient.Set(ctx.Background(), key, nodeStatus.String(), nil)
	return err
}

func SetNodeConfigStatusWithPrevIndex(etcdClient etcd.KeysAPI, nodeID string, nodeStatus NodeConfigStatus,
	prevIndex uint64) (*etcd.Response, error) {

	key := GetNodeStatusKey("", nodeID)
	resp, err := etcdClient.Set(ctx.Background(), key, nodeStatus.String(), &etcd.SetOptions{PrevIndex: prevIndex})
	return resp, err
}

func GetNodeConfigStatus(etcdClient etcd.KeysAPI, taskKey, nodeID string) (NodeConfigStatus, uint64, error) {
	key := GetNodeStatusKey(taskKey, nodeID)
	value, err := etcdClient.Get(ctx.Background(), key, nil)
	if err != nil {
		return NodeConfigStatusUnknown, 0, err
	}

	retVal := ParseNodeConfigStatus(value.Node.Value)
	if retVal == NodeConfigStatusUnknown {
		return NodeConfigStatusUnknown, value.Index, errors.New("failed to parse status: " + value.Node.Value)
	}

	return retVal, value.Index, nil
}

// WatchNodeConfigStatus watches for changes to the node config status etcd key
func WatchNodeConfigStatus(etcdClient etcd.KeysAPI, taskKey, nodeID string, timeout int, index *uint64) (NodeConfigStatus, error) {
	key := GetNodeStatusKey(taskKey, nodeID)
	value, timedOut, err := util.WatchEtcdKey(etcdClient, key, index, timeout)
	if err != nil {
		if timedOut {
			return NodeConfigStatusTimeout, nil
		}
		return NodeConfigStatusUnknown, err
	}

	return ParseNodeConfigStatus(value), nil
}
