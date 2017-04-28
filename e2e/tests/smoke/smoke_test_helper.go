package smoke

import (
	"fmt"
	"github.com/rook/rook/e2e/framework/clients"
	"github.com/rook/rook/e2e/framework/contracts"
	"github.com/rook/rook/e2e/framework/enums"
	"github.com/rook/rook/e2e/framework/utils"
)

type SmokeTestHelper struct {
	platform   enums.RookPlatformType
	rookclient *clients.RookClient
	rookHelp   *utils.RookHelper
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
	aws_endpoint          string
	aws_host              string
	aws_secret_key_id     string
	aws_secret_access_key string
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
		rookHelp:   utils.CreateRookHelper(),
		k8sHelp:    utils.CreatK8sHelper()}, err

}

func getRookClient(platform enums.RookPlatformType) (*clients.RookClient, error) {
	return clients.CreateRook_Client(platform)

}
func (h *SmokeTestHelper) GetBlockClient() contracts.IRookBlock {
	return h.rookclient.GetBlockClient()
}

func (h *SmokeTestHelper) GetFileSystemClient() contracts.IRookFilesystem {
	return h.rookclient.GetFileSystemClient()
}

func (h *SmokeTestHelper) GetObjectClient() contracts.IRookObject {
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
		blocklistraw, _ := h.GetBlockClient().BlockList()
		blocklistMap := h.rookHelp.ParseBlockListData(blocklistraw)
		h.k8sHelp.CleanUpDymaincCreatedPVC(blocklistMap)
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
		// Create pod that has has file sytem mounted
		_, err := h.k8sHelp.ResourceOperationFromTemplate("create", fileTestData.podDefPath)
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
		// Create pod that has has file sytem mounted
		_, err := h.k8sHelp.ResourceOperationFromTemplate("delete", fileTestData.podDefPath)
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
		_, err := h.GetObjectClient().ObjectCreate()
		if err != nil {
			return "Couldn't create object store ", err
		}
		if h.k8sHelp.IsServiceUpInNameSpace("rgw") {
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

func (h *SmokeTestHelper) GetObjectStoreUsers() (map[string]utils.ObjectUserListData, error) {
	rawdata, err := h.GetObjectClient().ObjectListUser()
	if err != nil {
		return nil, err
	} else {
		return h.rookHelp.ParserObjectUserListData(rawdata), nil
	}

}

func (h *SmokeTestHelper) GetObjectStoreUser(userid string) (utils.ObjectUserData, error) {
	rawdata, err := h.GetObjectClient().ObjectGetUser(userid)
	if err != nil {
		return utils.ObjectUserData{}, err
	} else {
		return h.rookHelp.ParserObjectUserData(rawdata), nil
	}

}

func (h *SmokeTestHelper) GetObjectStoreConnection(userid string) (utils.ObjectConnectionData, error) {
	rawdata, err := h.GetObjectClient().ObjectConnection(userid)
	if err != nil {
		return utils.ObjectConnectionData{}, err
	} else {
		return h.rookHelp.ParserObjectConnectionData(rawdata), nil
	}

}

func (h *SmokeTestHelper) GetObjectStoreBucketList() (map[string]utils.ObjectBucketListData, error) {
	rawdata, err := h.GetObjectClient().ObjectBucketList()
	if err != nil {
		return nil, err
	} else {
		return h.rookHelp.ParserObjectBucketListData(rawdata), nil
	}
}

func (h *SmokeTestHelper) DeleteObjectStoreUser() (string, error) {
	objectUserdata := h.getObjectStoreUserData()
	_, err := h.GetObjectClient().ObjectDeleteUser(objectUserdata.userId)
	if err != nil {
		return "USER NOT DELETED", fmt.Errorf("User not deleted for object store")
	}
	return "USER DELETED ", nil
}

func (h *SmokeTestHelper) GetRGWPort() (string, error) {
	switch h.platform {
	case enums.Kubernetes:
		rawdata, err := h.k8sHelp.GetService("rgw-external")
		if err != nil {
			return "port not found", err
		}
		return h.rookHelp.GetRgwServiceNodePort(rawdata)
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", fmt.Errorf("NOT YET IMPLEMENTED")
	default:
		return "", fmt.Errorf("Unsupported Rook Platform Type")

	}
}
