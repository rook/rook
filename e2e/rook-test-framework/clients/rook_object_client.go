package clients

import (
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
)

type rookObjectClient struct {
	transportClient contracts.ITransportClient
}

func CreateRookObjectClient(client contracts.ITransportClient) *rookObjectClient {
	return &rookObjectClient{transportClient: client}

}

func (r *rookObjectClient) CreateObjectStorage() (string, error) {
	cmd := []string{"exec -n rook rook-client rook object create"}
	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error creating  Object storage")
	}
}

func (r *rookObjectClient) UserList() (string, error) {
	cmd := []string{"exec -n rook rook-client rook object user list"}

	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error listing object users")
	}
}

func (r *rookObjectClient) UserCreate(userId string, displayName string) (string, error) {
	cmd := []string{"exec -n rook rook-client -- rook object user create " + userId + " " + displayName}

	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error creating user")
	}
}

func (r *rookObjectClient) UserGet(userId string) (string, error) {
	cmd := []string{"exec -n rook rook-client -- rook object user get " + userId}
	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error creating user")
	}
}

func (r *rookObjectClient) UserDelete(userId string) (string, error) {
	cmd := []string{"exec -n rook rook-client -- rook object user delete " + userId}

	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error creating user")
	}
}

func (r *rookObjectClient) BucketList() (string, error) {
	cmd := []string{"exec -n rook rook-client -- rook object bucket list"}

	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error creating user")
	}
}
func (r *rookObjectClient) ConnectionInfo(userId string, formatFlag string) (string, error) {
	cmd := []string{"exec -n rook rook-client -- rook object connection " + userId + " --format " + formatFlag}

	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error creating user")
	}
}
