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
	"time"

	etcd "github.com/coreos/etcd/client"
	"github.com/rook/rook/pkg/util"
	ctx "golang.org/x/net/context"
)

// Listen for notifications from the orchestrator that components should be installed on the agent
func watchForAgentServiceConfig(context *Context) {
	restoreDesiredState(context)

	// Get the initial status of the configuration
	configStatus, configStatusIndex, err := GetNodeConfigStatus(context.EtcdClient, "", context.NodeID)

	// Stay in a loop looking for status updates to the orchestrator that is sending
	// instructions to install and configure cluster components on this agent.
	// This loop should never exit until the agent is stopped.
	for {
		if configStatus == NodeConfigStatusTriggered {
			// This node has been triggered, stay in a loop installing services as long as there is an instruction that
			// a service should be installed
			checkAgentStatuses := true
			for checkAgentStatuses {
				checkAgentStatuses = false
				for _, service := range context.Services {
					for _, agent := range service.Agents {
						if ConfigureServiceIfTriggered(context, agent) {
							checkAgentStatuses = true
						}
					}
				}

				// Attempt to clear the key that indicates an agent on this node has been triggered (node level status key),
				// specifying that the previous index of the key should be unchanged from its index we saw last. If the index
				// has changed, then someone has tried to trigger more agents, so instead of clearing the key out, check agent
				// statuses again.
				clearNodeConfigStatus(context.EtcdClient, context.NodeID, &configStatusIndex, &checkAgentStatuses, 5)
			}
		}

		// Watch the status key to trigger component install.
		configStatus, err = WatchNodeConfigStatus(context.EtcdClient, "", context.NodeID, InfiniteTimeout, &configStatusIndex)
		if err != nil {
			if util.IsEtcdKeyReset(err) {
				// Reset the watch index
				logger.Infof("Config status key was reset. Refreshing the index. err=%v", err)
				configStatus, configStatusIndex, _ = GetNodeConfigStatus(context.EtcdClient, "", context.NodeID)
			} else {
				// Unknown error
				logger.Warningf("Failed to watch orchestration state. Sleeping 5s. err=%s", err.Error())
				<-time.After(5 * time.Second)
			}
			continue
		} else if configStatus == NodeConfigStatusAbort {
			break
		}
	}
}

// At startup, make a best effort to start up the services that were started in a previous orchestration.
func restoreDesiredState(context *Context) {
	for _, service := range context.Services {
		for _, agent := range service.Agents {
			err := agent.ConfigureLocalService(context)
			if err != nil {
				logger.Warningf("Failed to restore agent %s. %v", agent.Name(), err)
			}
		}
	}
}

func ConfigureServiceIfTriggered(context *Context, agent ServiceAgent) bool {
	state, _, err := GetNodeConfigStatus(context.EtcdClient, agent.Name(), context.NodeID)
	// Start the install action if it was just triggered or if it had already been running and now has restarted
	if err == nil && (state == NodeConfigStatusTriggered || state == NodeConfigStatusRunning) {
		RunConfigureAgent(context, agent)
		return true
	}

	return false
}

func RunConfigureAgent(context *Context, agent ServiceAgent) error {
	SetNodeConfigStatus(context.EtcdClient, context.NodeID, agent.Name(), NodeConfigStatusRunning)

	err := agent.ConfigureLocalService(context)
	if err != nil {
		logger.Errorf("AGENT: Service %s failed configuration: %v", agent.Name(), err)
		SetNodeConfigStatus(context.EtcdClient, context.NodeID, agent.Name(), NodeConfigStatusFailed)
		return err
	}

	logger.Infof("AGENT: Service %s succeeded the configuration", agent.Name())
	SetNodeConfigStatus(context.EtcdClient, context.NodeID, agent.Name(), NodeConfigStatusSucceeded)

	return nil
}

func clearNodeConfigStatus(etcdClient etcd.KeysAPI, nodeID string, configStatusIndex *uint64, checkAgentStatuses *bool, sleepSecs time.Duration) {
	resp, err := SetNodeConfigStatusWithPrevIndex(etcdClient, nodeID, NodeConfigStatusNotTriggered, *configStatusIndex)
	if err == nil {
		*configStatusIndex = resp.Index
	} else {
		if util.IsEtcdCompareFailed(err) {
			// FIX: Get rid of this hack. We need to use the prevIndex.
			err = SetNodeConfigStatus(etcdClient, nodeID, "", NodeConfigStatusNotTriggered)

			// the key has been updated from the last time we saw it, it's not safe to clear it out and move on.
			// Instead, refresh	our copy of the key's current index and check agent statuses again
			*checkAgentStatuses = true
			_, newIndex, err := GetNodeConfigStatus(etcdClient, "", nodeID)
			if err == nil {
				*configStatusIndex = newIndex
			}
		}
	}
}

// Watch an etcd key for changes since the provided index
func watchEtcdKey(etcdClient etcd.KeysAPI, key string, index *uint64, timeout int) (string, error) {
	options := &etcd.WatcherOptions{AfterIndex: *index}
	watcher := etcdClient.Watcher(key, options)
	cancelableContext, cancelFunc := ctx.WithCancel(ctx.Background())

	value := ""
	var err error = nil
	watcherChannel := make(chan bool, 1)
	go func() {
		logger.Tracef("waiting for response")
		var response *etcd.Response
		response, err = watcher.Next(cancelableContext)
		if err != nil {
			if err != ctx.Canceled {
				// If there was an error watching, attempt to get the value of the key and reset the current index
				// This can be a common occurrence for the index to get out of date. See documentation on the error
				// "The event in requested index is outdated and cleared"
				response, geterr := etcdClient.Get(ctx.Background(), key, nil)
				if geterr == nil {
					logger.Infof("Watching %s failed on index %d, but Get succeeded with index %d", key, *index, response.Index)
					*index = response.Index
					value = response.Node.Value
					err = nil
				}
			}
		} else {
			logger.Tracef("Watched key %s, value=%s, index=%d", key, response.Node.Value, *index)
			*index = response.Index
			value = response.Node.Value
		}
		watcherChannel <- true
	}()

	if timeout == InfiniteTimeout {
		// Wait indefinitely for the etcd watcher to respond
		logger.Tracef("Watching key %s after index %d", key, *index)
		<-watcherChannel
		return value, err

	} else {
		// Start a timer to allow a timeout if the watch doesn't return in a timely manner
		timer := time.NewTimer(time.Second * time.Duration(timeout))

		// Return when the first channel completes
		logger.Tracef("Watching key %s after index %d for at most %d seconds", key, *index, timeout)
		select {
		case <-timer.C:
			logger.Errorf("Timed out watching key %s", key)
			cancelFunc()
			return "", errors.New("the etcd watch timed out")
		case <-watcherChannel:
			logger.Debugf("Completed watching key %s. value=%s", key, value)
			timer.Stop()
			return value, err
		}
	}
}
