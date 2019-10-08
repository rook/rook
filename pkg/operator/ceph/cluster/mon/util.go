/*
Copyright 2017 The Rook Authors. All rights reserved.

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

package mon

import (
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	maxPerChar = 26
)

func monInQuorum(monitor client.MonMapEntry, quorum []int) bool {
	for _, rank := range quorum {
		if rank == monitor.Rank {
			return true
		}
	}
	return false
}

// convert the mon name to the numeric mon ID
func fullNameToIndex(name string) (int, error) {
	prefix := AppName + "-"
	if strings.Index(name, prefix) != -1 && len(prefix) < len(name) {
		return k8sutil.NameToIndex(name[len(prefix)+1:])
	}

	// attempt to parse the legacy mon name
	legacyPrefix := AppName
	if strings.Index(name, legacyPrefix) == -1 || len(name) < len(AppName) {
		return -1, errors.New("unexpected mon name")
	}
	id, err := strconv.Atoi(name[len(legacyPrefix):])
	if err != nil {
		return -1, err
	}
	return id, nil
}

// addServicePort adds a port to a service
func addServicePort(service *v1.Service, name string, port int32) {
	if port == 0 {
		return
	}
	service.Spec.Ports = append(service.Spec.Ports, v1.ServicePort{
		Name:       name,
		Port:       port,
		TargetPort: intstr.FromInt(int(port)),
		Protocol:   v1.ProtocolTCP,
	})
}

// addContainerPort adds a port to a container
func addContainerPort(container v1.Container, name string, port int32) {
	if port == 0 {
		return
	}
	container.Ports = append(container.Ports, v1.ContainerPort{
		Name:          name,
		ContainerPort: port,
		Protocol:      v1.ProtocolTCP,
	})
}
