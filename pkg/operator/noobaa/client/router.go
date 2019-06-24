/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package client

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// RPCRouter should be able to map noobaa api names to actual addresses
// See implementations below: RPCRouterNodePort, RPCRouterPodPort, RPCRouterServicePort
type RPCRouter interface {
	GetAddress(api string) string
}

// RPCRouterNodePort uses the service node port to route to NodeIP:NodePorts
type RPCRouterNodePort struct {
	ServiceMgmt *corev1.Service
	NodeIP      string
}

// RPCRouterPodPort uses the service target port to route to PodIP:TargetPort
type RPCRouterPodPort struct {
	ServiceMgmt *corev1.Service
	PodIP       string
}

// RPCRouterServicePort uses the service port to route to Srv.Namespace:Port
type RPCRouterServicePort struct {
	ServiceMgmt *corev1.Service
}

// GetAddress implements the router
func (r *RPCRouterNodePort) GetAddress(api string) string {
	port := FindPortByName(r.ServiceMgmt, GetAPIPortName(api)).NodePort
	return fmt.Sprintf("https://%s:%d/rpc/", r.NodeIP, port)
}

// GetAddress implements the router
func (r *RPCRouterPodPort) GetAddress(api string) string {
	port := FindPortByName(r.ServiceMgmt, GetAPIPortName(api)).TargetPort.IntValue()
	return fmt.Sprintf("https://%s:%d/rpc/", r.PodIP, port)
}

// GetAddress implements the router
func (r *RPCRouterServicePort) GetAddress(api string) string {
	port := FindPortByName(r.ServiceMgmt, GetAPIPortName(api)).Port
	return fmt.Sprintf("https://%s.%s:%d/rpc/", r.ServiceMgmt.Name, r.ServiceMgmt.Namespace, port)
}

// FindPortByName returns the port in the service that matches the given name.
func FindPortByName(srv *corev1.Service, portName string) *corev1.ServicePort {
	for _, p := range srv.Spec.Ports {
		if p.Name == portName {
			return &p
		}
	}
	return &corev1.ServicePort{}
}

// GetAPIPortName maps every noobaa api name to the service port name that serves it.
func GetAPIPortName(api string) string {
	if api == "object_api" || api == "func_api" {
		return "md-https"
	}
	if api == "scrubber_api" {
		return "bg-https"
	}
	if api == "hosted_agents_api" {
		return "hosted-agents-https"
	}
	return "mgmt-https"
}
