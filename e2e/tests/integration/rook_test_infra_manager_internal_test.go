package integration

import (
	"testing"
	"github.com/dangula/rook/e2e/rook-test-framework/managers"
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
)

func TestDockerSetup(t *testing.T) {
	_, rookInfraMgr := managers.GetRookTestInfraManager(enums.Kubernetes, true)

	error := rookInfraMgr.ValidateAndPrepareEnvironment()

	if error != nil {
		t.Error(error.Error())
	}


}
