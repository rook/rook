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

package contracts

import (
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/tests/framework/utils"
)

//BlockOperator - interface for rook block operations
type BlockOperator interface {
	BlockCreate(name string, size int) (string, error)
	BlockDelete(name string) (string, error)
	BlockList() ([]model.BlockImage, error)
	BlockMap(name string, mountpath string) (string, error)
	BlockWrite(name string, mountpath string, data string, filename string, namespace string) (string, error)
	BlockRead(name string, mountpath string, filename string, namespace string) (string, error)
	BlockUnmap(name string, mountpath string) (string, error)
}

//FileSystemOperator - interface for rook fileSystem operations
type FileSystemOperator interface {
	FSCreate(name, namespace string, callAPI bool, k8sh *utils.K8sHelper) error
	FSDelete(name string) (string, error)
	FSList() ([]model.Filesystem, error)
	FSMount(name string, mountpath string) (string, error)
	FSWrite(name string, mountpath string, data string, filename string, namespace string) (string, error)
	FSRead(name string, mountpath string, filename string, namespace string) (string, error)
	FSUnmount(mountpath string) (string, error)
}

//ObjectOperator - interface for rook object operations
type ObjectOperator interface {
	ObjectCreate(namespace, storeName string, replicaCount int32, dnsName string, callAPI bool, k8sh *utils.K8sHelper) error
	ObjectDelete(namespace, storeName string, replicaCount int32, callAPI bool, k8sh *utils.K8sHelper) error
	ObjectBucketList(storeName string) ([]model.ObjectBucket, error)
	ObjectConnection(storeName string) (*model.ObjectStoreConnectInfo, error)
	ObjectCreateUser(storeName, userid string, displayname string) (*model.ObjectUser, error)
	ObjectUpdateUser(storeName, userid string, displayname string, emailid string) (*model.ObjectUser, error)
	ObjectDeleteUser(storeName, userid string) error
	ObjectGetUser(storeName, userid string) (*model.ObjectUser, error)
	ObjectListUser(storeName string) ([]model.ObjectUser, error)
}

//PoolOperator - interface for rook pool operations
type PoolOperator interface {
	PoolList() ([]model.Pool, error)
	PoolCreate(pool model.Pool) (string, error)
}

//RestAPIOperator - interface for rook rest API operations
type RestAPIOperator interface {
	URL() string
	GetNodes() ([]model.Node, error)
	GetPools() ([]model.Pool, error)
	CreatePool(pool model.Pool) (string, error)
	GetBlockImages() ([]model.BlockImage, error)
	CreateBlockImage(image model.BlockImage) (string, error)
	DeleteBlockImage(image model.BlockImage) (string, error)
	GetClientAccessInfo() (model.ClientAccessInfo, error)
	GetFilesystems() ([]model.Filesystem, error)
	CreateFilesystem(fsmodel model.FilesystemRequest) (string, error)
	DeleteFilesystem(fsmodel model.FilesystemRequest) (string, error)
	GetStatusDetails() (model.StatusDetails, error)
	CreateObjectStore(store model.ObjectStore) (string, error)
	DeleteObjectStore(storeName string) error
	GetObjectStoreConnectionInfo(storeName string) (*model.ObjectStoreConnectInfo, error)
	ListBuckets(storeName string) ([]model.ObjectBucket, error)
	ListObjectUsers(storeName string) ([]model.ObjectUser, error)
	GetObjectUser(storeName, id string) (*model.ObjectUser, error)
	CreateObjectUser(storeName string, user model.ObjectUser) (*model.ObjectUser, error)
	UpdateObjectUser(storeName string, user model.ObjectUser) (*model.ObjectUser, error)
	DeleteObjectUser(storeName, id string) error
}

type TestOperator interface {
	TearDown()
}
