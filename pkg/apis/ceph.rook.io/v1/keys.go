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
	rookcore "github.com/rook/rook/pkg/apis/rook.io"
)

const (
	KeyAll                         = "all"
	KeyMds        rookcore.KeyType = "mds"
	KeyMon        rookcore.KeyType = "mon"
	KeyMonArbiter rookcore.KeyType = "arbiter"
	KeyMgr        rookcore.KeyType = "mgr"
	KeyOSDPrepare rookcore.KeyType = "prepareosd"
	KeyOSD        rookcore.KeyType = "osd"
	KeyCleanup    rookcore.KeyType = "cleanup"
	KeyMonitoring rookcore.KeyType = "monitoring"
)
