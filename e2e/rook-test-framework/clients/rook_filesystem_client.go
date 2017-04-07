package clients

import (
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
)

type rookFileSystemClient struct {
	transportClient contracts.ITransportClient
}

func CreateRookFileSystemClient(client contracts.ITransportClient) *rookFileSystemClient {

	return &rookFileSystemClient{transportClient: client}

}

func (r *rookFileSystemClient) List() (string, error) {
	cmd := []string{"exec", "-n", "rook", "rook-client", "rook", "filesystem", "ls"}
	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error listing Blocks")
	}
}

func (r *rookFileSystemClient) Create(name string) (string, error) {
	cmd := []string{"exec -n rook rook-client -- rook filesystem create --name " + name}

	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error creating block")
	}
}

func (r *rookFileSystemClient) Delete(name string) (string, error) {
	cmd := []string{"exec -n rook rook-client -- rook filesystem delete --name " + name}

	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error creating block")
	}
}

func (r *rookFileSystemClient) Mount(name string, path string) (string, error) {
	cmd := []string{"exec -n rook rook-client -- rook filesystem mount --name " + name + " --path " + path}

	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error Mounting block")
	}
}

func (r *rookFileSystemClient) UnMount(path string) (string, error) {
	cmd := []string{"exec -n rook rook-client -- rook filesystem unmount --path " + path}

	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error UnMonuting block")
	}
}
