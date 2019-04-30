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
package v1beta1

const (
	DeploymentRtlfs     = "rtlfs"
	DeploymentRtrd      = "rtrd"
	DeploymentAutoRtlfs = "autoRtlfs"
)

type ClusterDeploymentConfig struct {
	DeploymentType string        //rtlfs, rtrd, autortlfs
	TransportKey   string        //rtlfs or rtrd
	Directories    []RtlfsDevice //cluster wide directories
	DevConfig      map[string]DevicesConfig
	NeedPrivileges bool
}

type DevicesConfig struct {
	Rtrd          RTDevices
	RtrdSlaves    []RTDevices
	Rtlfs         RtlfsDevices
	Zone          int
	IsGatewayNode bool
}

type DevicesResurrectOptions struct {
	NeedToResurrect bool
	NeedToZap       bool
	NeedToWait      bool
	SlaveContainers int
}
