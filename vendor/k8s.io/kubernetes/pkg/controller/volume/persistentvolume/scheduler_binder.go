/*
Copyright 2017 The Kubernetes Authors.

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

package persistentvolume

import (
	"fmt"
	"sort"

	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	coreinformers "k8s.io/client-go/informers/core/v1"
	storageinformers "k8s.io/client-go/informers/storage/v1"
	clientset "k8s.io/client-go/kubernetes"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
	"k8s.io/kubernetes/pkg/features"
	volumeutil "k8s.io/kubernetes/pkg/volume/util"
)

// SchedulerVolumeBinder is used by the scheduler to handle PVC/PV binding
// and dynamic provisioning.  The binding decisions are integrated into the pod scheduling
// workflow so that the PV NodeAffinity is also considered along with the pod's other
// scheduling requirements.
//
// This integrates into the existing default scheduler workflow as follows:
// 1. The scheduler takes a Pod off the scheduler queue and processes it serially:
//    a. Invokes all predicate functions, parallelized across nodes.  FindPodVolumes() is invoked here.
//    b. Invokes all priority functions.  Future/TBD
//    c. Selects the best node for the Pod.
//    d. Cache the node selection for the Pod. (Assume phase)
//       i.  If PVC binding is required, cache in-memory only:
//           * Updated PV objects for prebinding to the corresponding PVCs.
//           * For the pod, which PVs need API updates.
//           AssumePodVolumes() is invoked here.  Then BindPodVolumes() is called asynchronously by the
//           scheduler.  After BindPodVolumes() is complete, the Pod is added back to the scheduler queue
//           to be processed again until all PVCs are bound.
//       ii. If PVC binding is not required, cache the Pod->Node binding in the scheduler's pod cache,
//           and asynchronously bind the Pod to the Node.  This is handled in the scheduler and not here.
// 2. Once the assume operation is done, the scheduler processes the next Pod in the scheduler queue
//    while the actual binding operation occurs in the background.
type SchedulerVolumeBinder interface {
	// FindPodVolumes checks if all of a Pod's PVCs can be satisfied by the node.
	//
	// If a PVC is bound, it checks if the PV's NodeAffinity matches the Node.
	// Otherwise, it tries to find an available PV to bind to the PVC.
	//
	// It returns true if all of the Pod's PVCs have matching PVs or can be dynamic provisioned,
	// and returns true if bound volumes satisfy the PV NodeAffinity.
	//
	// This function is called by the volume binding scheduler predicate and can be called in parallel
	FindPodVolumes(pod *v1.Pod, node *v1.Node) (unboundVolumesSatisified, boundVolumesSatisfied bool, err error)

	// AssumePodVolumes will:
	// 1. Take the PV matches for unbound PVCs and update the PV cache assuming
	// that the PV is prebound to the PVC.
	// 2. Take the PVCs that need provisioning and update the PVC cache with related
	// annotations set.
	//
	// It returns true if all volumes are fully bound, and returns true if any volume binding/provisioning
	// API operation needs to be done afterwards.
	//
	// This function will modify assumedPod with the node name.
	// This function is called serially.
	AssumePodVolumes(assumedPod *v1.Pod, nodeName string) (allFullyBound bool, bindingRequired bool, err error)

	// BindPodVolumes will:
	// 1. Initiate the volume binding by making the API call to prebind the PV
	// to its matching PVC.
	// 2. Trigger the volume provisioning by making the API call to set related
	// annotations on the PVC
	//
	// This function can be called in parallel.
	BindPodVolumes(assumedPod *v1.Pod) error

	// GetBindingsCache returns the cache used (if any) to store volume binding decisions.
	GetBindingsCache() PodBindingCache
}

type volumeBinder struct {
	ctrl *PersistentVolumeController

	pvcCache PVCAssumeCache
	pvCache  PVAssumeCache

	// Stores binding decisions that were made in FindPodVolumes for use in AssumePodVolumes.
	// AssumePodVolumes modifies the bindings again for use in BindPodVolumes.
	podBindingCache PodBindingCache
}

// NewVolumeBinder sets up all the caches needed for the scheduler to make volume binding decisions.
func NewVolumeBinder(
	kubeClient clientset.Interface,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	pvInformer coreinformers.PersistentVolumeInformer,
	storageClassInformer storageinformers.StorageClassInformer) SchedulerVolumeBinder {

	// TODO: find better way...
	ctrl := &PersistentVolumeController{
		kubeClient:  kubeClient,
		classLister: storageClassInformer.Lister(),
	}

	b := &volumeBinder{
		ctrl:            ctrl,
		pvcCache:        NewPVCAssumeCache(pvcInformer.Informer()),
		pvCache:         NewPVAssumeCache(pvInformer.Informer()),
		podBindingCache: NewPodBindingCache(),
	}

	return b
}

func (b *volumeBinder) GetBindingsCache() PodBindingCache {
	return b.podBindingCache
}

// FindPodVolumes caches the matching PVs and PVCs to provision per node in podBindingCache
func (b *volumeBinder) FindPodVolumes(pod *v1.Pod, node *v1.Node) (unboundVolumesSatisfied, boundVolumesSatisfied bool, err error) {
	podName := getPodName(pod)

	// Warning: Below log needs high verbosity as it can be printed several times (#60933).
	glog.V(5).Infof("FindPodVolumes for pod %q, node %q", podName, node.Name)

	// Initialize to true for pods that don't have volumes
	unboundVolumesSatisfied = true
	boundVolumesSatisfied = true

	// The pod's volumes need to be processed in one call to avoid the race condition where
	// volumes can get bound/provisioned in between calls.
	boundClaims, claimsToBind, unboundClaimsImmediate, err := b.getPodVolumes(pod)
	if err != nil {
		return false, false, err
	}

	// Immediate claims should be bound
	if len(unboundClaimsImmediate) > 0 {
		return false, false, fmt.Errorf("pod has unbound PersistentVolumeClaims")
	}

	// Check PV node affinity on bound volumes
	if len(boundClaims) > 0 {
		boundVolumesSatisfied, err = b.checkBoundClaims(boundClaims, node, podName)
		if err != nil {
			return false, false, err
		}
	}

	if len(claimsToBind) > 0 {
		var claimsToProvision []*v1.PersistentVolumeClaim
		unboundVolumesSatisfied, claimsToProvision, err = b.findMatchingVolumes(pod, claimsToBind, node)
		if err != nil {
			return false, false, err
		}

		if utilfeature.DefaultFeatureGate.Enabled(features.DynamicProvisioningScheduling) {
			// Try to provision for unbound volumes
			if !unboundVolumesSatisfied {
				unboundVolumesSatisfied, err = b.checkVolumeProvisions(pod, claimsToProvision, node)
				if err != nil {
					return false, false, err
				}
			}
		}
	}

	return unboundVolumesSatisfied, boundVolumesSatisfied, nil
}

// AssumePodVolumes will take the cached matching PVs and PVCs to provision
// in podBindingCache for the chosen node, and:
// 1. Update the pvCache with the new prebound PV.
// 2. Update the pvcCache with the new PVCs with annotations set
// It will update podBindingCache again with the PVs and PVCs that need an API update.
func (b *volumeBinder) AssumePodVolumes(assumedPod *v1.Pod, nodeName string) (allFullyBound, bindingRequired bool, err error) {
	podName := getPodName(assumedPod)

	glog.V(4).Infof("AssumePodVolumes for pod %q, node %q", podName, nodeName)

	if allBound := b.arePodVolumesBound(assumedPod); allBound {
		glog.V(4).Infof("AssumePodVolumes for pod %q, node %q: all PVCs bound and nothing to do", podName, nodeName)
		return true, false, nil
	}

	assumedPod.Spec.NodeName = nodeName
	// Assume PV
	claimsToBind := b.podBindingCache.GetBindings(assumedPod, nodeName)
	newBindings := []*bindingInfo{}

	for _, binding := range claimsToBind {
		newPV, dirty, err := b.ctrl.getBindVolumeToClaim(binding.pv, binding.pvc)
		glog.V(5).Infof("AssumePodVolumes: getBindVolumeToClaim for pod %q, PV %q, PVC %q.  newPV %p, dirty %v, err: %v",
			podName,
			binding.pv.Name,
			binding.pvc.Name,
			newPV,
			dirty,
			err)
		if err != nil {
			b.revertAssumedPVs(newBindings)
			return false, true, err
		}
		if dirty {
			err = b.pvCache.Assume(newPV)
			if err != nil {
				b.revertAssumedPVs(newBindings)
				return false, true, err
			}

			newBindings = append(newBindings, &bindingInfo{pv: newPV, pvc: binding.pvc})
		}
	}

	// Don't update cached bindings if no API updates are needed.  This can happen if we
	// previously updated the PV object and are waiting for the PV controller to finish binding.
	if len(newBindings) != 0 {
		bindingRequired = true
		b.podBindingCache.UpdateBindings(assumedPod, nodeName, newBindings)
	}

	// Assume PVCs
	claimsToProvision := b.podBindingCache.GetProvisionedPVCs(assumedPod, nodeName)

	newProvisionedPVCs := []*v1.PersistentVolumeClaim{}
	for _, claim := range claimsToProvision {
		// The claims from method args can be pointing to watcher cache. We must not
		// modify these, therefore create a copy.
		claimClone := claim.DeepCopy()
		metav1.SetMetaDataAnnotation(&claimClone.ObjectMeta, annSelectedNode, nodeName)
		err = b.pvcCache.Assume(claimClone)
		if err != nil {
			b.revertAssumedPVs(newBindings)
			b.revertAssumedPVCs(newProvisionedPVCs)
			return
		}

		newProvisionedPVCs = append(newProvisionedPVCs, claimClone)
	}

	if len(newProvisionedPVCs) != 0 {
		bindingRequired = true
		b.podBindingCache.UpdateProvisionedPVCs(assumedPod, nodeName, newProvisionedPVCs)
	}

	return
}

// BindPodVolumes gets the cached bindings and PVCs to provision in podBindingCache
// and makes the API update for those PVs/PVCs.
func (b *volumeBinder) BindPodVolumes(assumedPod *v1.Pod) error {
	podName := getPodName(assumedPod)
	glog.V(4).Infof("BindPodVolumes for pod %q", podName)

	bindings := b.podBindingCache.GetBindings(assumedPod, assumedPod.Spec.NodeName)
	claimsToProvision := b.podBindingCache.GetProvisionedPVCs(assumedPod, assumedPod.Spec.NodeName)

	// Do the actual prebinding. Let the PV controller take care of the rest
	// There is no API rollback if the actual binding fails
	for i, bindingInfo := range bindings {
		glog.V(5).Infof("BindPodVolumes: Pod %q, binding PV %q to PVC %q", podName, bindingInfo.pv.Name, bindingInfo.pvc.Name)
		_, err := b.ctrl.updateBindVolumeToClaim(bindingInfo.pv, bindingInfo.pvc, false)
		if err != nil {
			// only revert assumed cached updates for volumes we haven't successfully bound
			b.revertAssumedPVs(bindings[i:])
			// Revert all of the assumed cached updates for claims,
			// since no actual API update will be done
			b.revertAssumedPVCs(claimsToProvision)
			return err
		}
	}

	// Update claims objects to trigger volume provisioning. Let the PV controller take care of the rest
	// PV controller is expect to signal back by removing related annotations if actual provisioning fails
	for i, claim := range claimsToProvision {
		if _, err := b.ctrl.kubeClient.CoreV1().PersistentVolumeClaims(claim.Namespace).Update(claim); err != nil {
			glog.V(4).Infof("updating PersistentVolumeClaim[%s] failed: %v", getPVCName(claim), err)
			// only revert assumed cached updates for claims we haven't successfully updated
			b.revertAssumedPVCs(claimsToProvision[i:])
			return err
		}
	}

	return nil
}

func getPodName(pod *v1.Pod) string {
	return pod.Namespace + "/" + pod.Name
}

func getPVCName(pvc *v1.PersistentVolumeClaim) string {
	return pvc.Namespace + "/" + pvc.Name
}

func (b *volumeBinder) isVolumeBound(namespace string, vol *v1.Volume, checkFullyBound bool) (bool, *v1.PersistentVolumeClaim, error) {
	if vol.PersistentVolumeClaim == nil {
		return true, nil, nil
	}

	pvcName := vol.PersistentVolumeClaim.ClaimName
	claim := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
		},
	}
	pvc, err := b.pvcCache.GetPVC(getPVCName(claim))
	if err != nil || pvc == nil {
		return false, nil, fmt.Errorf("error getting PVC %q: %v", pvcName, err)
	}

	pvName := pvc.Spec.VolumeName
	if pvName != "" {
		if checkFullyBound {
			if metav1.HasAnnotation(pvc.ObjectMeta, annBindCompleted) {
				glog.V(5).Infof("PVC %q is fully bound to PV %q", getPVCName(pvc), pvName)
				return true, pvc, nil
			} else {
				glog.V(5).Infof("PVC %q is not fully bound to PV %q", getPVCName(pvc), pvName)
				return false, pvc, nil
			}
		}
		glog.V(5).Infof("PVC %q is bound or prebound to PV %q", getPVCName(pvc), pvName)
		return true, pvc, nil
	}

	glog.V(5).Infof("PVC %q is not bound", getPVCName(pvc))
	return false, pvc, nil
}

// arePodVolumesBound returns true if all volumes are fully bound
func (b *volumeBinder) arePodVolumesBound(pod *v1.Pod) bool {
	for _, vol := range pod.Spec.Volumes {
		if isBound, _, _ := b.isVolumeBound(pod.Namespace, &vol, true); !isBound {
			// Pod has at least one PVC that needs binding
			return false
		}
	}
	return true
}

// getPodVolumes returns a pod's PVCs separated into bound (including prebound), unbound with delayed binding,
// and unbound with immediate binding
func (b *volumeBinder) getPodVolumes(pod *v1.Pod) (boundClaims []*v1.PersistentVolumeClaim, unboundClaims []*bindingInfo, unboundClaimsImmediate []*v1.PersistentVolumeClaim, err error) {
	boundClaims = []*v1.PersistentVolumeClaim{}
	unboundClaimsImmediate = []*v1.PersistentVolumeClaim{}
	unboundClaims = []*bindingInfo{}

	for _, vol := range pod.Spec.Volumes {
		volumeBound, pvc, err := b.isVolumeBound(pod.Namespace, &vol, false)
		if err != nil {
			return nil, nil, nil, err
		}
		if pvc == nil {
			continue
		}
		if volumeBound {
			boundClaims = append(boundClaims, pvc)
		} else {
			delayBinding, err := b.ctrl.shouldDelayBinding(pvc)
			if err != nil {
				return nil, nil, nil, err
			}
			if delayBinding {
				// Scheduler path
				unboundClaims = append(unboundClaims, &bindingInfo{pvc: pvc})
			} else {
				// Immediate binding should have already been bound
				unboundClaimsImmediate = append(unboundClaimsImmediate, pvc)
			}
		}
	}
	return boundClaims, unboundClaims, unboundClaimsImmediate, nil
}

func (b *volumeBinder) checkBoundClaims(claims []*v1.PersistentVolumeClaim, node *v1.Node, podName string) (bool, error) {
	for _, pvc := range claims {
		pvName := pvc.Spec.VolumeName
		pv, err := b.pvCache.GetPV(pvName)
		if err != nil {
			return false, err
		}

		err = volumeutil.CheckNodeAffinity(pv, node.Labels)
		if err != nil {
			glog.V(4).Infof("PersistentVolume %q, Node %q mismatch for Pod %q: %v", pvName, node.Name, err.Error(), podName)
			return false, nil
		}
		glog.V(5).Infof("PersistentVolume %q, Node %q matches for Pod %q", pvName, node.Name, podName)
	}

	glog.V(4).Infof("All bound volumes for Pod %q match with Node %q", podName, node.Name)
	return true, nil
}

// findMatchingVolumes tries to find matching volumes for given claims,
// and return unbound claims for further provision.
func (b *volumeBinder) findMatchingVolumes(pod *v1.Pod, claimsToBind []*bindingInfo, node *v1.Node) (foundMatches bool, unboundClaims []*v1.PersistentVolumeClaim, err error) {
	podName := getPodName(pod)
	// Sort all the claims by increasing size request to get the smallest fits
	sort.Sort(byPVCSize(claimsToBind))

	chosenPVs := map[string]*v1.PersistentVolume{}

	foundMatches = true
	matchedClaims := []*bindingInfo{}

	for _, bindingInfo := range claimsToBind {
		// Get storage class name from each PVC
		storageClassName := ""
		storageClass := bindingInfo.pvc.Spec.StorageClassName
		if storageClass != nil {
			storageClassName = *storageClass
		}
		allPVs := b.pvCache.ListPVs(storageClassName)

		// Find a matching PV
		bindingInfo.pv, err = findMatchingVolume(bindingInfo.pvc, allPVs, node, chosenPVs, true)
		if err != nil {
			return false, nil, err
		}
		if bindingInfo.pv == nil {
			glog.V(4).Infof("No matching volumes for Pod %q, PVC %q on node %q", podName, getPVCName(bindingInfo.pvc), node.Name)
			unboundClaims = append(unboundClaims, bindingInfo.pvc)
			foundMatches = false
			continue
		}

		// matching PV needs to be excluded so we don't select it again
		chosenPVs[bindingInfo.pv.Name] = bindingInfo.pv
		matchedClaims = append(matchedClaims, bindingInfo)
		glog.V(5).Infof("Found matching PV %q for PVC %q on node %q for pod %q", bindingInfo.pv.Name, getPVCName(bindingInfo.pvc), node.Name, podName)
	}

	// Mark cache with all the matches for each PVC for this node
	if len(matchedClaims) > 0 {
		b.podBindingCache.UpdateBindings(pod, node.Name, matchedClaims)
	}

	if foundMatches {
		glog.V(4).Infof("Found matching volumes for pod %q on node %q", podName, node.Name)
	}

	return
}

// checkVolumeProvisions checks given unbound claims (the claims have gone through func
// findMatchingVolumes, and do not have matching volumes for binding), and return true
// if all of the claims are eligible for dynamic provision.
func (b *volumeBinder) checkVolumeProvisions(pod *v1.Pod, claimsToProvision []*v1.PersistentVolumeClaim, node *v1.Node) (provisionSatisfied bool, err error) {
	podName := getPodName(pod)
	provisionedClaims := []*v1.PersistentVolumeClaim{}

	for _, claim := range claimsToProvision {
		className := v1helper.GetPersistentVolumeClaimClass(claim)
		if className == "" {
			return false, fmt.Errorf("no class for claim %q", getPVCName(claim))
		}

		class, err := b.ctrl.classLister.Get(className)
		if err != nil {
			return false, fmt.Errorf("failed to find storage class %q", className)
		}
		provisioner := class.Provisioner
		if provisioner == "" || provisioner == notSupportedProvisioner {
			glog.V(4).Infof("storage class %q of claim %q does not support dynamic provisioning", className, getPVCName(claim))
			return false, nil
		}

		// Check if the node can satisfy the topology requirement in the class
		if !v1helper.MatchTopologySelectorTerms(class.AllowedTopologies, labels.Set(node.Labels)) {
			glog.V(4).Infof("Node %q cannot satisfy provisioning topology requirements of claim %q", node.Name, getPVCName(claim))
			return false, nil
		}

		// TODO: Check if capacity of the node domain in the storage class
		// can satisfy resource requirement of given claim

		provisionedClaims = append(provisionedClaims, claim)

	}
	glog.V(4).Infof("Provisioning for claims of pod %q that has no matching volumes on node %q ...", podName, node.Name)

	// Mark cache with all the PVCs that need provisioning for this node
	b.podBindingCache.UpdateProvisionedPVCs(pod, node.Name, provisionedClaims)

	return true, nil
}

func (b *volumeBinder) revertAssumedPVs(bindings []*bindingInfo) {
	for _, bindingInfo := range bindings {
		b.pvCache.Restore(bindingInfo.pv.Name)
	}
}

func (b *volumeBinder) revertAssumedPVCs(claims []*v1.PersistentVolumeClaim) {
	for _, claim := range claims {
		b.pvcCache.Restore(getPVCName(claim))
	}
}

type bindingInfo struct {
	// Claim that needs to be bound
	pvc *v1.PersistentVolumeClaim

	// Proposed PV to bind to this claim
	pv *v1.PersistentVolume
}

// Used in unit test errors
func (b bindingInfo) String() string {
	pvcName := ""
	pvName := ""
	if b.pvc != nil {
		pvcName = getPVCName(b.pvc)
	}
	if b.pv != nil {
		pvName = b.pv.Name
	}
	return fmt.Sprintf("[PVC %q, PV %q]", pvcName, pvName)
}

type byPVCSize []*bindingInfo

func (a byPVCSize) Len() int {
	return len(a)
}

func (a byPVCSize) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a byPVCSize) Less(i, j int) bool {
	iSize := a[i].pvc.Spec.Resources.Requests[v1.ResourceStorage]
	jSize := a[j].pvc.Spec.Resources.Requests[v1.ResourceStorage]
	// return true if iSize is less than jSize
	return iSize.Cmp(jSize) == -1
}
