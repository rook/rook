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
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

const (
	// OrchestrationStatusStarting denotes the OSD provisioning is beginning.
	OrchestrationStatusStarting = "starting"
	// OrchestrationStatusOrchestrating denotes the OSD provisioning has begun and is running.
	OrchestrationStatusOrchestrating = "orchestrating"
	// OrchestrationStatusCompleted denotes the OSD provisioning has completed. This does not imply
	// the provisioning completed successfully in whole or in part.
	OrchestrationStatusCompleted = "completed"
	// OrchestrationStatusAlreadyExists denotes the OSD provisioning was not started because an OSD
	// has already been provisioned and started.
	OrchestrationStatusAlreadyExists = "alreadyExists"
	// OrchestrationStatusFailed denotes the OSD provisioning has failed.
	OrchestrationStatusFailed = "failed"

	orchestrationStatusMapName = "rook-ceph-osd-%s-status"
	orchestrationStatusKey     = "status"
	provisioningLabelKey       = "provisioning"
	nodeLabelKey               = "node"
	completeProvisionTimeout   = 20
)

var (
	// time to wait before updating OSDs opportunistically while waiting for OSDs to finish provisioning
	osdOpportunisticUpdateDuration = 100 * time.Millisecond
)

type provisionConfig struct {
	errorMessages []string
	DataPathMap   *config.DataPathMap // location to store data in container
}

func (c *Cluster) newProvisionConfig() *provisionConfig {
	return &provisionConfig{
		DataPathMap: config.NewDatalessDaemonDataPathMap(c.clusterInfo.Namespace, c.spec.DataDirHostPath),
	}
}

func (c *provisionConfig) addError(message string, args ...interface{}) {
	logger.Errorf(message, args...)
	c.errorMessages = append(c.errorMessages, fmt.Sprintf(message, args...))
}

func (c *Cluster) updateOSDStatus(node string, status OrchestrationStatus) {
	UpdateNodeStatus(c.kv, node, status)
}

func UpdateNodeStatus(kv *k8sutil.ConfigMapKVStore, node string, status OrchestrationStatus) {
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
		// log the error, but allow the orchestration to continue even if the status update failed
		logger.Errorf("failed to set node %q status to %q for osd orchestration. %s", node, status.Status, status.Message)
	}
}

func (c *Cluster) handleOrchestrationFailure(config *provisionConfig, nodeName, message string) {
	config.addError(message)
	status := OrchestrationStatus{Status: OrchestrationStatusFailed, Message: message}
	UpdateNodeStatus(c.kv, nodeName, status)
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

func (c *Cluster) checkNodesCompleted(selector string, config *provisionConfig, configOSDs bool) (int, *util.Set, bool, *v1.ConfigMapList, []*v1.ConfigMap, error) {
	ctx := context.TODO()
	opts := metav1.ListOptions{
		LabelSelector: selector,
		Watch:         false,
	}
	remainingNodes := util.NewSet()
	deferredConfigMaps := []*v1.ConfigMap{}

	// check the status map to see if the node is already completed before we start watching
	statuses, err := c.context.Clientset.CoreV1().ConfigMaps(c.clusterInfo.Namespace).List(ctx, opts)
	if err != nil {
		if !kerrors.IsNotFound(err) {
			config.addError("failed to get config status. %v", err)
			return 0, remainingNodes, false, statuses, deferredConfigMaps, err
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
		var completed bool
		localconfigMap := configMap
		completed, deferredConfigMaps = c.handleStatusConfigMapStatus(node, config, &localconfigMap, deferredConfigMaps, configOSDs)
		if !completed {
			remainingNodes.Add(node)
		}
	}
	if remainingNodes.Count() == 0 {
		return originalNodes, remainingNodes, true, statuses, deferredConfigMaps, nil
	}
	return originalNodes, remainingNodes, false, statuses, deferredConfigMaps, nil
}

func (c *Cluster) completeOSDsForAllNodes(config *provisionConfig, configOSDs bool, timeoutMinutes int) bool {
	ctx := context.TODO()
	selector := fmt.Sprintf("%s=%s,%s=%s",
		k8sutil.AppAttr, AppName,
		orchestrationStatusKey, provisioningLabelKey,
	)

	// For OSDs that are definitely update operations, we want to be able to defer the update until
	// after we have created new OSDs in order to speed up cluster scale-out. Allow us to defer
	// updating by keeping a list of deferred configmaps and processing those after creates have
	// been done. If OSD provisioning doesn't report "alreadyExists" status, then no configmaps will
	// be deferred, and OSD create/update ops will be intermixed in whatever order is processed.
	deferredConfigMaps := []*v1.ConfigMap{}
	// handleDeferredConfigMaps NEEDS to happen before the outer function returns.
	// The nested for loops make it hard to break out and handle the return value cleanly,
	// so remember to call this before all return statements.
	handleDeferredConfigMaps := func() {
		logger.Debugf("handling all deferred OSD statuses")
		for _, configMap := range deferredConfigMaps {
			node, ok := configMap.Labels[nodeLabelKey]
			if !ok {
				logger.Infof("missing node label on deferred configmap %q", configMap.Name)
				continue
			}

			c.handleDeferredStatusConfigMapStatus(node, config, configMap, configOSDs)
		}
	}

	var originalNodes int
	var remainingNodes *util.Set
	var completed bool
	var statuses *v1.ConfigMapList
	var err error
	originalNodes, remainingNodes, completed, statuses, deferredConfigMaps, err = c.checkNodesCompleted(selector, config, configOSDs)
	if err == nil && completed {
		handleDeferredConfigMaps()
		return true
	}

	// Make a timer to help us opportunistically handle OSD updates while waiting on OSD prepare
	// jobs to finish. The timer value created here doesn't practically matter for the code below.
	opportunityTimer := time.NewTimer(osdOpportunisticUpdateDuration)

	// Make a timer that helps us log every minute that goes by without making progress and will
	// be used to determine if we have gone past our timeout without making progress.
	timeoutTimer := time.NewTimer(time.Minute)
	shouldResetTimeoutTimer := true // set true when timeout timer should be reset
	currentTimeoutMinutes := 0      // track how many minutes have gone by without progress

	for {
		// Check whether we need to cancel the orchestration
		if err := controller.CheckForCancelledOrchestration(c.context); err != nil {
			config.addError("%s", err.Error())
			return false
		}

		opts := metav1.ListOptions{
			LabelSelector: selector,
			Watch:         true,
			// start watching for changes on the orchestration status map
			ResourceVersion: statuses.ResourceVersion,
		}
		logger.Infof("%d/%d node(s) completed osd provisioning, resource version %v", (originalNodes - remainingNodes.Count()), originalNodes, opts.ResourceVersion)

		w, err := c.context.Clientset.CoreV1().ConfigMaps(c.clusterInfo.Namespace).Watch(ctx, opts)
		if err != nil {
			logger.Warningf("failed to start watch on osd status, trying again. %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		defer w.Stop()

	ResultLoop:
		for {
			// Reset the opportunity timer every time we start this loop. If the timer fires before
			// there is a ConfigMap update (new OSD created), we opportunistically handle an OSD
			// update and then return to the top of this loop. If the ConfigMap watcher has a result
			// before the timeout, a new OSD will be created. The opportunity timer might fire while
			// creation is happening. When the OSD is done being created, we return to the top of
			// this loop and reset the timeout here to make sure a previous timer firing doesn't
			// take update precedence away from any ConfigMap updates (new OSD) that need handled.
			if !opportunityTimer.Stop() {
				// the timer already fired
				if len(opportunityTimer.C) > 0 {
					// the timer fired, but the result channel needs drained before Reset below
					<-opportunityTimer.C
				}
			}
			opportunityTimer.Reset(osdOpportunisticUpdateDuration)

			if shouldResetTimeoutTimer {
				if !timeoutTimer.Stop() {
					// the timer already fired
					if len(timeoutTimer.C) > 0 {
						// the timer fired, but the result channel needs drained before Reset below
						<-timeoutTimer.C
					}
				}
				timeoutTimer.Reset(time.Minute)
			}
			shouldResetTimeoutTimer = false // don't reset the timer unless a case below asks for it

			select {
			case e, ok := <-w.ResultChan():
				if !ok {
					logger.Infof("orchestration status config map result channel closed, will restart watch.")
					w.Stop()
					<-time.After(5 * time.Second)
					var leftNodes int
					var leftRemainingNodes *util.Set
					var completed bool
					leftNodes, leftRemainingNodes, completed, _, deferredConfigMaps, err = c.checkNodesCompleted(selector, config, configOSDs)
					if err == nil {
						if completed {
							logger.Infof("additional %d/%d node(s) completed osd provisioning", leftNodes, originalNodes)
							handleDeferredConfigMaps()
							return true
						}
						remainingNodes = leftRemainingNodes
					} else {
						logger.Warningf("failed to list orchestration configmap, status: %v", err)
					}
					break ResultLoop
				}
				if e.Type == watch.Modified {
					configMap, ok := e.Object.(*v1.ConfigMap)
					if !ok {
						logger.Errorf("expected type ConfigMap but found %T", configMap)
						continue
					}
					node, ok := configMap.Labels[nodeLabelKey]
					if !ok {
						logger.Infof("missing node label on configmap %s", configMap.Name)
						continue
					}
					if !remainingNodes.Contains(node) {
						logger.Infof("skipping event from node %s status update since it is already completed", node)
						continue
					}
					var completed bool
					completed, deferredConfigMaps = c.handleStatusConfigMapStatus(node, config, configMap, deferredConfigMaps, configOSDs)
					if completed {
						remainingNodes.Remove(node)
						if remainingNodes.Count() == 0 {
							logger.Infof("%d/%d node(s) completed osd provisioning", originalNodes, originalNodes)
							handleDeferredConfigMaps()
							return true
						}
					}
					// made progress on creating/updating OSDs; reset the timeout timer
					shouldResetTimeoutTimer = true
				}

			case <-opportunityTimer.C:
				// We want to make a best-effort attempt to update configmaps while provisioning is
				// ongoing. While there are deferred configmaps remaining, grab one and process it.
				if len(deferredConfigMaps) > 0 {
					configMap := deferredConfigMaps[0]
					logger.Debugf("opportunistically handling deferred OSD status %q", configMap.Name)

					node, ok := configMap.Labels[nodeLabelKey]
					if !ok {
						logger.Infof("missing node label on deferred configmap %q", configMap.Name)
						continue
					}

					c.handleDeferredStatusConfigMapStatus(node, config, configMap, configOSDs)

					// remove the configmap we just processed from the deferred configmap list
					deferredConfigMaps = deferredConfigMaps[1:]
					// made progress on updating OSDs; reset the timeout timer
					shouldResetTimeoutTimer = true
				}

			case <-timeoutTimer.C:
				// Check whether we need to cancel the orchestration
				if err := controller.CheckForCancelledOrchestration(c.context); err != nil {
					config.addError("%s", err.Error())
					return false
				}
				// log every so often while we are waiting
				currentTimeoutMinutes++
				if currentTimeoutMinutes == timeoutMinutes {
					config.addError("timed out waiting for %d nodes: %+v", remainingNodes.Count(), remainingNodes)
					// start to remove remainingNodes waiting timeout.
					for remainingNode := range remainingNodes.Iter() {
						clearNodeName := k8sutil.TruncateNodeName(orchestrationStatusMapName, remainingNode)
						if err := c.kv.ClearStore(clearNodeName); err != nil {
							config.addError("failed to clear node %q status with name %q. %v", remainingNode, clearNodeName, err)
						}
					}
					handleDeferredConfigMaps()
					return false
				}
				logger.Infof("waiting on orchestration status update from %d remaining nodes", remainingNodes.Count())
				// made no progress, but the timer needs reset so we can timeout again
				shouldResetTimeoutTimer = true
			}
		}
	}
}

func (c *Cluster) createOrUpdateOSDs(status *OrchestrationStatus, nodeName string, config *provisionConfig, configMap *v1.ConfigMap) {
	if status.PvcBackedOSD {
		c.startOSDDaemonsOnPVC(nodeName, config, configMap, status)
	} else {
		c.startOSDDaemonsOnNode(nodeName, config, configMap, status)
	}
	// remove the status configmap that indicated the progress
	if err := c.kv.ClearStore(k8sutil.TruncateNodeName(orchestrationStatusMapName, nodeName)); err != nil {
		logger.Errorf("failed to remove the status configmap %q. %v", orchestrationStatusMapName, err)
	}
}

func (c *Cluster) handleStatusConfigMapStatus(nodeName string, config *provisionConfig, configMap *v1.ConfigMap, deferredConfigMaps []*v1.ConfigMap, configOSDs bool) (completed bool, updatedDeferredConfigMaps []*v1.ConfigMap) {
	// by default, do not append the input configmap to output to be deferred for processing later
	updatedDeferredConfigMaps = deferredConfigMaps

	status := parseOrchestrationStatus(configMap.Data)
	if status == nil {
		return false, deferredConfigMaps
	}

	logger.Infof("osd orchestration status for node %s is %s", nodeName, status.Status)

	if status.Status == OrchestrationStatusAlreadyExists {
		logger.Debugf("deferring OSD update for node %q", nodeName)

		// in case where the OSD has already been provisioned, this is an update case which we want
		// to defer until later
		updatedDeferredConfigMaps = append(deferredConfigMaps, configMap)

		// we report that the node finished provisioning here but still rely on the caller to make
		// sure to process deferred configmaps at a later point
		return true, updatedDeferredConfigMaps
	}

	if status.Status == OrchestrationStatusCompleted {
		if configOSDs {
			c.createOrUpdateOSDs(status, nodeName, config, configMap)
		}

		return true, updatedDeferredConfigMaps
	}

	if status.Status == OrchestrationStatusFailed {
		config.addError("orchestration for node %s failed: %+v", nodeName, status)
		return true, updatedDeferredConfigMaps
	}
	return false, updatedDeferredConfigMaps
}

// deferred configmaps are OSD update operations we want to defer until after new OSDs have been created
func (c *Cluster) handleDeferredStatusConfigMapStatus(nodeName string, config *provisionConfig, configMap *v1.ConfigMap, configOSDs bool) {
	status := parseOrchestrationStatus(configMap.Data)
	if status == nil {
		logger.Warningf("failed to handle deferred update of OSD for node %s. osd orchestration status for node %s is %s", nodeName, nodeName, status.Status)
		return
	}

	if configOSDs {
		logger.Infof("handling deferred update of OSD for node %q", nodeName)
		c.createOrUpdateOSDs(status, nodeName, config, configMap)
	}
}
