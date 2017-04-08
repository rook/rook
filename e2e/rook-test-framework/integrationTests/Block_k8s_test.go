package integrationTests

import (
	"github.com/dangula/rook/e2e/rook-test-framework/clients"
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
	"testing"
)

var (
	PVC_PATH	="../TestData/podDefinitions/smoke_pool_sc_pvc.yaml"
	POD_PATH        = "../TestData/podDefinitions/smoke_block_mount.yaml"
	POD_VOLUME_PATH = "/tmp/rook-volume4"
	POD_DATA        = "Block test Smoke test"
)

// - This is jsut a sample test with no asserts
func TestK8sRookBlockoperations(t *testing.T) {
	t.Log("Test Create,Mount and Unmount Block")
	rc, _ := clients.CreateRook_Client(enums.Kubernetes)
	rbc := rc.Get_Block_client()
	t.Log("Sample k8s rook block tesst")

	t.Log("Step 1 : List  Block")
	blocklist1, _ := rbc.Block_List()
	t.Log(blocklist1)

	//TODO: NOTE - I have hardcoded the mons in the pvc uyaml, need to figure out how to do it dynamically
	t.Log("Step 2 : Create PVC and")
	createMsg, _ := rbc.Block_Create(PVC_PATH, 0)
	t.Log(createMsg)

	blocklist2, _ := rbc.Block_List()
	t.Log(blocklist2)

	t.Log("Step 3 : Delete PVC and list block")
	deleteMsg, _ := rbc.Block_Delete(PVC_PATH)
	t.Log(deleteMsg)

	blocklist3, _ := rbc.Block_List()
	t.Log(blocklist3)
	//TODO - open client and explicitely delete the image eg rook block delete --name somename --pool-name replicapool

}
