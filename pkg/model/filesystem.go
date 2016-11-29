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
package model

type FilesystemRequest struct {
	Name     string `json:"name"`
	PoolName string `json:"poolName"`
}

type Filesystem struct {
	Name         string   `json:"name"`
	MetadataPool string   `json:"metadataPool"`
	DataPools    []string `json:"dataPools"`
}
