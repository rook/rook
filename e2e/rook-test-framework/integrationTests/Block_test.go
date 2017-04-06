package integrationTests

import (
	"github.com/dangula/rook/e2e/rook-test-framework/transport"
	"github.com/quantum/rook-client-helpers/clients"
	"github.com/stretchr/testify/assert"
	"testing"
)

const BLOCK_NAME = "test4"
const VOLUME_PATH = "/tmp/rook-volume4"
const FILENAME = "file1"
const DATA = "Block test Smoke test"

func TestBlockClient(t *testing.T) {
	t.Log("Test Create,Mount and Unmount Block")
	rookBlockClient := clients.CreateRookBlockClient(transport.CreateNewk8sTransportClient())

	t.Log("Step 1 : Creating Block")
	if create, error := rookBlockClient.Create(BLOCK_NAME, 1000000); error != nil {
		t.Log(create)
		t.Errorf("Expected Block to be created")
		t.Fail()
	} else {
		t.Log(create)
	}
	t.Log("Step 2 : Mount Block")
	if mount, error := rookBlockClient.Mount(BLOCK_NAME, VOLUME_PATH); error != nil {
		t.Log(mount)
		t.Errorf("Expected Block to be Mounted")
		t.Fail()

	} else {
		t.Log(mount)
	}
	t.Log("Step 3 : write a file to block volume")
	if write, error := rookBlockClient.Write(DATA, VOLUME_PATH, FILENAME); error != nil {
		t.Log(write)
		t.Log(error)
		t.Errorf("Expected file to be created on the block")
		t.Fail()

	} else {
		t.Log(write)
	}

	t.Log("Step 4 : Read a file to block volume")
	if read, error := rookBlockClient.Read(VOLUME_PATH, FILENAME); error != nil {
		t.Log(read)
		t.Errorf("Expected file to be read from the block")
		t.Fail()

	} else {
		t.Log(read)
		//assert.Equal(t, DATA, read, "make sure the content of the file is unchanged")
		assert.Contains(t, read, DATA, "make sure content of the files is unchanged")
	}

	t.Log("Step 5 : Unmount Block")
	if mount, error := rookBlockClient.UnMount(VOLUME_PATH); error != nil {
		t.Log(mount)
		t.Errorf("Expected Block to be UnMounted")
		t.Fail()

	} else {
		t.Log(mount)
	}

}
