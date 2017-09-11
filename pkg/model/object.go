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

import (
	"time"

	"k8s.io/api/core/v1"
)

const RGWPort = 53390

type ObjectStoreResponse struct {
	Name        string           `json:"name"`
	ClusterIP   string           `json:"clusterIP"`
	ExternalIPs []string         `json:"externalIPs"`
	Ports       []v1.ServicePort `json:"ports"`
}

type ObjectStore struct {
	Name           string  `json:"name"`
	DataConfig     Pool    `json:"dataConfig"`
	MetadataConfig Pool    `json:"metadataConfig"`
	Gateway        Gateway `json:"gateway"`
}

type Gateway struct {
	Port           int32  `json:"port"`
	Replicas       int32  `json:"replicas"`
	Certificate    string `json:"certificate"`
	CertificateRef string `json:"certificateRef"`
}

type ObjectStoreConnectInfo struct {
	Host       string `json:"host"`
	IPEndpoint string `json:"ipEndpoint"`
}

type ObjectUser struct {
	UserID      string  `json:"userId"`
	DisplayName *string `json:"displayName"`
	Email       *string `json:"email"`
	AccessKey   *string `json:"accessKey"`
	SecretKey   *string `json:"secretKey"`
}

type ObjectBucketMetadata struct {
	Owner     string    `json:"owner"`
	CreatedAt time.Time `json:"createdAt"`
}

type ObjectBucketStats struct {
	Size            uint64 `json:"size"`
	NumberOfObjects uint64 `json:"numberOfObjects"`
}

type ObjectBucket struct {
	Name string `json:"name"`
	ObjectBucketMetadata
	ObjectBucketStats
}

type ObjectBuckets []ObjectBucket

func (slice ObjectBuckets) Len() int {
	return len(slice)
}

func (slice ObjectBuckets) Less(i, j int) bool {
	return slice[i].Name < slice[j].Name
}

func (slice ObjectBuckets) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}
