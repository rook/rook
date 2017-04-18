package integration

import (
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
	"github.com/dangula/rook/e2e/rook-test-framework/rook-infra-manager"
	"testing"
	"strings"
	"github.com/dangula/rook/e2e/rook-test-framework/objects"
)



func TestSetupPlatform_test(t *testing.T) {
	envManifest := objects.NewManifest()
	var rookTag = "quay.io/rook/rookd:master-latest"

	if !strings.EqualFold(envManifest.RookTag, "") {
		rookTag = envManifest.RookTag
	}

	_, rookInfraMgr := rook_infra_manager.GetRookTestInfraManager(enums.Kubernetes, true, enums.V1dot6)

	rookInfraMgr.ValidateAndSetupTestPlatform()

	err, _ := rookInfraMgr.InstallRook(rookTag)

	if err != nil {
		t.Error(err)
		return
	}
}
