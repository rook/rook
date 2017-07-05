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

package smoke

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/enums"
	"github.com/rook/rook/tests/framework/utils"
	"strings"
)

const (
	dataPath = "src/github.com/rook/rook/tests/data"
)

//TestHelper struct for smoke test
type TestHelper struct {
	platform   enums.RookPlatformType
	rookclient *clients.TestClient
	k8sHelp    *utils.K8sHelper
	k8sVersion string
}

type blockTestData struct {
	name       string
	size       int
	mountpath  string
	pvcDefPath string
	podDefPath string
}

type fileTestData struct {
	name       string
	mountpath  string
	podname    string
	podDefPath string
}

type objectUserData struct {
	userID      string
	displayname string
	emailID     string
}

type objectConnectionData struct {
	awsEndpoint        string
	awsHost            string
	awsSecretKeyID     string
	awsSecretAccessKey string
}

func createObjectUserData(userid string, displayname string, emailid string) objectUserData {
	return objectUserData{userid, displayname, emailid}
}

func createObjectConnectionData(endpoint string, host string, secretid string, secretkey string) objectConnectionData {
	return objectConnectionData{endpoint, host, secretid, secretkey}
}

//CreateSmokeTestClient constructor for creating instance of smoke test client
func CreateSmokeTestClient(platform enums.RookPlatformType, k8sversion string) (*TestHelper, error) {

	rc, err := getRookClient(platform)

	return &TestHelper{platform: platform,
		rookclient: rc,
		k8sHelp:    utils.CreatK8sHelper(),
		k8sVersion: k8sversion}, err

}

func getRookClient(platform enums.RookPlatformType) (*clients.TestClient, error) {
	return clients.CreateTestClient(platform)

}

//GetBlockClient returns a pointer to block client to perform block operations
func (h *TestHelper) GetBlockClient() contracts.BlockOperator {
	return h.rookclient.GetBlockClient()
}

//GetFileSystemClient returns a pointer to fileSystem client to perform filesSystem operations
func (h *TestHelper) GetFileSystemClient() contracts.FileSystemOperator {
	return h.rookclient.GetFileSystemClient()
}

//GetObjectClient returns a pointer to object store client to perform object store operations
func (h *TestHelper) GetObjectClient() contracts.ObjectOperator {
	return h.rookclient.GetObjectClient()
}

func getDataPath(file string) string {
	return filepath.Join(os.Getenv("GOPATH"), dataPath, file)
}

func (h *TestHelper) getBlockTestData() (blockTestData, error) {
	switch h.platform {
	case enums.Kubernetes:
		return blockTestData{"block-test",
			1048576,
			"/tmp/rook1",
			getDataPath("smoke/pool_pvc.yaml"),
			getDataPath("smoke/block_mount.yaml")}, nil
	case enums.StandAlone:
		return blockTestData{"block-test",
			1048576,
			"/tmp/rook1",
			"",
			""}, nil
	default:
		return blockTestData{}, fmt.Errorf("Unsupported Rook Platform Type")

	}
}

func (h *TestHelper) getFileTestData() (fileTestData, error) {
	switch h.platform {
	case enums.Kubernetes:
		return fileTestData{"testfs",
			"/tmp/rookfs",
			"file-test",
			getDataPath("smoke/file_mount.tmpl")}, nil
	case enums.StandAlone:
		return fileTestData{"testfs",
			"/tmp/rookfs",
			"", ""}, nil
	default:
		return fileTestData{}, fmt.Errorf("Unsupported Rook Platform Type")
	}
}

func (h *TestHelper) getObjectStoreUserData() objectUserData {
	return objectUserData{"rook-user", "A rook RGW user", ""}
}
func (h *TestHelper) getRGWExtenalSevDef() string {
	return getDataPath("smoke/rgw_external.yaml")
}

//CreateBlockStorage function creates a block store
func (h *TestHelper) CreateBlockStorage() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		blockData, _ := h.getBlockTestData()
		if strings.EqualFold(h.k8sVersion, "v1.5") {
			h.k8sHelp.ResourceOperation("create", getDataPath("smoke/pool_sc_1_5.yaml"))

		} else {
			h.k8sHelp.ResourceOperation("create", getDataPath("smoke/pool_sc.yaml"))
		}
		// see https://github.com/rook/rook/issues/767
		time.Sleep(10 * time.Second)
		return h.k8sHelp.ResourceOperation("create", blockData.pvcDefPath)

	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}

//WaitUntilPVCIsBound waits untill PVC is in bound state and returs true if the state is transitisioned to
//Bound state, or returns false
func (h *TestHelper) WaitUntilPVCIsBound() bool {

	return h.k8sHelp.WaitUntilPVCIsBound("block-pv-claim")
}

//MountBlockStorage function mounts a rook created block storage
func (h *TestHelper) MountBlockStorage() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		blockData, _ := h.getBlockTestData()
		_, err := h.GetBlockClient().BlockMap(blockData.podDefPath, blockData.mountpath)
		if err != nil {
			return "MOUNT UNSUCCESSFUL", err
		}
		if h.k8sHelp.IsPodRunning(blockData.name) {

			return "MOUNT SUCCESSFUL", nil
		}
		return "MOUNT UNSUCCESSFUL", fmt.Errorf("Cannot mount block storage")

	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}

//WriteToBlockStorage function writes a block to rook provisioned block store
func (h *TestHelper) WriteToBlockStorage(data string, filename string) (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		blockData, _ := h.getBlockTestData()
		return h.GetBlockClient().BlockWrite(blockData.name,
			blockData.mountpath, data, filename, "")
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}

//ReadFromBlockStorage function Reads a block from rook provisioned block store
func (h *TestHelper) ReadFromBlockStorage(filename string) (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		blockData, _ := h.getBlockTestData()
		return h.GetBlockClient().BlockRead(blockData.name,
			blockData.mountpath, filename, "")
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}

//UnMountBlockStorage function unmounts Block storage
func (h *TestHelper) UnMountBlockStorage() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		blockData, _ := h.getBlockTestData()
		_, err := h.GetBlockClient().BlockUnmap(blockData.podDefPath, blockData.mountpath)
		if err != nil {
			return "UNMOUNT UNSUCCESSFUL", err
		}
		if h.k8sHelp.IsPodTerminated(blockData.name) {
			return "UNMOUNT SUCCESSFUL", nil
		}
		return "UNMOUNT UNSUCCESSFUL", fmt.Errorf("Cannot unmount block storage")

	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}

//DeleteBlockStorage function deletes block storage
func (h *TestHelper) DeleteBlockStorage() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		blockData, _ := h.getBlockTestData()
		// Delete storage pool, storage class and pvc
		if strings.EqualFold(h.k8sVersion, "v1.5") {
			h.k8sHelp.ResourceOperation("delete", getDataPath("smoke/pool_sc_1_5.yaml"))

		} else {
			h.k8sHelp.ResourceOperation("delete", getDataPath("smoke/pool_sc.yaml"))
		}
		return h.k8sHelp.ResourceOperation("delete", blockData.pvcDefPath)
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}

//CleanUpDymanicBlockStorage is helper method to clean up bock storage created by tests
func (h *TestHelper) CleanUpDymanicBlockStorage() {
	switch h.platform {
	case enums.Kubernetes:
		// Delete storage pool, storage class and pvc
		blockImagesList, _ := h.GetBlockClient().BlockList()
		for _, blockImage := range blockImagesList {
			h.rookclient.GetRestAPIClient().DeleteBlockImage(blockImage)

		}

	}
}

//CreateFileStorage function creates a file system
func (h *TestHelper) CreateFileStorage() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		fileTestData, _ := h.getFileTestData()
		// Create storage pool, storage class and pvc
		return h.GetFileSystemClient().FSCreate(fileTestData.name)
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}

//MountFileStorage function mounts a file system
func (h *TestHelper) MountFileStorage() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		fileTestData, _ := h.getFileTestData()
		mons, err := h.k8sHelp.GetMonitorPods()
		if err != nil {
			return "MOUNT UNSUCCESSFUL", err
		}
		ip1, _ := h.k8sHelp.GetMonIP(mons[0])
		ip2, _ := h.k8sHelp.GetMonIP(mons[1])
		ip3, _ := h.k8sHelp.GetMonIP(mons[2])

		config := map[string]string{
			"mon0": ip1,
			"mon1": ip2,
			"mon2": ip3,
		}

		// Create pod that has has file sytem mounted
		_, err = h.k8sHelp.ResourceOperationFromTemplate("create", fileTestData.podDefPath, config)
		if err != nil {
			return "MOUNT UNSUCCESSFUL", err
		}
		if h.k8sHelp.IsPodRunningInNamespace(fileTestData.podname) {
			return "MOUNT SUCCESSFUL", nil
		}
		return "MOUNT UNSUCCESSFUL", fmt.Errorf("Cannot mount File storage")
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}

//WriteToFileStorage functions creates a new file on rook file system
func (h *TestHelper) WriteToFileStorage(data string, filename string) (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		fileTestData, _ := h.getFileTestData()
		return h.GetFileSystemClient().FSWrite(fileTestData.podname,
			fileTestData.mountpath, data, filename, "rook")
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}

//ReadFromFileStorage function get file content from a rook file storage
func (h *TestHelper) ReadFromFileStorage(filename string) (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		fileTestData, _ := h.getFileTestData()
		return h.GetFileSystemClient().FSRead(fileTestData.podname,
			fileTestData.mountpath, filename, "rook")
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}

//UnmountFileStorage funxtion unmounts a file system
func (h *TestHelper) UnmountFileStorage() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		fileTestData, _ := h.getFileTestData()
		mons, err := h.k8sHelp.GetMonitorPods()
		if err != nil {
			return "UNMOUNT UNSUCCESSFUL", err
		}
		ip1, _ := h.k8sHelp.GetMonIP(mons[0])
		ip2, _ := h.k8sHelp.GetMonIP(mons[1])
		ip3, _ := h.k8sHelp.GetMonIP(mons[2])

		config := map[string]string{
			"mon0": ip1,
			"mon1": ip2,
			"mon2": ip3,
		}
		// Create pod that has has file sytem mounted
		_, err = h.k8sHelp.ResourceOperationFromTemplate("delete", fileTestData.podDefPath, config)
		if err != nil {
			return "UNMOUNT UNSUCCESSFUL", err
		}
		if h.k8sHelp.IsPodTerminatedInNamespace(fileTestData.podname) {
			return "UNMOUNT SUCCESSFUL", nil
		}
		return "UNMOUNT UNSUCCESSFUL", fmt.Errorf("Cannot unmount File storage")
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}

//DeleteFileStorage deletes File system
func (h *TestHelper) DeleteFileStorage() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		fileTestData, _ := h.getFileTestData()
		// Create storage pool, storage class and pvc
		return h.GetFileSystemClient().FSDelete(fileTestData.name)
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}

//CreateObjectStore function creates a object store
func (h *TestHelper) CreateObjectStore() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		h.GetObjectClient().ObjectCreate()
		time.Sleep(time.Second * 2) //wait for rgw service to to started
		if h.k8sHelp.IsServiceUpInNameSpace("rook-ceph-rgw") {
			_, err := h.k8sHelp.GetService("rgw-external")
			if err != nil {
				h.k8sHelp.ResourceOperation("create", h.getRGWExtenalSevDef())
				if !h.k8sHelp.IsServiceUpInNameSpace("rgw-external") {
					return "Couldn't create object store ", fmt.Errorf("Cannot expose rgw servie for external user")
				}

			}

			return "OJECT STORE CREATED", nil
		}
		return "Couldn't create object store ", fmt.Errorf("Cannot create object store")
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")
	}
}

//CreateObjectStoreUser function creates a new object store user
func (h *TestHelper) CreateObjectStoreUser() (string, error) {
	objectUserdata := h.getObjectStoreUserData()
	_, err := h.GetObjectClient().ObjectCreateUser(objectUserdata.userID, objectUserdata.displayname)
	if err != nil {
		return "USER NOT CREATED", fmt.Errorf("User not created for object store  %+v", err)
	}
	return "USER CREATED ", nil
}

//GetObjectStoreUsers returns all useres
func (h *TestHelper) GetObjectStoreUsers() ([]model.ObjectUser, error) {
	return h.GetObjectClient().ObjectListUser()

}

//GetObjectStoreUser function returns a user object with matching userId
func (h *TestHelper) GetObjectStoreUser(userid string) (*model.ObjectUser, error) {
	return h.GetObjectClient().ObjectGetUser(userid)

}

//GetObjectStoreConnection function returns connection information about object store
func (h *TestHelper) GetObjectStoreConnection() (*model.ObjectStoreConnectInfo, error) {
	return h.GetObjectClient().ObjectConnection()

}

//GetObjectStoreBucketList function returns list of buckets for a user
func (h *TestHelper) GetObjectStoreBucketList() ([]model.ObjectBucket, error) {
	return h.GetObjectClient().ObjectBucketList()

}

//DeleteObjectStoreUser function deletes rgw object store user
func (h *TestHelper) DeleteObjectStoreUser() error {
	objectUserdata := h.getObjectStoreUserData()
	return h.GetObjectClient().ObjectDeleteUser(objectUserdata.userID)

}

//GetRGWServiceURL returns URL of ceph RGW service in the cluster
func (h *TestHelper) GetRGWServiceURL() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		hostip, err := h.k8sHelp.GetPodHostID("rook-ceph-rgw", "rook")
		if err != nil {
			panic(fmt.Errorf("RGW pods not found/object store possibly not started"))
		}
		endpoint := hostip + ":30001"
		return endpoint, err
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}
