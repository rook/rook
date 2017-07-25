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

// Package osd for the Ceph OSDs.
package osd

import (
	cephosd "github.com/rook/rook/pkg/ceph/osd"
)

// StorageSpec CRD settings
type StorageSpec struct {
	Nodes       []Node `json:"nodes,omitempty"`
	UseAllNodes bool   `json:"useAllNodes,omitempty"`
	Selection
	Config
}

// Node specific CRD settings
type Node struct {
	Name        string      `json:"name,omitempty"`
	Devices     []Device    `json:"devices,omitempty"`
	Directories []Directory `json:"directories,omitempty"`
	Selection
	Config
}

// Device CRD settings
type Device struct {
	Name string `json:"name,omitempty"`
}

// Directory CRD settings
type Directory struct {
	Path string `json:"path,omitempty"`
}

// Selection CRD settings
type Selection struct {
	// Whether to consume all the storage devices found on a machine
	UseAllDevices *bool `json:"useAllDevices,omitempty"`

	// A regular expression to allow more fine-grained selection of devices on nodes across the cluster
	DeviceFilter string `json:"deviceFilter,omitempty"`

	MetadataDevice string `json:"metadataDevice,omitempty"`
}

// Config CRD settings
type Config struct {
	StoreConfig cephosd.StoreConfig `json:"storeConfig,omitempty"`
	Location    string              `json:"location,omitempty"`
}
