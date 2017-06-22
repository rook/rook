package smoke

import (
	"fmt"
	"time"

	"github.com/rook/rook/e2e/framework/clients"
	"github.com/rook/rook/e2e/framework/contracts"
	"github.com/rook/rook/e2e/framework/enums"
	"github.com/rook/rook/e2e/framework/utils"
	"github.com/rook/rook/pkg/model"
)

type SmokeTestHelper struct {
	platform   enums.RookPlatformType
	rookclient *clients.TestClient
	k8sHelp    *utils.K8sHelper
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
	userId      string
	displayname string
	emailId     string
}

type objectConnectionData struct {
	awsEndpoint        string
	awsHost            string
	awsSecretKeyId     string
	awsSecretAccessKey string
}

func createObjectUserData(userid string, displayname string, emailid string) objectUserData {
	return objectUserData{userid, displayname, emailid}
}

func createObjectConnectionData(endpoint string, host string, secretid string, secretkey string) objectConnectionData {
	return objectConnectionData{endpoint, host, secretid, secretkey}
}

func CreateSmokeTestClient(platform enums.RookPlatformType) (*SmokeTestHelper, error) {

	rc, err := getRookClient(platform)

	return &SmokeTestHelper{platform: platform,
		rookclient: rc,
		k8sHelp:    utils.CreatK8sHelper()}, err

}

func getRookClient(platform enums.RookPlatformType) (*clients.TestClient, error) {
	return clients.CreateTestClient(platform)

}
func (h *SmokeTestHelper) GetBlockClient() contracts.BlockOperator {
	return h.rookclient.GetBlockClient()
}

func (h *SmokeTestHelper) GetFileSystemClient() contracts.FileSystemOperator {
	return h.rookclient.GetFileSystemClient()
}

func (h *SmokeTestHelper) GetObjectClient() contracts.ObjectOperator {
	return h.rookclient.GetObjectClient()
}

func (h *SmokeTestHelper) getBlockTestData() (blockTestData, error) {
	switch h.platform {
	case enums.Kubernetes:
		return blockTestData{"block-test",
			1048576,
			"/tmp/rook1",
			"../../data/smoke/pool_sc_pvc.yaml",
			"../../data/smoke/block_mount.yaml"}, nil
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

func (h *SmokeTestHelper) getFileTestData() (fileTestData, error) {
	switch h.platform {
	case enums.Kubernetes:
		return fileTestData{"testfs",
			"/tmp/rookfs",
			"file-test",
			"../../data/smoke/file_mount.tmpl"}, nil
	case enums.StandAlone:
		return fileTestData{"testfs",
			"/tmp/rookfs",
			"", ""}, nil
	default:
		return fileTestData{}, fmt.Errorf("Unsupported Rook Platform Type")
	}
}

func (h *SmokeTestHelper) getObjectStoreUserData() objectUserData {
	return objectUserData{"rook-user", "A rook RGW user", ""}
}
func (h *SmokeTestHelper) getRGWExtenalSevDef() string {
	return "../../data/smoke/rgw_external.yaml"
}

func (h *SmokeTestHelper) CreateBlockStorage() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		blockData, _ := h.getBlockTestData()
		// Create storage pool, storage class and pvc
		return h.k8sHelp.ResourceOperation("create", blockData.pvcDefPath)
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}

func (h *SmokeTestHelper) MountBlockStorage() (string, error) {
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

func (h *SmokeTestHelper) WriteToBlockStorage(data string, filename string) (string, error) {
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

func (h *SmokeTestHelper) ReadFromBlockStorage(filename string) (string, error) {
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

func (h *SmokeTestHelper) UnMountBlockStorage() (string, error) {
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

func (h *SmokeTestHelper) DeleteBlockStorage() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		blockData, _ := h.getBlockTestData()
		// Delete storage pool, storage class and pvc
		return h.k8sHelp.ResourceOperation("delete", blockData.pvcDefPath)
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}

func (h *SmokeTestHelper) CleanUpDymanicBlockStorge() {
	switch h.platform {
	case enums.Kubernetes:
		// Delete storage pool, storage class and pvc
		blockImagesList, _ := h.GetBlockClient().BlockList()
		for _, blockImage := range blockImagesList {
			h.rookclient.GetRestAPIClient().DeleteBlockImage(blockImage)

		}

	}
}

func (h *SmokeTestHelper) CreateFileStorage() (string, error) {
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

func (h *SmokeTestHelper) MountFileStorage() (string, error) {
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

func (h *SmokeTestHelper) WriteToFileStorage(data string, filename string) (string, error) {
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

func (h *SmokeTestHelper) ReadFromFileStorage(filename string) (string, error) {
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

func (h *SmokeTestHelper) UnmountFileStorage() (string, error) {
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

func (h *SmokeTestHelper) DeleteFileStorage() (string, error) {
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

func (h *SmokeTestHelper) CreateObjectStore() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		h.GetObjectClient().ObjectCreate()
		time.Sleep(time.Second * 2) //wait for rgw service to to started
		if h.k8sHelp.IsServiceUpInNameSpace("rook-ceph-rgw") {
			_, err := h.k8sHelp.GetService("rgw-external")
			if err != nil {
				h.k8sHelp.ResourceOperation("create", h.getRGWExtenalSevDef())
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

func (h *SmokeTestHelper) CreateObjectStoreUser() (string, error) {
	objectUserdata := h.getObjectStoreUserData()
	_, err := h.GetObjectClient().ObjectCreateUser(objectUserdata.userId, objectUserdata.displayname)
	if err != nil {
		return "USER NOT CREATED", fmt.Errorf("User not created for object store")
	}
	return "USER CREATED ", nil
}

func (h *SmokeTestHelper) GetObjectStoreUsers() ([]model.ObjectUser, error) {
	return h.GetObjectClient().ObjectListUser()

}

func (h *SmokeTestHelper) GetObjectStoreUser(userid string) (*model.ObjectUser, error) {
	return h.GetObjectClient().ObjectGetUser(userid)

}

func (h *SmokeTestHelper) GetObjectStoreConnection() (*model.ObjectStoreConnectInfo, error) {
	return h.GetObjectClient().ObjectConnection()

}

func (h *SmokeTestHelper) GetObjectStoreBucketList() ([]model.ObjectBucket, error) {
	return h.GetObjectClient().ObjectBucketList()

}

func (h *SmokeTestHelper) DeleteObjectStoreUser() error {
	objectUserdata := h.getObjectStoreUserData()
	return h.GetObjectClient().ObjectDeleteUser(objectUserdata.userId)

}

func (h *SmokeTestHelper) GetRGWServiceUrl() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		hostip, err := h.k8sHelp.GetPodHostId("rook-ceph-rgw", "rook")
		if err != nil {
			panic(fmt.Errorf("RGW pods not found/object store possibly not started"))
		}
		return hostip + ":30001", err
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}
