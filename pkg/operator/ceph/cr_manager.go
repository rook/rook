package operator

import (
	"github.com/coreos/pkg/capnslog"

	"github.com/rook/rook/pkg/operator/ceph/controllers"
	"github.com/rook/rook/pkg/operator/ceph/controllers/controllerconfig"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var mgrLog = capnslog.NewPackageLogger("github.com/rook/rook", "op-controller-runtime")

func (o *Operator) startManager(stopCh <-chan struct{}) {

	// Set up a manager
	mgrOpts := manager.Options{
		LeaderElection: false,
	}

	mgrLog.Info("setting up manager")
	mgr, err := manager.New(o.context.KubeConfig, mgrOpts)
	if err != nil {
		mgrLog.Errorf("unable to set up overall controller manager: %+v", err)
		return
	}
	// options to pass to the controllers
	controllerOpts := &controllerconfig.Options{
		Context:           o.context,
		OperatorNamespace: o.operatorNamespace,
	}

	// Add the registered controllers to the manager (entrypoint for controllers)
	err = controllers.AddToManager(mgr, controllerOpts)
	if err != nil {
		mgrLog.Errorf("ControllerOptinons not passed to controllers")
	}

	mgrLog.Info("starting manager")
	if err := mgr.Start(stopCh); err != nil {
		mgrLog.Errorf("unable to run manager: %+v", err)
		return
	}
}
