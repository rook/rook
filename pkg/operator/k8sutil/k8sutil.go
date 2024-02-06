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
	"github.com/pkg/errors"
	rookversion "github.com/rook/rook/pkg/version"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/kubernetes"
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
		logger.Debugf("returning version %s instead of %s", newVersion, serverVersion.GitVersion)
		serverVersion.GitVersion = newVersion
	}
	return version.MustParseSemantic(serverVersion.GitVersion), nil
}

// Hash stableName computes a stable pseudorandom string suitable for inclusion in a Kubernetes object name from the given seed string.
// Do **NOT** edit this function in a way that would change its output as it needs to
// provide consistent mappings from string to hash across versions of rook.
func Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:16])
}

// TruncateNodeNameForJob hashes the nodeName in case it would case the name to be longer than 63 characters
// and avoids for a K8s 1.22 bug in the job pod name generation. If the job name contains a . or - in a certain
// position, the pod will fail to create.
func TruncateNodeNameForJob(format, nodeName string) string {
	// In k8s 1.22, the job name is truncated an additional 10 characters which can cause an issue
	// in the generated pod name if it then ends in a non-alphanumeric character. In that case,
	// we more aggressively generate a hashed job name.
	jobNameShortenLength := 10
	return truncateNodeName(format, nodeName, validation.DNS1035LabelMaxLength-jobNameShortenLength)
}

// TruncateNodeName hashes the nodeName in case it would case the name to be longer than 63 characters
// WARNING If your format and nodeName as a hash, are longer than 63 chars it won't be truncated!
// Your format alone should only be 31 chars at max because of MD5 hash being 32 chars.
// For more information, see the following resources:
// https://stackoverflow.com/a/50451893
// https://stackoverflow.com/a/32294443
// Do **NOT** edit this function in a way that would change its output as it needs to
// provide consistent mappings from string to hash across versions of rook.
func TruncateNodeName(format, nodeName string) string {
	return truncateNodeName(format, nodeName, validation.DNS1035LabelMaxLength)
}

// truncateNodeName takes the max length desired for a string and hashes the value if needed to shorten it.
func truncateNodeName(format, nodeName string, maxLength int) string {
	if len(nodeName)+len(fmt.Sprintf(format, "")) > maxLength {
		hashed := Hash(nodeName)
		logger.Infof("format and nodeName longer than %d chars, nodeName %s will be %s", maxLength, nodeName, hashed)
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
		if !kerrors.IsNotFound(err) {
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
			if kerrors.IsNotFound(err) {
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
// will result in the deployment/daemonset/etc. recreating all of its pods even if an update
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

func UsePDBV1Beta1Version(Clientset kubernetes.Interface) (bool, error) {
	k8sVersion, err := GetK8SVersion(Clientset)
	if err != nil {
		return false, errors.Wrap(err, "failed to fetch k8s version")
	}
	logger.Debugf("kubernetes version fetched %v", k8sVersion)
	// minimum k8s version required for v1 PodDisruptionBudget is 'v1.21.0'. Apply v1 if k8s version is at least 'v1.21.0', else apply v1beta1 PodDisruptionBudget.
	minVersionForPDBV1 := "1.21.0"
	return k8sVersion.LessThan(version.MustParseSemantic(minVersionForPDBV1)), nil
}

// ToValidDNSLabel converts a given string to a valid DNS-1035 spec label. The DNS-1035 spec
// follows the regex '[a-z]([-a-z0-9]*[a-z0-9])?' and is at most 63 chars long. DNS-1035 is used
// over DNS-1123 because it is more strict. Kubernetes docs are not always clear when a DNS_LABEL is
// supposed to be 1035 or 1123 compliant, so we use the more strict version for ease of use.
//   - Any input symbol that is not valid is converted to a dash ('-').
//   - Multiple resultant dashes in a row are compressed to a single dash.
//   - If the starting character is a number, a 'd' is prepended to preserve the number.
//   - Any non-alphanumeric starting or ending characters are removed.
//   - If the resultant string is longer than the maximum-allowed 63 characters], characters are
//     removed from the middle and replaced with a double dash ('--') to reduce the string to 63
//     characters.
func ToValidDNSLabel(input string) string {
	maxl := validation.DNS1035LabelMaxLength

	if input == "" {
		return ""
	}

	outbuf := make([]byte, len(input)+1)
	j := 0 // position in output buffer
	last := byte('-')
	for _, c := range []byte(input) {
		switch {
		case c >= 'a' && c <= 'z':
			outbuf[j] = c
		case c >= '0' && c <= '9':
			// if the first char is a number, add a 'd' (for decimal) in front
			if j == 0 {
				outbuf[j] = 'd' // for decimal
				j++
			}
			outbuf[j] = c
		case c >= 'A' && c <= 'Z':
			// convert to lower case
			outbuf[j] = c - 'A' + 'a' // convert to lower case
		default:
			if last == '-' {
				// don't write two dashes in a row
				continue
			}
			outbuf[j] = byte('-')
		}
		last = outbuf[j]
		j++
	}

	// set the length of the output buffer to the number of chars we copied to it so there aren't
	// \0x00 chars at the end
	outbuf = outbuf[:j]

	// trim any leading or trailing dashes
	out := strings.Trim(string(outbuf), "-")

	// if string is longer than max length, cut content from the middle to get it to length
	if len(out) > maxl {
		out = cutMiddle(out, maxl)
	}

	return out
}

// don't use this function for anything less than toSize=4 chars long
func cutMiddle(input string, toSize int) string {
	if len(input) <= toSize {
		return input
	}

	lenLeft := toSize / 2               // truncation rounds down the left side
	lenRight := toSize/2 + (toSize % 2) // modulo rounds up the right side

	buf := []byte(input)

	return string(buf[:lenLeft-1]) + "--" + string(buf[len(input)-lenRight+1:])
}
