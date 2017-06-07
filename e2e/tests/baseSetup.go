package tests

import (
	"fmt"
	"github.com/rook/rook/e2e/framework/enums"
	"github.com/rook/rook/e2e/framework/manager"
	"github.com/rook/rook/e2e/framework/objects"
	"strings"
)

var (
	env      objects.EnvironmentManifest
	Platform enums.RookPlatformType
)

//One init function per package - initializes Rook infra and installs rook(if needed based on flags)
func init() {
	env = objects.NewManifest()
	var err error

	rookPlatform, err := enums.GetRookPlatFormTypeFromString(env.Platform)
	if err != nil {
		panic(fmt.Errorf("Cannot get platform", err))
	}

	k8sVersion, _ := enums.GetK8sVersionFromString(env.K8sVersion)

	rookTag := env.RookTag
	if err != nil {
		panic(fmt.Errorf("Rook Tag is required", err))
	}

	err, rookInfra := rook_test_infra.GetRookTestInfraManager(rookPlatform, true, k8sVersion)
	if err != nil {
		panic(fmt.Errorf("Error during Rook Infra Setup", err))
	}
	skipRookInstall := strings.EqualFold(env.SkipInstallRook, "true")
	rookInfra.ValidateAndSetupTestPlatform(skipRookInstall)

	err = rookInfra.InstallRook(rookTag, skipRookInstall)
	if err != nil {
		panic(fmt.Errorf("Error during Rook Infra Setup", err))
	}
	Platform = rookInfra.GetRookPlatform()
}
