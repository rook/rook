package integration

import (
	"testing"
	"github.com/dangula/rook/e2e/rook-test-framework/managers"
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
)

func TestDockerSetup_test(t *testing.T) {
	_, rookInfraMgr := managers.GetRookTestInfraManager(enums.Kubernetes, true)

	error := rookInfraMgr.ValidateAndPrepareEnvironment()

	if error != nil {
		t.Error(error.Error())
		return
	}

	err, _ := rookInfraMgr.InstallRook("quay.io/rook/rookd:master-latest")


	if err != nil {
		t.Error(error.Error())
		return
	}
}
