// Package controllers contains all the controller-runtime controllers and
// exports a method for registering them all with a manager.

package controllers

import (
	"fmt"

	"github.com/rook/rook/pkg/operator/ceph/controllers/controllerconfig"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager (entrypoint for controller)
var AddToManagerFuncs = []func(manager.Manager, *controllerconfig.Options) error{}

// AddToManager adds all the registered controllers to the passed manager.
// each controller package will have an Add method listed in AddToManagerFuncs
// which will setup all the necessary watch
func AddToManager(m manager.Manager, o *controllerconfig.Options) error {
	if o == nil {
		return fmt.Errorf("nil controllerconfig passed")
	}
	for _, f := range AddToManagerFuncs {
		if err := f(m, o); err != nil {
			return err
		}
	}
	return nil
}
