package client

import (
	"fmt"

	"github.com/rook/rook/pkg/clusterd"
)

// MgrEnableModule Enable module for mgr
func MgrEnableModule(context *clusterd.Context, clusterName, name string, force bool) error {
	args := []string{"mgr", "module", "enable", name}
	if force {
		args = append(args, "--force")
	}
	_, err := ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to mgr module enable for %s: %+v", name, err)
	}

	return nil
}
