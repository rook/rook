package clients

import (
	"errors"
	"fmt"
	"strconv"
	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
)

type rookBlockClient struct {
	transportClient contracts.ITransportClient
}

func CreateRookBlockClient(client contracts.ITransportClient) *rookBlockClient {

	return &rookBlockClient{transportClient: client}
}

func (r *rookBlockClient) List() (string, error) {
	cmd := []string{"exec", "-n", "rook", "rook-client", "rook", "block", "ls"}
	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error listing Blocks")
	}

}

func (r *rookBlockClient) Mount(name string, path string) (string, error) {
	cmd := []string{"exec", "-n", "rook", "rook-client", "--", "rook", "block", "map", "--name", name, "--format", "--mount", path}

	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error Mounting block")
	}
}

func (r *rookBlockClient) UnMount(path string) (string, error) {
	cmd := []string{"exec", "-n", "rook", "rook-client", "--", "rook", "block", "unmap", "--mount", path}

	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error UnMonuting block")
	}
}

func (r *rookBlockClient) Create(name string, size int) (string, error) {
	cmd := []string{"exec", "-n", "rook", "rook-client", "--", "rook", "block", "create", "--name", name, "--size", strconv.Itoa(size)}

	out, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return out, nil
	} else {
		return err, errors.New("Error creating block")
	}
}

func (r *rookBlockClient) Write(data string, path string, filename string) (string, error) {
	//cmd := "exec -n rook rook-client -- /bin/bash -c echo \"test\" > "+path+"/"+filename
	//cmd := "exec -n rook rook-client -- echo \"test\" > "+path+"/"+filename
	//cmd := []string{"exec", "-n", "rook", "rook-client", "--", "bash","-c", "\"echo test>" + path + "/" + filename + "\""}
	wt := "echo \"" + data + "\">" + path + "/" + filename
	fmt.Println(wt)
	cmd := []string{"exec", "-it", "-n", "rook", "rook-client", "--", "bash", "-c", wt}
	wr, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return wr, nil
	} else {
		return err, errors.New("Error writing file to block volume path")
	}

}

func (r *rookBlockClient) Read(path string, filename string) (string, error) {
	//cmd := "exec -n rook rook-client -- cat "+path+"/"+filename
	rd := path + "/" + filename
	fmt.Println(rd)
	cmd := []string{"exec", "-n", "rook", "rook-client", "--", "cat", rd}

	rd, err, status := r.transportClient.Execute(cmd)
	if status == 0 {
		return rd, nil
	} else {
		return err, errors.New("Error reading file to block volume path")
	}

}
