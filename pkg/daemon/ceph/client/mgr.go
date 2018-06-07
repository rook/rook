package client

import (
	"fmt"

	"github.com/rook/rook/pkg/clusterd"
)

// MgrEnableModule enables a mgr module
func MgrEnableModule(context *clusterd.Context, clusterName, name string, force bool) error {
	return enableModule(context, clusterName, name, force, "enable")
}

// MgrDisableModule disables a mgr module
func MgrDisableModule(context *clusterd.Context, clusterName, name string) error {
	return enableModule(context, clusterName, name, false, "disable")
}

func enableModule(context *clusterd.Context, clusterName, name string, force bool, action string) error {
	args := []string{"mgr", "module", action, name}
	if force {
		args = append(args, "--force")
	}
	_, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to mgr module enable for %s: %+v", name, err)
	}

	return nil
}
