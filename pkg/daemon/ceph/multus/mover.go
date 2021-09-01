/*
Copyright 2022 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package multus

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/vishvananda/netlink"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/workqueue"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	MoverControllerName = "multus-mover"
	HolderAppLabel      = "csi-multus-holder"

	// MovedNetworkInterfacePrefix is the prefix to use for interfaces that are moved by this mover.
	// NOTE: The practical limit for interface name length in linux is 13-15 chars depending on what
	// kernel or DNS bugs may be present in the cluster.
	MovedNetworkInterfacePrefix = "rookm"
)

type Mover struct {
	ClusterContext *clusterd.Context
	Namespace      string
	NodeName       string
}

func (m *Mover) Run(ctx context.Context) error {
	// A simple controller-runtime controller allows us to easily watch for events on holder pods in
	// the namespace without worrying about implementing any of the caching/informer logic
	mgr, err := controllerruntime.NewManager(controllerruntime.GetConfigOrDie(), manager.Options{
		LeaderElection:     false,
		Namespace:          m.Namespace, // limit watch to namespace
		MetricsBindAddress: "0",         // disable metrics so ports don't collide on host
	})
	if err != nil {
		return errors.Wrapf(err, "failed to create manager for %q controller", MoverControllerName)
	}

	movedInterfaceCache := NewMovedInterfaceCache()
	var networkMutex sync.Mutex

	reconciler := &HolderPodReconciler{
		Client:       mgr.GetClient(),
		NetworkMutex: &networkMutex,
		MovedCache:   movedInterfaceCache,
	}

	err = controllerruntime.
		NewControllerManagedBy(mgr).
		Named(MoverControllerName).
		For(&corev1.Pod{}). // limit watch to pods
		WithEventFilter(predicateAcceptOnlyHolderPods(m.NodeName)).
		WithOptions(controller.Options{
			// retry at least every 10 seconds so we don't make CSI wait longer than that in the
			// worst case, but don't bother retrying more often than once every 1/2 second
			RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(500*time.Millisecond, 10*time.Second),
		}).
		Complete(reconciler)
	if err != nil {
		return errors.Wrapf(err, "failed to create %q controller", MoverControllerName)
	}

	ctx, cancelCtx := context.WithCancel(ctx)
	defer cancelCtx()

	// perform an initial best-effort clean-up in case a previous mover instance died without cleaning up
	cleanupAllInterfacesWithPrefix(MovedNetworkInterfacePrefix)

	// before starting the controller, register a routine to clean up when we return
	// importantly, this will clean up even if the routine panics
	defer func() {
		logger.Info("removing multus interfaces from host network namespace before terminating mover process")

		// Stop any ongoing reconciles (important if this is a cleanup due to a panic)
		cancelCtx()

		// treat the full cleanup as an atomic action that should block the reconciler from
		// creating more interface copies during teardown. If we get this lock, we can be sure there
		// is no current reconcile copying interfaces, and because the context is canceled we
		// can be sure there will be no subsequent reconciles after we unlock.
		networkMutex.Lock()
		defer networkMutex.Unlock()

		for holderPodName, info := range movedInterfaceCache.AsMap() {
			logger.Infof("un-moving interface for holder pod %q with info: %+v", holderPodName, info)

			if err := cleanUpMovedInterface(holderPodName, movedInterfaceCache); err != nil {
				logger.Errorf("failed to clean up interface %q for holder pod %q during termination; "+
					"continuing despite failure. %v", info.NetIface, holderPodName, err)
			}
		}

		// clean up any remaining interfaces that might not have been cleaned up for some reason
		cleanupAllInterfacesWithPrefix(MovedNetworkInterfacePrefix)
	}()

	// NOTE: this blocks until the context is canceled
	err = mgr.Start(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to start %q controller", MoverControllerName)
	}

	return nil
}

type HolderPodReconciler struct {
	client.Client
	NetworkMutex *sync.Mutex // use during network move/cleanup operations to avoid races
	MovedCache   *MovedInterfaceCache
}

func (r *HolderPodReconciler) Reconcile(ctx context.Context, req controllerruntime.Request) (controllerruntime.Result, error) {
	// wrap the reconcile so we can log errors with Rook's logger
	res, err := r.reconcile(ctx, req)
	if err != nil {
		logger.Errorf("reconcile failed for pod %q. %s", req.Name, err.Error())
		if !res.IsZero() {
			// if there is an error and the result is nonzero, pass the result to controller-runtime
			// with a nil-ed error so that we can delay re-reconciles a bit even in the error case
			err = nil
		}
	}
	return res, err
}

func (r *HolderPodReconciler) reconcile(ctx context.Context, req controllerruntime.Request) (controllerruntime.Result, error) {
	if ctx.Err() != nil {
		logger.Infof("not reconciling holder pod %q since reconciliation is canceled", req.Name)
		return controllerruntime.Result{}, nil // canceled, nothing more to do
	}

	pod := &corev1.Pod{}
	err := r.Get(ctx, req.NamespacedName, pod)
	if err != nil {
		if kerrors.IsNotFound(err) {
			//
			// clean up interface of deleted pod
			//
			logger.Infof("holder pod %q was deleted; ensuring its interface is cleaned up", req.Name)
			return r.cleanUpMovedInterface(req.Name)

		}
		return controllerruntime.Result{},
			errors.Wrapf(err, "failed to get kubernetes pod resource for holder pod %q", req.Name)
	}
	if pod.ObjectMeta.DeletionTimestamp != nil {
		//
		// clean up interface of pod marked for deletion
		//
		logger.Infof("holder pod %q is marked for deletion; ensuring its interface is cleaned up", req.Name)
		return r.cleanUpMovedInterface(pod.Name)
	}

	//
	// move interface
	//
	logger.Infof("ensuring interface for holder pod %q is moved to the host network namespace", pod.Name)
	return r.ensureCopiedInterface(ctx, pod)
}

// should be idempotent and good at making sure interfaces exist, so don't skip any steps if the
// holder pod is already in the moved interface cache
func (r *HolderPodReconciler) ensureCopiedInterface(ctx context.Context, holderPod *corev1.Pod) (controllerruntime.Result, error) {
	holderIP, err := GetHolderPodIP(holderPod)
	if err != nil {
		return HolderPodNotReadyResult, // holder pod likely doesn't have networks attached yet
			errors.Wrapf(err, "failed to get ip of holder pod %q", holderPod.Name)
	}

	multusNet, err := GetHolderPodMultusNetwork(holderPod)
	if err != nil {
		return HolderPodNotReadyResult, // holder pod likely doesn't have networks attached yet
			errors.Wrapf(err, "failed to get networks attached to holder pod %q", holderPod.Name)
	}
	holderMultusIface := multusNet.Interface

	// let the interface move be treated as an atomic action
	r.NetworkMutex.Lock()
	defer r.NetworkMutex.Unlock()

	// if we got the lock but the process is being canceled, bail out before the atomic move op
	if ctx.Err() != nil {
		logger.Infof("abandoning reconciliation for holder pod %q after acquiring network lock", holderPod.Name)
		return controllerruntime.Result{}, nil // canceled, nothing more to do
	}

	// we end up printing the same info a lot in error/info messages, so keep the common known info
	knownInfo := fmt.Sprintf("holder pod %q, ip %q", holderPod.Name, holderIP)

	// Using the pod's CNI IP as a reference, find the network namespace of the pod
	holderNs, err := FindNetworkNamespaceWithIP(holderIP)
	if err != nil {
		return controllerruntime.Result{},
			errors.Wrapf(err, "failed to determine network namespace of holder pod: %s", knownInfo)
	}
	logger.Infof("found network namespace %q: %s", holderNs.Path(), knownInfo)

	knownInfo = fmt.Sprintf("holder pod %q, ip %q, net namespace %q, multus interface %q",
		holderPod.Name, holderIP, holderNs.Path(), holderMultusIface)

	// Get the network config (ip address, etc.) of the multus interface in the holder namespace
	// so that we can set the "moved" interface's IP address appropriately once it exists
	netConfig, err := GetNetworkConfig(holderNs, holderMultusIface)
	if err != nil {
		return controllerruntime.Result{},
			errors.Wrapf(err, "failed to get network config of multus interface: %s", knownInfo)
	}
	if netConfig.Addrs == nil || len(netConfig.Addrs) < 1 {
		return controllerruntime.Result{}, errors.Errorf("failed to find addresses for interface: %s", knownInfo)
	}
	knownInfo = fmt.Sprintf("holder pod %q, ip %q, mac %q, net namespace %q, multus interface %q",
		holderPod.Name, holderIP, netConfig.LinkAttrs.HardwareAddr.String(), holderNs.Path(), holderMultusIface)
	logger.Infof("found network address and route info for multus interface: %s\t address: %+v\t route:%+v",
		knownInfo, netConfig.Addrs, netConfig.Routes)

	// Get a handle to the host namespace
	hostNs, err := netExec.GetCurrentNS()
	if err != nil {
		return controllerruntime.Result{}, errors.Wrap(err, "failed to determine host network namespace")
	}

	// Determine if there is already a copy of the interface in the host ns
	existingIfaceCopy, err := FindInterfaceWithHardwareAddr(hostNs, netConfig.LinkAttrs.HardwareAddr)
	if err != nil {
		return controllerruntime.Result{},
			errors.Wrapf(err, "failed to determine (by MAC address) if the multus interface was already moved to the host net namespace: %s", knownInfo)
	}
	if existingIfaceCopy != "" && !strings.HasPrefix(existingIfaceCopy, MovedNetworkInterfacePrefix) {
		return controllerruntime.Result{},
			errors.Errorf("failure: unsupported scenario. this is not a macvlan setup. "+
				"existing interface %q with the same MAC address as the holder interface exists on the host, but it is not a moved interface: %s",
				existingIfaceCopy, knownInfo)
	}
	movedIface := existingIfaceCopy

	// Only create a copy of the interface if one doesn't already exist in the host net namespace
	if movedIface == "" {
		// Generate the name of the moved interface based on the holder pod's name
		newIface, err := DetermineNextInterface(MovedNetworkInterfacePrefix)
		if err != nil {
			return controllerruntime.Result{},
				errors.Wrapf(err, "failed to determine an available interface with prefix %q in host net namespace", MovedNetworkInterfacePrefix)
		}
		movedIface = newIface

		// Instead of moving the interface to the host namespace, we actually disable the interface
		// by setting it "down" and then make a copy in the host network namespace. This way, if
		// there is a corner case where the interface can't be moved back to the holder pod's
		// namespace during cleanup, the original holder pod interface will still exist as the
		// primary reference for future reconciles.
		if err := DisableInterface(holderNs, holderMultusIface); err != nil {
			return controllerruntime.Result{},
				errors.Wrapf(err, "failed to disable multus interface: %s", knownInfo)
		}

		err = CopyInterfaceToHostNamespace(holderNs, hostNs, holderMultusIface, newIface)
		if err != nil {
			return controllerruntime.Result{}, errors.Wrapf(err, "failed to copy interface to host net namespace: %s", knownInfo)
		}
	} else {
		logger.Infof("interface %q was already moved to the host network namespace; still ensuring it is configured: %s", movedIface, knownInfo)
	}

	// Ensure the "moved" interface has the same network config as the original, and set interface "up"
	err = ConfigureInterface(hostNs, movedIface, &netConfig)
	if err != nil {
		if existingIfaceCopy == "" {
			err2 := DeleteInterface(hostNs, movedIface)
			if err2 != nil {
				logger.Errorf("failed to delete moved interface copy %q. %v", movedIface, err)
			}
		}
		return controllerruntime.Result{}, errors.Wrapf(err, "failed to (re)configure moved interface %q: %s", movedIface, knownInfo)
	}

	r.MovedCache.Add(holderPod.Name, MovedInterfaceInfo{NetIface: movedIface})

	logger.Infof("successfully moved holder interface to host network namespace as %q: %s", movedIface, knownInfo)
	return controllerruntime.Result{}, nil
}

func (r *HolderPodReconciler) cleanUpMovedInterface(holderPodName string) (controllerruntime.Result, error) {
	// let the interface cleanup be treated as an atomic action
	r.NetworkMutex.Lock()
	defer r.NetworkMutex.Unlock()

	// if context is canceled here, we may as well still continue the reconcile attempt to
	// clean up the interface since we'll need to do that anyway

	if err := cleanUpMovedInterface(holderPodName, r.MovedCache); err != nil {
		return controllerruntime.Result{},
			errors.Wrapf(err, "failed to clean up interface for holder pod %q", holderPodName)
	}

	return controllerruntime.Result{}, nil
}

// this version of the function can be reused outside of the reconciler (i.e., the main process cleanup)
// calling function must manage the mutex lock to make this an atomic action
func cleanUpMovedInterface(holderPodName string, movedCache *MovedInterfaceCache) error {
	info, err := movedCache.Get(holderPodName)
	if err != nil {
		return nil // no cleanup to do
	}

	// Get a handle to the host namespace
	hostNs, err := netExec.GetCurrentNS()
	if err != nil {
		return errors.Wrap(err, "failed to determine host network namespace")
	}

	if err := DeleteInterface(hostNs, info.NetIface); err != nil {
		return errors.Wrapf(err, "failed to delete moved interface %q for holder pod %q", info.NetIface, holderPodName)
	}

	movedCache.Remove(holderPodName)

	logger.Infof("successfully cleaned up moved interface %q for holder pod %q", info.NetIface, holderPodName)
	return nil
}

func predicateAcceptOnlyHolderPods(nodeName string) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if !isHolderPodOnNode(e.Object, nodeName) {
				return false
			}
			logger.Infof("holder pod %q was created on node %q", e.Object.GetName(), nodeName)
			return true
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			if !isHolderPodOnNode(e.ObjectNew, nodeName) {
				return false
			}
			logger.Infof("holder pod %q was updated on node %q", e.ObjectNew.GetName(), nodeName)
			return true
		},

		DeleteFunc: func(e event.DeleteEvent) bool {
			if !isHolderPodOnNode(e.Object, nodeName) {
				return false
			}
			logger.Infof("holder pod %q was deleted on node %q", e.Object.GetName(), nodeName)
			return true
		},

		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

func isHolderPodOnNode(o client.Object, nodeName string) bool {
	pod, ok := o.(*corev1.Pod)
	if !ok {
		logger.Warningf("got object %+v that is not a pod", o)
		return false
	}

	if pod.Spec.NodeName != nodeName {
		return false // pod is not on this mover's node
	}

	labels := pod.GetLabels()
	if labels == nil {
		return false // if labels are nil, it can't possibly be a holder
	}
	app, ok := labels[k8sutil.AppAttr]
	if ok && app == HolderAppLabel {
		return true // is holder pod
	}

	return false
}

func cleanupAllInterfacesWithPrefix(prefix string) {
	// this is used for cleanup only, which we want to attempt even if there are errors
	// we would normally use netNS.Do() here, but to minimize the chance of errors, just start
	// in the current namespace which should be the host anyway

	ifaces, err := net.Interfaces()
	if err != nil {
		logger.Errorf("failed to get interfaces in host net namespace during cleanup")
	}

	// TODO: this will clobber moved interfaces for other CephClusters if we allow multiple clusters
	for _, iface := range ifaces {
		if strings.HasPrefix(iface.Name, prefix) {
			logger.Warningf("cleaning up interface %q which still exists", iface.Name)

			link, err := netlink.LinkByName(iface.Name)
			if err != nil {
				logger.Errorf("failed to clean up interface %q which still exists. %v", iface.Name, err)
				continue
			}

			// this might help identify which holder pod the interface originally came from
			logger.Warningf("still-existing interface %q has hardware address %q", iface.Name, link.Attrs().HardwareAddr.String())

			err = netlink.LinkDel(link)
			if err != nil {
				logger.Errorf("failed to clean up interface %q which still exists. %v", iface.Name, err)
			}
		}
	}
}
