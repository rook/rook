package client

import (
	"fmt"
	"strings"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
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

func MgrSetConfig(context *clusterd.Context, clusterName, cephVersionName, key, val string) (bool, error) {
	var getArgs, setArgs []string

	if cephVersionName == cephv1beta1.Luminous || cephVersionName == "" {
		getArgs = append(getArgs, "config-key", "get", key)
		if val == "" {
			setArgs = append(setArgs, "config-key", "del", key)
		} else {
			setArgs = append(setArgs, "config-key", "set", key, val)
		}
	} else {
		getArgs = append(getArgs, "config", "get", "mgr.", key)
		if val == "" {
			setArgs = append(setArgs, "config", "rm", "mgr", key)
		} else {
			setArgs = append(setArgs, "config", "set", "mgr", key, val)
		}
	}

	// Retrieve previous value to monitor changes
	var prevVal string
	buf, err := ExecuteCephCommand(context, clusterName, getArgs)
	if err == nil {
		prevVal = strings.TrimSpace(string(buf))
	}

	if _, err := ExecuteCephCommand(context, clusterName, setArgs); err != nil {
		return false, fmt.Errorf("failed to set mgr config key %s to \"%s\": %+v", key, val, err)
	}

	hasChanged := prevVal != val
	return hasChanged, nil
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
