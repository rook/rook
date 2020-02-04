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

package v1

import (
	rook "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
)

const (
	KeyMon            rook.KeyType = "mon"
	KeyMgr            rook.KeyType = "mgr"
	KeyOSD            rook.KeyType = "osd"
	KeyRBDMirror      rook.KeyType = "rbdmirror"
	KeyRGW            rook.KeyType = "rgw"
	KeyCSIProvisioner rook.KeyType = "csi-provisioner"
	KeyCSIPlugin      rook.KeyType = "csi-plugin"
)
