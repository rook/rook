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

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8swatch "k8s.io/apimachinery/pkg/watch"
)

const (
	// OrchestrationStatusStarting denotes the OSD provisioning is beginning.
	OrchestrationStatusStarting = "starting"
	// OrchestrationStatusOrchestrating denotes the OSD provisioning has begun and is running.
	OrchestrationStatusOrchestrating = "orchestrating"
	// OrchestrationStatusCompleted denotes the OSD provisioning has completed. This does not imply
	// the provisioning completed successfully in whole or in part.
	OrchestrationStatusCompleted = "completed"
	// OrchestrationStatusFailed denotes the OSD provisioning has failed.
	OrchestrationStatusFailed = "failed"

	orchestrationStatusMapName = "rook-ceph-osd-%s-status"
	orchestrationStatusKey     = "status"
	provisioningLabelKey       = "provisioning"
	nodeLabelKey               = "node"
)

var (
	// time to wait before updating OSDs opportunistically while waiting for OSDs to finish provisioning
	osdOpportunisticUpdateDuration = 100 * time.Millisecond

	// a ticker that ticks every minute to check progress
	minuteTickerDuration = time.Minute
)

type provisionConfig struct {
	DataPathMap *config.DataPathMap // location to store data in OSD and OSD prepare containers
}

func (c *Cluster) newProvisionConfig() *provisionConfig {
	return &provisionConfig{
		DataPathMap: config.NewDatalessDaemonDataPathMap(c.clusterInfo.Namespace, c.spec.DataDirHostPath),
	}
}

// The provisionErrors struct can get passed around to provisioning code which can add errors to its
// internal list of errors. The errors will be reported at the end of provisioning.
type provisionErrors struct {
	errors []error
}

func newProvisionErrors() *provisionErrors {
	return &provisionErrors{
		errors: []error{},
	}
}

func (e *provisionErrors) addError(message string, args ...interface{}) {
	logger.Errorf(message, args...)
	e.errors = append(e.errors, errors.Errorf(message, args...))
}

func (e *provisionErrors) len() int {
	return len(e.errors)
}

func (e *provisionErrors) asMessages() string {
	o := ""
	for _, err := range e.errors {
		o = fmt.Sprintf("%s\n%v", o, err)
	}
	return o
}

// return name of status ConfigMap
func (c *Cluster) updateOSDStatus(node string, status OrchestrationStatus) string {
	return UpdateNodeOrPVCStatus(c.kv, node, status)
}

func statusConfigMapLabels(node string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr:        AppName,
		orchestrationStatusKey: provisioningLabelKey,
		nodeLabelKey:           node,
	}
}

// UpdateNodeOrPVCStatus updates the status ConfigMap for the OSD on the given node or PVC. It returns the name
// the ConfigMap used.
func UpdateNodeOrPVCStatus(kv *k8sutil.ConfigMapKVStore, nodeOrPVC string, status OrchestrationStatus) string {
	labels := statusConfigMapLabels(nodeOrPVC)

	// update the status map with the given status now
	s, _ := json.Marshal(status)
	cmName := statusConfigMapName(nodeOrPVC)
	if err := kv.SetValueWithLabels(
		cmName,
		orchestrationStatusKey,
		string(s),
		labels,
	); err != nil {
		// log the error, but allow the orchestration to continue even if the status update failed
		logger.Errorf("failed to set node or PVC %q status to %q for osd orchestration. %s", nodeOrPVC, status.Status, status.Message)
	}
	return cmName
}

func (c *Cluster) handleOrchestrationFailure(errors *provisionErrors, nodeName, message string, args ...interface{}) {
	errors.addError(message, args...)
	status := OrchestrationStatus{Status: OrchestrationStatusFailed, Message: message}
	UpdateNodeOrPVCStatus(c.kv, nodeName, status)
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

// return errors from this function when OSD provisioning should stop and the reconcile be restarted
func (c *Cluster) updateAndCreateOSDs(
	createConfig *createConfig,
	updateConfig *updateConfig,
	errs *provisionErrors, // add errors here
) error {
	// tick every mintue to check-in on housekeeping stuff and report overall progress
	minuteTicker := time.NewTicker(minuteTickerDuration)
	defer minuteTicker.Stop()

	var err error

	doLoop := true
	for doLoop {
		doLoop, err = c.updateAndCreateOSDsLoop(createConfig, updateConfig, minuteTicker, errs)
		if err != nil {
			if !doLoop {
				return err
			}
			logger.Errorf("%v", err)
		}
	}

	return nil
}

func statusConfigMapSelector() string {
	return fmt.Sprintf("%s=%s,%s=%s",
		k8sutil.AppAttr, AppName,
		orchestrationStatusKey, provisioningLabelKey,
	)
}

func (c *Cluster) updateAndCreateOSDsLoop(
	createConfig *createConfig,
	updateConfig *updateConfig,
	minuteTicker *time.Ticker, // pass in the minute ticker so that we always know when a minute passes
	errs *provisionErrors, // add errors here
) (shouldRestart bool, err error) {
	cmClient := c.context.Clientset.CoreV1().ConfigMaps(c.clusterInfo.Namespace)
	ctx := context.TODO()
	selector := statusConfigMapSelector()

	listOptions := metav1.ListOptions{
		LabelSelector: selector,
	}
	configMapList, err := cmClient.List(ctx, listOptions)
	if err != nil {
		return false, errors.Wrapf(err, "failed to list OSD provisioning status ConfigMaps")
	}

	// Process the configmaps initially in case any are already in a processable state
	for i := range configMapList.Items {
		// reference index to prevent implicit memory aliasing error
		c.createOSDsForStatusMap(&configMapList.Items[i], createConfig, errs)
	}

	watchOptions := metav1.ListOptions{
		LabelSelector:   selector,
		Watch:           true,
		ResourceVersion: configMapList.ResourceVersion,
	}
	watcher, err := cmClient.Watch(ctx, watchOptions)
	defer watcher.Stop()
	if err != nil {
		return false, errors.Wrapf(err, "failed to start watching OSD provisioning status ConfigMaps")
	}

	// tick after a short time of waiting for new OSD provision status configmaps to change state
	// in order to allow opportunistic deployment updates while we wait
	updateTicker := time.NewTicker(osdOpportunisticUpdateDuration)
	defer updateTicker.Stop()

	watchErrMsg := "failed during watch of OSD provisioning status ConfigMaps"
	for {
		if updateConfig.doneUpdating() && createConfig.doneCreating() {
			break // loop
		}

		// reset the update ticker (and drain the channel if necessary) to make sure we always
		// wait a little bit for an OSD prepare result before opportunistically updating deployments
		updateTicker.Reset(osdOpportunisticUpdateDuration)
		if len(updateTicker.C) > 0 {
			<-updateTicker.C
		}

		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				logger.Infof("restarting watcher for OSD provisioning status ConfigMaps. the watcher closed the channel")
				return true, nil
			}

			if !isAddOrModifyEvent(event.Type) {
				// We don't want to process delete events when we delete configmaps after having
				// processed them. We also don't want to process BOOKMARK or ERROR events.
				logger.Debugf("not processing %s event for object %q", event.Type, eventObjectName(event))
				break // case
			}

			configMap, ok := event.Object.(*corev1.ConfigMap)
			if !ok {
				logger.Errorf("recovering. %s. expected type ConfigMap but found %T", watchErrMsg, configMap)
				break // case
			}

			c.createOSDsForStatusMap(configMap, createConfig, errs)

		case <-updateTicker.C:
			// do an update
			updateConfig.updateExistingOSDs(errs)

		case <-minuteTicker.C:
			// Check whether we need to cancel the orchestration
			if err := controller.CheckForCancelledOrchestration(c.context); err != nil {
				return false, err
			}
			// Log progress
			c, cExp := createConfig.progress()
			u, uExp := updateConfig.progress()
			logger.Infof("waiting... %d of %d OSD prepare jobs have finished processing and %d of %d OSDs have been updated", c, cExp, u, uExp)
		}
	}

	return false, nil
}

func isAddOrModifyEvent(t k8swatch.EventType) bool {
	switch t {
	case k8swatch.Added, k8swatch.Modified:
		return true
	default:
		return false
	}
}

func eventObjectName(e k8swatch.Event) string {
	objName := "[could not determine name]"
	objMeta, _ := meta.Accessor(e.Object)
	if objMeta != nil {
		objName = objMeta.GetName()
	}
	return objName
}

// Create OSD Deployments for OSDs reported by the prepare job status configmap.
// Do not create OSD deployments if a deployment already exists for a given OSD.
func (c *Cluster) createOSDsForStatusMap(
	configMap *corev1.ConfigMap,
	createConfig *createConfig,
	errs *provisionErrors, // add errors here
) {
	nodeOrPVCName, ok := configMap.Labels[nodeLabelKey]
	if !ok {
		logger.Warningf("missing node label on configmap %s", configMap.Name)
		return
	}

	status := parseOrchestrationStatus(configMap.Data)
	if status == nil {
		return
	}
	nodeOrPVC := "node"
	if status.PvcBackedOSD {
		nodeOrPVC = "PVC"
	}

	logger.Infof("OSD orchestration status for %s %s is %q", nodeOrPVC, nodeOrPVCName, status.Status)

	if status.Status == OrchestrationStatusCompleted {
		createConfig.createNewOSDsFromStatus(status, nodeOrPVCName, errs)
		c.deleteStatusConfigMap(nodeOrPVCName) // remove the provisioning status configmap
		return
	}

	if status.Status == OrchestrationStatusFailed {
		createConfig.doneWithStatus(nodeOrPVCName)
		errs.addError("failed to provision OSD(s) on %s %s. %+v", nodeOrPVC, nodeOrPVCName, status)
		c.deleteStatusConfigMap(nodeOrPVCName) // remove the provisioning status configmap
		return
	}
}

func statusConfigMapName(nodeOrPVCName string) string {
	return k8sutil.TruncateNodeName(orchestrationStatusMapName, nodeOrPVCName)
}

func (c *Cluster) deleteStatusConfigMap(nodeOrPVCName string) {
	if err := c.kv.ClearStore(statusConfigMapName(nodeOrPVCName)); err != nil {
		logger.Errorf("failed to remove the status configmap %q. %v", statusConfigMapName(nodeOrPVCName), err)
	}
}

func (c *Cluster) deleteAllStatusConfigMaps() {
	ctx := context.TODO()
	listOpts := metav1.ListOptions{
		LabelSelector: statusConfigMapSelector(),
	}
	cmClientset := c.context.Clientset.CoreV1().ConfigMaps(c.clusterInfo.Namespace)
	cms, err := cmClientset.List(ctx, listOpts)
	if err != nil {
		logger.Warningf("failed to clean up any dangling OSD prepare status configmaps. failed to list OSD prepare status configmaps. %v", err)
		return
	}
	for _, cm := range cms.Items {
		logger.Debugf("cleaning up dangling OSD prepare status configmap %q", cm.Name)
		err := cmClientset.Delete(ctx, cm.Name, metav1.DeleteOptions{})
		if err != nil {
			logger.Warningf("failed to clean up dangling OSD prepare status configmap %q. %v", cm.Name, err)
		}
	}
}
