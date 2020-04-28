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

// Package k8sutil for Kubernetes helpers.
package k8sutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	rookversion "github.com/rook/rook/pkg/version"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-k8sutil")

const (
	// Namespace for rook
	Namespace = "rook"
	// DefaultNamespace for the cluster
	DefaultNamespace = "default"
	// DataDirVolume data dir volume
	DataDirVolume = "rook-data"
	// DataDir folder
	DataDir = "/var/lib/rook"
	// RookType for the CRD
	RookType = "kubernetes.io/rook"
	// PodNameEnvVar is the env variable for getting the pod name via downward api
	PodNameEnvVar = "POD_NAME"
	// PodNamespaceEnvVar is the env variable for getting the pod namespace via downward api
	PodNamespaceEnvVar = "POD_NAMESPACE"
	// NodeNameEnvVar is the env variable for getting the node via downward api
	NodeNameEnvVar = "NODE_NAME"

	// RookVersionLabelKey is the key used for reporting the Rook version which last created or
	// modified a resource.
	RookVersionLabelKey = "rook-version"
)

// GetK8SVersion gets the version of the running K8S cluster
func GetK8SVersion(clientset kubernetes.Interface) (*version.Version, error) {
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("error getting server version: %v", err)
	}

	// make sure the kubernetes version is parseable
	index := strings.Index(serverVersion.GitVersion, "+")
	if index != -1 {
		newVersion := serverVersion.GitVersion[:index]
		logger.Infof("returning version %s instead of %s", newVersion, serverVersion.GitVersion)
		serverVersion.GitVersion = newVersion
	}
	return version.MustParseSemantic(serverVersion.GitVersion), nil
}

// Hash stableName computes a stable pseudorandom string suitable for inclusion in a Kubernetes object name from the given seed string.
func Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:16])
}

// TruncateNodeName hashes the nodeName in case it would case the name to be longer than 63 characters
// WARNING If your format and nodeName as a hash, are longer than 63 chars it won't be truncated!
// Your format alone should only be 31 chars at max because of MD5 hash being 32 chars.
// For more information, see the following resources:
// https://stackoverflow.com/a/50451893
// https://stackoverflow.com/a/32294443
func TruncateNodeName(format, nodeName string) string {
	if len(nodeName)+len(fmt.Sprintf(format, "")) > validation.DNS1035LabelMaxLength {
		hashed := Hash(nodeName)
		logger.Infof("format and nodeName longer than %d chars, nodeName %s will be %s", validation.DNS1035LabelMaxLength, nodeName, hashed)
		nodeName = hashed
	}
	return fmt.Sprintf(format, nodeName)
}

// deleteResourceAndWait will delete a resource, then wait for it to be purged from the system
func deleteResourceAndWait(namespace, name, resourceType string,
	deleteAction func(*metav1.DeleteOptions) error,
	getAction func() error,
) error {
	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}

	// Delete the resource if it exists
	logger.Infof("removing %s %s if it exists", resourceType, name)
	err := deleteAction(options)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete %s. %+v", name, err)
		}
		return nil
	}
	logger.Infof("Removed %s %s", resourceType, name)

	// wait for the resource to be deleted
	sleepTime := 2 * time.Second
	for i := 0; i < 45; i++ {
		// check for the existence of the resource
		err = getAction()
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Infof("confirmed %s does not exist", name)
				return nil
			}
			return fmt.Errorf("failed to get %s. %+v", name, err)
		}

		if i%5 == 0 {
			// occasionally print a message
			logger.Infof("%q still found. waiting...", name)
		}
		time.Sleep(sleepTime)
	}

	return fmt.Errorf("gave up waiting for %s pods to be terminated", name)
}

// Add the rook version to the labels. This should *not* be used on pod specifications, because this
// will result in the deployment/daemonset/ect. recreating all of its pods even if an update
// wouldn't otherwise be required. Upgrading unnecessarily increases risk for loss of data
// reliability, even if only briefly.
//
// Note that the label may not match the version string exactly, since some characters used
// in version strings are illegal in pod labels.
func addRookVersionLabel(labels map[string]string) {
	value := validateLabelValue(rookversion.Version)
	labels[RookVersionLabelKey] = value
}

// validateLabelValue replaces any invalid characters
// in the input string with a replacement character,
// and enforces other limitations for k8s label values.
//
// See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
func validateLabelValue(value string) string {
	repl := "-"
	maxlen := validation.LabelValueMaxLength
	re := regexp.MustCompile("[^a-z0-9A-Z._-]")
	// restrict label value to valid character set
	sanitized := re.ReplaceAllLiteralString(value, repl)
	// ensure value begins and ends with a valid character
	sanitized = strings.TrimRight(strings.TrimLeft(sanitized, ".-_"), ".-_")
	// limit length
	if len(sanitized) > maxlen {
		sanitized = sanitized[:maxlen]
	}
	return sanitized
}

// StartOperatorSettingsWatch starts the watch for Operator Settings ConfigMap
func StartOperatorSettingsWatch(context *clusterd.Context, operatorNamespace, operatorSettingConfigMapName string,
	addFunc func(obj interface{}), updateFunc func(oldObj, newObj interface{}), deleteFunc func(obj interface{}), stopCh chan struct{}) {
	_, cacheController := cache.NewInformer(cache.NewFilteredListWatchFromClient(context.Clientset.CoreV1().RESTClient(),
		"configmaps", operatorNamespace, func(options *metav1.ListOptions) {
			options.FieldSelector = fmt.Sprintf("%s=%s", "metadata.name", operatorSettingConfigMapName)
		}), &v1.ConfigMap{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    addFunc,
			UpdateFunc: updateFunc,
			DeleteFunc: deleteFunc,
		})
	go cacheController.Run(stopCh)
}
