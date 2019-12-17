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

// Package osd for the Ceph OSDs.
package osd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

const (
	OrchestrationStatusStarting      = "starting"
	OrchestrationStatusComputingDiff = "computingDiff"
	OrchestrationStatusOrchestrating = "orchestrating"
	OrchestrationStatusCompleted     = "completed"
	OrchestrationStatusFailed        = "failed"
	orchestrationStatusMapName       = "rook-ceph-osd-%s-status"
	orchestrationStatusKey           = "status"
	provisioningLabelKey             = "provisioning"
	nodeLabelKey                     = "node"
	completeProvisionTimeout         = 20
	completeProvisionSkipOSDTimeout  = 5
)

type provisionConfig struct {
	errorMessages []string
	DataPathMap   *config.DataPathMap // location to store data in container
}

func (c *Cluster) newProvisionConfig() *provisionConfig {
	return &provisionConfig{
		DataPathMap: config.NewDatalessDaemonDataPathMap(c.Namespace, c.dataDirHostPath),
	}
}

func (c *provisionConfig) addError(message string, args ...interface{}) {
	logger.Errorf(message, args...)
	c.errorMessages = append(c.errorMessages, fmt.Sprintf(message, args...))
}

func (c *Cluster) updateOSDStatus(node string, status OrchestrationStatus) error {
	return UpdateNodeStatus(c.kv, node, status)
}

func UpdateNodeStatus(kv *k8sutil.ConfigMapKVStore, node string, status OrchestrationStatus) error {
	labels := map[string]string{
		k8sutil.AppAttr:        AppName,
		orchestrationStatusKey: provisioningLabelKey,
		nodeLabelKey:           node,
	}

	// update the status map with the given status now
	s, _ := json.Marshal(status)
	if err := kv.SetValueWithLabels(
		k8sutil.TruncateNodeName(orchestrationStatusMapName, node),
		orchestrationStatusKey,
		string(s),
		labels,
	); err != nil {
		return errors.Wrapf(err, "failed to set node %s status", node)
	}
	return nil
}

func (c *Cluster) handleOrchestrationFailure(config *provisionConfig, nodeName, message string) {
	config.addError(message)
	status := OrchestrationStatus{Status: OrchestrationStatusFailed, Message: message}
	if err := c.updateOSDStatus(nodeName, status); err != nil {
		config.addError("failed to update status for node %q. %v", nodeName, err)
	}
}

func isStatusCompleted(status OrchestrationStatus) bool {
	return status.Status == OrchestrationStatusCompleted || status.Status == OrchestrationStatusFailed
}

func parseOrchestrationStatus(data map[string]string) *OrchestrationStatus {
	if data == nil {
		return nil
	}

	statusRaw, ok := data[orchestrationStatusKey]
	if !ok {
		return nil
	}

	// we have status for this node, unmarshal it
	var status OrchestrationStatus
	if err := json.Unmarshal([]byte(statusRaw), &status); err != nil {
		logger.Warningf("failed to unmarshal orchestration status. status: %s. %v", statusRaw, err)
		return nil
	}

	return &status
}

func (c *Cluster) completeProvision(config *provisionConfig) bool {
	return c.completeOSDsForAllNodes(config, true, completeProvisionTimeout)
}

func (c *Cluster) completeProvisionSkipOSDStart(config *provisionConfig) bool {
	return c.completeOSDsForAllNodes(config, false, completeProvisionSkipOSDTimeout)
}

func (c *Cluster) checkNodesCompleted(selector string, config *provisionConfig, configOSDs bool) (int, *util.Set, bool, *v1.ConfigMapList, error) {
	opts := metav1.ListOptions{
		LabelSelector: selector,
		Watch:         false,
	}
	remainingNodes := util.NewSet()
	// check the status map to see if the node is already completed before we start watching
	statuses, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).List(opts)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			config.addError("failed to get config status. %v", err)
			return 0, remainingNodes, false, statuses, err
		}
		// the status map doesn't exist yet, watching below is still an OK thing to do
	}

	originalNodes := len(statuses.Items)
	// check the nodes to see which ones are already completed
	for _, configMap := range statuses.Items {
		node, ok := configMap.Labels[nodeLabelKey]
		if !ok {
			logger.Warningf("missing node label on configmap %s", configMap.Name)
			continue
		}

		completed := c.handleStatusConfigMapStatus(node, config, &configMap, configOSDs)
		if !completed {
			remainingNodes.Add(node)
		}
	}
	if remainingNodes.Count() == 0 {
		return originalNodes, remainingNodes, true, statuses, nil
	}
	return originalNodes, remainingNodes, false, statuses, nil
}

func (c *Cluster) completeOSDsForAllNodes(config *provisionConfig, configOSDs bool, timeoutMinutes int) bool {
	selector := fmt.Sprintf("%s=%s,%s=%s",
		k8sutil.AppAttr, AppName,
		orchestrationStatusKey, provisioningLabelKey,
	)

	originalNodes, remainingNodes, completed, statuses, err := c.checkNodesCompleted(selector, config, configOSDs)
	if err == nil && completed {
		return true
	}

	currentTimeoutMinutes := 0
	for {
		opts := metav1.ListOptions{
			LabelSelector: selector,
			Watch:         true,
			// start watching for changes on the orchestration status map
			ResourceVersion: statuses.ResourceVersion,
		}
		logger.Infof("%d/%d node(s) completed osd provisioning, resource version %v", (originalNodes - remainingNodes.Count()), originalNodes, opts.ResourceVersion)

		w, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Watch(opts)
		if err != nil {
			logger.Warningf("failed to start watch on osd status, trying again. %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		defer w.Stop()

	ResultLoop:
		for {
			select {
			case e, ok := <-w.ResultChan():
				if !ok {
					logger.Infof("orchestration status config map result channel closed, will restart watch.")
					w.Stop()
					<-time.After(5 * time.Second)
					leftNodes := 0
					leftRemainingNodes := util.NewSet()
					leftNodes, leftRemainingNodes, completed, statuses, err = c.checkNodesCompleted(selector, config, configOSDs)
					if err == nil {
						if completed {
							logger.Infof("additional %d/%d node(s) completed osd provisioning", leftNodes, originalNodes)
							return true
						}
						remainingNodes = leftRemainingNodes
					} else {
						logger.Warningf("failed to list orchestration configmap, status: %v", err)
					}
					break ResultLoop
				}
				if e.Type == watch.Modified {
					configMap := e.Object.(*v1.ConfigMap)
					node, ok := configMap.Labels[nodeLabelKey]
					if !ok {
						logger.Infof("missing node label on configmap %s", configMap.Name)
						continue
					}
					if !remainingNodes.Contains(node) {
						logger.Infof("skipping event from node %s status update since it is already completed", node)
						continue
					}
					completed := c.handleStatusConfigMapStatus(node, config, configMap, configOSDs)
					if completed {
						remainingNodes.Remove(node)
						if remainingNodes.Count() == 0 {
							logger.Infof("%d/%d node(s) completed osd provisioning", originalNodes, originalNodes)
							return true
						}
					}
				}

			case <-time.After(time.Minute):
				// log every so often while we are waiting
				currentTimeoutMinutes++
				if currentTimeoutMinutes == timeoutMinutes {
					config.addError("timed out waiting for %d nodes: %+v", remainingNodes.Count(), remainingNodes)
					//start to remove remainingNodes waiting timeout.
					for remainingNode := range remainingNodes.Iter() {
						clearNodeName := k8sutil.TruncateNodeName(orchestrationStatusMapName, remainingNode)
						if err := c.kv.ClearStore(clearNodeName); err != nil {
							config.addError("failed to clear node %q status with name %q. %v", remainingNode, clearNodeName, err)
						}
					}
					return false
				}
				logger.Infof("waiting on orchestration status update from %d remaining nodes", remainingNodes.Count())
			}
		}
	}
}

func (c *Cluster) handleStatusConfigMapStatus(nodeName string, config *provisionConfig, configMap *v1.ConfigMap, configOSDs bool) bool {

	status := parseOrchestrationStatus(configMap.Data)
	if status == nil {
		return false
	}

	logger.Infof("osd orchestration status for node %s is %s", nodeName, status.Status)
	if status.Status == OrchestrationStatusCompleted {
		if configOSDs {
			if status.PvcBackedOSD {
				c.startOSDDaemonsOnPVC(nodeName, config, configMap, status)
			} else {
				c.startOSDDaemonsOnNode(nodeName, config, configMap, status)
			}
			// remove the status configmap that indicated the progress
			c.kv.ClearStore(fmt.Sprintf(orchestrationStatusMapName, nodeName))
		}

		return true
	}

	if status.Status == OrchestrationStatusFailed {
		config.addError("orchestration for node %s failed: %+v", nodeName, status)
		return true
	}
	return false
}
