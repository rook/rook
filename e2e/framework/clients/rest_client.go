package clients

import (
	"fmt"
	"net/http"

	"github.com/rook/rook/e2e/framework/enums"
	"github.com/rook/rook/e2e/framework/utils"
	"github.com/rook/rook/pkg/model"
	rclient "github.com/rook/rook/pkg/rook/client"
)

type RestAPIClient struct {
	rrc *rclient.RookNetworkRestClient
}

//Create Rook REST API client
func CreateRestAPIClient(platform enums.RookPlatformType) *RestAPIClient {
	var endpoint string
	switch {
	case platform == enums.Kubernetes:
		rkh := utils.CreatK8sHelper()
		//Start rook_api_external server via nodePort if not it not already running.
		_, err := rkh.GetService("rook-api-external")
		if err != nil {
			rkh.ResourceOperation("create", "../../data/smoke/rook_api_external.yaml")
		}
		apiIp, err := rkh.GetPodHostId("rook-api", "rook")
		if err != nil {
			panic(fmt.Errorf("Host Ip for Rook-api service not found"))
		}
		endpoint = "http://" + apiIp + ":30002"
	case platform == enums.StandAlone:
		endpoint = "http://localhost:8124"
	default:
		panic(fmt.Errorf("platfrom type %s not yet supported", platform))
	}

	client := rclient.NewRookNetworkRestClient(endpoint, http.DefaultClient)

	return &RestAPIClient{client}
}

func (a *RestAPIClient) URL() string {
	return a.rrc.RestURL
}

func (a *RestAPIClient) GetNodes() ([]model.Node, error) {
	return a.rrc.GetNodes()
}

func (a *RestAPIClient) GetPools() ([]model.Pool, error) {
	return a.rrc.GetPools()
}

func (a *RestAPIClient) CreatePool(pool model.Pool) (string, error) {
	return a.rrc.CreatePool(pool)
}

func (a *RestAPIClient) GetBlockImages() ([]model.BlockImage, error) {
	return a.rrc.GetBlockImages()
}
func (a *RestAPIClient) CreateBlockImage(image model.BlockImage) (string, error) {
	return a.rrc.CreateBlockImage(image)

}
func (a *RestAPIClient) DeleteBlockImage(image model.BlockImage) (string, error) {
	return a.rrc.DeleteBlockImage(image)
}
func (a *RestAPIClient) GetClientAccessInfo() (model.ClientAccessInfo, error) {
	return a.rrc.GetClientAccessInfo()
}
func (a *RestAPIClient) GetFilesystems() ([]model.Filesystem, error) {
	return a.rrc.GetFilesystems()
}
func (a *RestAPIClient) CreateFilesystem(fsmodel model.FilesystemRequest) (string, error) {
	return a.rrc.CreateFilesystem(fsmodel)
}
func (a *RestAPIClient) DeleteFilesystem(fsmodel model.FilesystemRequest) (string, error) {
	return a.rrc.DeleteFilesystem(fsmodel)
}
func (a *RestAPIClient) GetStatusDetails() (model.StatusDetails, error) {
	return a.rrc.GetStatusDetails()
}
func (a *RestAPIClient) CreateObjectStore() (string, error) {
	return a.rrc.CreateObjectStore()
}
func (a *RestAPIClient) GetObjectStoreConnectionInfo() (*model.ObjectStoreConnectInfo, error) {
	return a.rrc.GetObjectStoreConnectionInfo()
}
func (a *RestAPIClient) ListBuckets() ([]model.ObjectBucket, error) {
	return a.rrc.ListBuckets()
}
func (a *RestAPIClient) ListObjectUsers() ([]model.ObjectUser, error) {
	return a.rrc.ListObjectUsers()
}
func (a *RestAPIClient) GetObjectUser(id string) (*model.ObjectUser, error) {
	return a.rrc.GetObjectUser(id)
}
func (a *RestAPIClient) CreateObjectUser(user model.ObjectUser) (*model.ObjectUser, error) {
	return a.rrc.CreateObjectUser(user)
}
func (a *RestAPIClient) UpdateObjectUser(user model.ObjectUser) (*model.ObjectUser, error) {
	return a.rrc.UpdateObjectUser(user)

}
func (a *RestAPIClient) DeleteObjectUser(id string) error {
	return a.rrc.DeleteObjectUser(id)

}
