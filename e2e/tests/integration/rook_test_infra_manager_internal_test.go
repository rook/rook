package integration

import (
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
	"github.com/dangula/rook/e2e/rook-test-framework/managers"
	"testing"

)



func TestDockerSetup_test(t *testing.T) {

	_, rookInfraMgr := managers.GetRookTestInfraManager(enums.Kubernetes, true, enums.V1dot5)

	rookInfraMgr.ValidateAndSetupTestPlatform()

	err, _ := rookInfraMgr.InstallRook("quay.io/rook/rookd:master-latest")

	if err != nil {
		t.Error(err)
		return
	}
}
