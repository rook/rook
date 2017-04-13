package smokeTest

import (
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/clients"
	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
	"github.com/dangula/rook/e2e/rook-test-framework/utils"
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

type objetUserData struct {
	userId      string
	displayname string
	emailId     string
}

type objctConnectionData struct {
	aws_endpoint          string
	aws_host              string
	aws_secret_key_id     string
	aws_secret_access_key string
}

func createObjectUserData(userid string, displayname string, emailid string) objetUserData {
	return objetUserData{userid, displayname, emailid}
}

func createObjectConnectionData(endpoint string, host string, secretid string, secretkey string) objctConnectionData {
	return objctConnectionData{endpoint, host, secretid, secretkey}
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
func (integrationTestClient *SmokeTestHelper) GetBlockClient() contracts.Irook_block {
	return integrationTestClient.rookclient.Get_Block_client()
}

func (integrationTestClient *SmokeTestHelper) GetFileSystemClient() contracts.Irook_filesystem {
	return integrationTestClient.rookclient.Get_FileSystem_client()
}

func (integrationTestClient *SmokeTestHelper) GetObjectClient() contracts.Irook_object {
	return integrationTestClient.rookclient.Get_Object_client()
}

func (integrationClient *SmokeTestHelper) getBlockTestData() (blockTestData, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		return blockTestData{"block-test",
			1048576,
			"/tmp/rook1",
			"../../../pod-specs/test-data/smoke-test-data/smoke_pool_sc_pvc.tmpl",
			"../../../pod-specs/test-data/smoke-test-data/smoke_block_mount.yaml"}, nil
	case enums.StandAlone:
		return blockTestData{"block-test",
			1048576,
			"/tmp/rook1",
			"",
			""}, nil
	default:
		return blockTestData{}, errors.New("Unsupported Rook Platform Type")

	}
}

func (integrationClient *SmokeTestHelper) getFileTestData() (fileTestData, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		return fileTestData{"testfs",
			"/tmp/rookfs",
			"file-test",
			"../../../pod-specs/test-data/smoke-test-data/smoke_file_mount.tmpl"}, nil
	case enums.StandAlone:
		return fileTestData{"testfs",
			"/tmp/rookfs",
			"", ""}, nil
	default:
		return fileTestData{}, errors.New("Unsupported Rook Platform Type")
	}
}

func (integrationClient *SmokeTestHelper) getObjectStoreUserData() objetUserData {
	return objetUserData{"rook-user", "A rook RGW user", ""}
}

func (integrationClient *SmokeTestHelper) CreateBlockStorage() (string, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		blockData, _ := integrationClient.getBlockTestData()
		// Create storage pool, storage class and pvc
		return integrationClient.k8sHelp.CreatePodFromTemplate(blockData.pvcDefPath)
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", errors.New("NOT YET IMPLEMENTED")
	default:
		return "", errors.New("Unsupported Rook Platform Type")

	}
}

func (integrationClient *SmokeTestHelper) MountBlockStorage() (string, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		blockData, _ := integrationClient.getBlockTestData()
		_, err := integrationClient.GetBlockClient().Block_Map(blockData.podDefPath, blockData.mountpath)
		if err != nil {
			return "MOUNT UNSUCCESSFUL", err
		}
		if integrationClient.k8sHelp.IsPodRunning(blockData.name) {
			return "MOUNT SUCCESSFUL", nil
		}
		return "MOUNT UNSUCCESSFUL", errors.New("Cannot mount block storage")

	case enums.StandAlone:
		return "NEED TO IMPLEMENT", errors.New("NOT YET IMPLEMENTED")
	default:
		return "", errors.New("Unsupported Rook Platform Type")

	}
}

func (integrationClient *SmokeTestHelper) WriteToBlockStorage(data string, filename string) (string, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		blockData, _ := integrationClient.getBlockTestData()
		return integrationClient.GetBlockClient().Block_Write(blockData.name,
			blockData.mountpath, data, filename, "")
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", errors.New("NOT YET IMPLEMENTED")
	default:
		return "", errors.New("Unsupported Rook Platform Type")

	}
}

func (integrationClient *SmokeTestHelper) ReadFromBlockStorage(filename string) (string, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		blockData, _ := integrationClient.getBlockTestData()
		return integrationClient.GetBlockClient().Block_Read(blockData.name,
			blockData.mountpath, filename, "")
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", errors.New("NOT YET IMPLEMENTED")
	default:
		return "", errors.New("Unsupported Rook Platform Type")

	}
}

func (integrationClient *SmokeTestHelper) UnMountBlockStorage() (string, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		blockData, _ := integrationClient.getBlockTestData()
		_, err := integrationClient.GetBlockClient().Block_Unmap(blockData.podDefPath, blockData.mountpath)
		if err != nil {
			return "UNMOUNT UNSUCCESSFUL", err
		}
		if integrationClient.k8sHelp.IsPodTerminated(blockData.name) {
			return "UNMOUNT SUCCESSFUL", nil
		}
		return "UNMOUNT UNSUCCESSFUL", errors.New("Cannot unmount block storage")

	case enums.StandAlone:
		return "NEED TO IMPLEMENT", errors.New("NOT YET IMPLEMENTED")
	default:
		return "", errors.New("Unsupported Rook Platform Type")

	}
}

func (integrationClient *SmokeTestHelper) DeleteBlockStorage() (string, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		blockData, _ := integrationClient.getBlockTestData()
		// Delete storage pool, storage class and pvc
		return integrationClient.k8sHelp.DeletePodFromTemplate(blockData.pvcDefPath)
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", errors.New("NOT YET IMPLEMENTED")
	default:
		return "", errors.New("Unsupported Rook Platform Type")

	}
}

func (integrationClient *SmokeTestHelper) CleanUpDymanicBlockStorge() {
	switch integrationClient.platform {
	case enums.Kubernetes:
		// Delete storage pool, storage class and pvc
		blocklistraw, _ := integrationClient.GetBlockClient().Block_List()
		blocklistMap := integrationClient.rookHelp.ParseBlockListData(blocklistraw)
		integrationClient.k8sHelp.CleanUpDymaincCreatedPVC(blocklistMap)
	}
}

func (integrationClient *SmokeTestHelper) CreateFileStorage() (string, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		fileTestData, _ := integrationClient.getFileTestData()
		// Create storage pool, storage class and pvc
		return integrationClient.GetFileSystemClient().FS_Create(fileTestData.name)
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", errors.New("NOT YET IMPLEMENTED")
	default:
		return "", errors.New("Unsupported Rook Platform Type")

	}
}

func (integrationClient *SmokeTestHelper) MountFileStorage() (string, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		fileTestData, _ := integrationClient.getFileTestData()
		// Create pod that has has file sytem mounted
		_, err := integrationClient.k8sHelp.CreatePodFromTemplate(fileTestData.podDefPath)
		if err != nil {
			return "MOUNT UNSUCCESSFUL", err
		}
		if integrationClient.k8sHelp.IsPodRunningInNamespace(fileTestData.podname) {
			return "MOUNT SUCCESSFUL", nil
		}
		return "MOUNT UNSUCCESSFUL", errors.New("Cannot mount File storage")
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", errors.New("NOT YET IMPLEMENTED")
	default:
		return "", errors.New("Unsupported Rook Platform Type")

	}
}

func (integrationClient *SmokeTestHelper) WriteToFileStorage(data string, filename string) (string, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		fileTestData, _ := integrationClient.getFileTestData()
		return integrationClient.GetFileSystemClient().FS_Write(fileTestData.podname,
			fileTestData.mountpath, data, filename, "rook")
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", errors.New("NOT YET IMPLEMENTED")
	default:
		return "", errors.New("Unsupported Rook Platform Type")

	}
}

func (integrationClient *SmokeTestHelper) ReadFromFileStorage(filename string) (string, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		fileTestData, _ := integrationClient.getFileTestData()
		return integrationClient.GetFileSystemClient().FS_Read(fileTestData.podname,
			fileTestData.mountpath, filename, "rook")
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", errors.New("NOT YET IMPLEMENTED")
	default:
		return "", errors.New("Unsupported Rook Platform Type")

	}
}

func (integrationClient *SmokeTestHelper) UnmountFileStorage() (string, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		fileTestData, _ := integrationClient.getFileTestData()
		// Create pod that has has file sytem mounted
		_, err := integrationClient.k8sHelp.DeletePodFromTemplate(fileTestData.podDefPath)
		if err != nil {
			return "UNMOUNT UNSUCCESSFUL", err
		}
		if integrationClient.k8sHelp.IsPodTerminatedInNamespace(fileTestData.podname) {
			return "UNMOUNT SUCCESSFUL", nil
		}
		return "UNMOUNT UNSUCCESSFUL", errors.New("Cannot unmount File storage")
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", errors.New("NOT YET IMPLEMENTED")
	default:
		return "", errors.New("Unsupported Rook Platform Type")

	}
}

func (integrationClient *SmokeTestHelper) DeleteFileStorage() (string, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		fileTestData, _ := integrationClient.getFileTestData()
		// Create storage pool, storage class and pvc
		return integrationClient.GetFileSystemClient().FS_Delete(fileTestData.name)
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", errors.New("NOT YET IMPLEMENTED")
	default:
		return "", errors.New("Unsupported Rook Platform Type")

	}
}

func (integrationClient *SmokeTestHelper) CreateObjectStore() (string, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		_, err := integrationClient.GetObjectClient().Object_Create()
		if err != nil {
			return "Couldn't create object store ", err
		}
		if integrationClient.k8sHelp.IsServiceUpInNameSpace("rgw") {
			return "OJECT STORE CREATED", nil
		}
		return "Couldn't create object store ", errors.New("Cannot create object store")
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", errors.New("NOT YET IMPLEMENTED")
	default:
		return "", errors.New("Unsupported Rook Platform Type")
	}
}

func (integrationClient *SmokeTestHelper) CreateObjectStoreUser() (string, error) {
	objectuserdata := integrationClient.getObjectStoreUserData()
	_, err := integrationClient.GetObjectClient().Object_Create_user(objectuserdata.userId, objectuserdata.displayname)
	if err != nil {
		return "USER NOT CREATED", errors.New("User not created for object store")
	}
	return "USER CREATED ", nil
}

func (integrationClient *SmokeTestHelper) GetObjectStoreUsers() (map[string]utils.ObjectUserListData, error) {
	rawdata, err := integrationClient.GetObjectClient().Object_List_user()
	if err != nil {
		return nil, err
	} else {
		return integrationClient.rookHelp.ParserObjectUserListData(rawdata), nil
	}

}

func (integrationClient *SmokeTestHelper) GetObjectStoreUser(userid string) (utils.ObjectUserData, error) {
	rawdata, err := integrationClient.GetObjectClient().Object_Get_user(userid)
	if err != nil {
		return utils.ObjectUserData{}, err
	} else {
		return integrationClient.rookHelp.ParserObjectUserData(rawdata), nil
	}

}

func (integrationClient *SmokeTestHelper) GetObjectStoreConnection(userid string) (utils.ObjectConnectionData, error) {
	rawdata, err := integrationClient.GetObjectClient().Object_Connection(userid)
	if err != nil {
		return utils.ObjectConnectionData{}, err
	} else {
		return integrationClient.rookHelp.ParserObjectConnectionData(rawdata), nil
	}

}

func (integrationClient *SmokeTestHelper) GetObjectStoreBucketList() (map[string]utils.ObjectBucketListData, error) {
	rawdata, err := integrationClient.GetObjectClient().Object_Bucket_list()
	if err != nil {
		return nil, err
	} else {
		return integrationClient.rookHelp.ParserObjectBucketListData(rawdata), nil
	}
}

func (integrationClient *SmokeTestHelper) DeleteObjectStoreUser() (string, error) {
	objectuserdata := integrationClient.getObjectStoreUserData()
	_, err := integrationClient.GetObjectClient().Object_Delete_user(objectuserdata.userId)
	if err != nil {
		return "USER NOT DELETED", errors.New("User not deleted for object store")
	}
	return "USER DELETED ", nil
}

func (integrationClient *SmokeTestHelper) GetRgwPort() (string, error) {
	switch integrationClient.platform {
	case enums.Kubernetes:
		rawdata, err := integrationClient.k8sHelp.GetService("rgw")
		if err != nil {
			return "port not found", err
		}
		return integrationClient.rookHelp.GetRgwServiceNodePort(rawdata)
	case enums.StandAlone:
		return "NEED TO IMPLEMENT", errors.New("NOT YET IMPLEMENTED")
	default:
		return "", errors.New("Unsupported Rook Platform Type")

	}
}
