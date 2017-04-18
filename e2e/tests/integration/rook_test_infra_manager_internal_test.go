package integration

import (
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
	"github.com/dangula/rook/e2e/rook-test-framework/rook-infra-manager"
	"testing"
	"github.com/dangula/rook/e2e/.glide/cache/src/https-github.com-dangula-rook/e2e/objects"
	"strings"
)



func TestSetupPlatform_test(t *testing.T) {
	envManifest := objects.NewManifest()
	var rookTag = "quay.io/rook/rookd:master-latest"

	if !strings.EqualFold(envManifest.RookTag, "") {
		rookTag = envManifest.RookTag
	}

	_, rookInfraMgr := rook.GetRookTestInfraManager(enums.Kubernetes, true, enums.V1dot6)

	rookInfraMgr.ValidateAndSetupTestPlatform()

	err, _ := rookInfraMgr.InstallRook(rookTag)

	if err != nil {
		t.Error(err)
		return
	}
}
