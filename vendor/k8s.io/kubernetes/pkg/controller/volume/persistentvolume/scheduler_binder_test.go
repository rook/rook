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
	"reflect"
	"testing"

	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/diff"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/api/testapi"
	"k8s.io/kubernetes/pkg/controller"
)

var (
	unboundPVC                  = makeTestPVC("unbound-pvc", "1G", pvcUnbound, "", "1", &waitClass)
	unboundPVC2                 = makeTestPVC("unbound-pvc2", "5G", pvcUnbound, "", "1", &waitClass)
	preboundPVC                 = makeTestPVC("prebound-pvc", "1G", pvcPrebound, "pv-node1a", "1", &waitClass)
	boundPVC                    = makeTestPVC("bound-pvc", "1G", pvcBound, "pv-bound", "1", &waitClass)
	boundPVC2                   = makeTestPVC("bound-pvc2", "1G", pvcBound, "pv-bound2", "1", &waitClass)
	badPVC                      = makeBadPVC()
	immediateUnboundPVC         = makeTestPVC("immediate-unbound-pvc", "1G", pvcUnbound, "", "1", &immediateClass)
	immediateBoundPVC           = makeTestPVC("immediate-bound-pvc", "1G", pvcBound, "pv-bound-immediate", "1", &immediateClass)
	provisionedPVC              = makeTestPVC("provisioned-pvc", "1Gi", pvcUnbound, "", "1", &waitClass)
	provisionedPVC2             = makeTestPVC("provisioned-pvc2", "1Gi", pvcUnbound, "", "1", &waitClass)
	provisionedPVCHigherVersion = makeTestPVC("provisioned-pvc2", "1Gi", pvcUnbound, "", "2", &waitClass)
	noProvisionerPVC            = makeTestPVC("no-provisioner-pvc", "1Gi", pvcUnbound, "", "1", &provisionNotSupportClass)
	topoMismatchPVC             = makeTestPVC("topo-mismatch-pvc", "1Gi", pvcUnbound, "", "1", &topoMismatchClass)

	pvNoNode                   = makeTestPV("pv-no-node", "", "1G", "1", nil, waitClass)
	pvNode1a                   = makeTestPV("pv-node1a", "node1", "5G", "1", nil, waitClass)
	pvNode1b                   = makeTestPV("pv-node1b", "node1", "10G", "1", nil, waitClass)
	pvNode1c                   = makeTestPV("pv-node1b", "node1", "5G", "1", nil, waitClass)
	pvNode2                    = makeTestPV("pv-node2", "node2", "1G", "1", nil, waitClass)
	pvPrebound                 = makeTestPV("pv-prebound", "node1", "1G", "1", unboundPVC, waitClass)
	pvBound                    = makeTestPV("pv-bound", "node1", "1G", "1", boundPVC, waitClass)
	pvNode1aBound              = makeTestPV("pv-node1a", "node1", "1G", "1", unboundPVC, waitClass)
	pvNode1bBound              = makeTestPV("pv-node1b", "node1", "5G", "1", unboundPVC2, waitClass)
	pvNode1bBoundHigherVersion = makeTestPV("pv-node1b", "node1", "5G", "2", unboundPVC2, waitClass)
	pvBoundImmediate           = makeTestPV("pv-bound-immediate", "node1", "1G", "1", immediateBoundPVC, immediateClass)
	pvBoundImmediateNode2      = makeTestPV("pv-bound-immediate", "node2", "1G", "1", immediateBoundPVC, immediateClass)

	binding1a      = makeBinding(unboundPVC, pvNode1a)
	binding1b      = makeBinding(unboundPVC2, pvNode1b)
	bindingNoNode  = makeBinding(unboundPVC, pvNoNode)
	bindingBad     = makeBinding(badPVC, pvNode1b)
	binding1aBound = makeBinding(unboundPVC, pvNode1aBound)
	binding1bBound = makeBinding(unboundPVC2, pvNode1bBound)

	waitClass                = "waitClass"
	immediateClass           = "immediateClass"
	provisionNotSupportClass = "provisionNotSupportedClass"
	topoMismatchClass        = "topoMismatchClass"

	nodeLabelKey   = "nodeKey"
	nodeLabelValue = "node1"
)

type testEnv struct {
	client           clientset.Interface
	reactor          *volumeReactor
	binder           SchedulerVolumeBinder
	internalBinder   *volumeBinder
	internalPVCache  *pvAssumeCache
	internalPVCCache *pvcAssumeCache
}

func newTestBinder(t *testing.T) *testEnv {
	client := &fake.Clientset{}
	reactor := newVolumeReactor(client, nil, nil, nil, nil)
	informerFactory := informers.NewSharedInformerFactory(client, controller.NoResyncPeriodFunc())

	pvcInformer := informerFactory.Core().V1().PersistentVolumeClaims()
	classInformer := informerFactory.Storage().V1().StorageClasses()

	binder := NewVolumeBinder(
		client,
		pvcInformer,
		informerFactory.Core().V1().PersistentVolumes(),
		classInformer)

	// Add storageclasses
	waitMode := storagev1.VolumeBindingWaitForFirstConsumer
	immediateMode := storagev1.VolumeBindingImmediate
	classes := []*storagev1.StorageClass{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: waitClass,
			},
			VolumeBindingMode: &waitMode,
			Provisioner:       "test-provisioner",
			AllowedTopologies: []v1.TopologySelectorTerm{
				{
					MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
						{
							Key:    nodeLabelKey,
							Values: []string{nodeLabelValue, "reference-value"},
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: immediateClass,
			},
			VolumeBindingMode: &immediateMode,
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: provisionNotSupportClass,
			},
			VolumeBindingMode: &waitMode,
			Provisioner:       "kubernetes.io/no-provisioner",
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: topoMismatchClass,
			},
			VolumeBindingMode: &waitMode,
			Provisioner:       "test-provisioner",
			AllowedTopologies: []v1.TopologySelectorTerm{
				{
					MatchLabelExpressions: []v1.TopologySelectorLabelRequirement{
						{
							Key:    nodeLabelKey,
							Values: []string{"reference-value"},
						},
					},
				},
			},
		},
	}
	for _, class := range classes {
		if err := classInformer.Informer().GetIndexer().Add(class); err != nil {
			t.Fatalf("Failed to add storage class to internal cache: %v", err)
		}
	}

	// Get internal types
	internalBinder, ok := binder.(*volumeBinder)
	if !ok {
		t.Fatalf("Failed to convert to internal binder")
	}

	pvCache := internalBinder.pvCache
	internalPVCache, ok := pvCache.(*pvAssumeCache)
	if !ok {
		t.Fatalf("Failed to convert to internal PV cache")
	}

	pvcCache := internalBinder.pvcCache
	internalPVCCache, ok := pvcCache.(*pvcAssumeCache)
	if !ok {
		t.Fatalf("Failed to convert to internal PVC cache")
	}

	return &testEnv{
		client:           client,
		reactor:          reactor,
		binder:           binder,
		internalBinder:   internalBinder,
		internalPVCache:  internalPVCache,
		internalPVCCache: internalPVCCache,
	}
}

func (env *testEnv) initClaims(cachedPVCs []*v1.PersistentVolumeClaim, apiPVCs []*v1.PersistentVolumeClaim) {
	internalPVCCache := env.internalPVCCache
	for _, pvc := range cachedPVCs {
		internalPVCCache.add(pvc)
		if apiPVCs == nil {
			env.reactor.claims[pvc.Name] = pvc
		}
	}
	for _, pvc := range apiPVCs {
		env.reactor.claims[pvc.Name] = pvc
	}
}

func (env *testEnv) initVolumes(cachedPVs []*v1.PersistentVolume, apiPVs []*v1.PersistentVolume) {
	internalPVCache := env.internalPVCache
	for _, pv := range cachedPVs {
		internalPVCache.add(pv)
		if apiPVs == nil {
			env.reactor.volumes[pv.Name] = pv
		}
	}
	for _, pv := range apiPVs {
		env.reactor.volumes[pv.Name] = pv
	}

}

func (env *testEnv) assumeVolumes(t *testing.T, name, node string, pod *v1.Pod, bindings []*bindingInfo, provisionings []*v1.PersistentVolumeClaim) {
	pvCache := env.internalBinder.pvCache
	for _, binding := range bindings {
		if err := pvCache.Assume(binding.pv); err != nil {
			t.Fatalf("Failed to setup test %q: error: %v", name, err)
		}
	}

	env.internalBinder.podBindingCache.UpdateBindings(pod, node, bindings)

	pvcCache := env.internalBinder.pvcCache
	for _, pvc := range provisionings {
		if err := pvcCache.Assume(pvc); err != nil {
			t.Fatalf("Failed to setup test %q: error: %v", name, err)
		}
	}

	env.internalBinder.podBindingCache.UpdateProvisionedPVCs(pod, node, provisionings)
}

func (env *testEnv) initPodCache(pod *v1.Pod, node string, bindings []*bindingInfo, provisionings []*v1.PersistentVolumeClaim) {
	cache := env.internalBinder.podBindingCache
	cache.UpdateBindings(pod, node, bindings)

	cache.UpdateProvisionedPVCs(pod, node, provisionings)
}

func (env *testEnv) validatePodCache(t *testing.T, name, node string, pod *v1.Pod, expectedBindings []*bindingInfo, expectedProvisionings []*v1.PersistentVolumeClaim) {
	cache := env.internalBinder.podBindingCache
	bindings := cache.GetBindings(pod, node)

	if !reflect.DeepEqual(expectedBindings, bindings) {
		t.Errorf("Test %q failed: Expected bindings %+v, got %+v", name, expectedBindings, bindings)
	}

	provisionedClaims := cache.GetProvisionedPVCs(pod, node)

	if !reflect.DeepEqual(expectedProvisionings, provisionedClaims) {
		t.Errorf("Test %q failed: Expected provisionings %+v, got %+v", name, expectedProvisionings, provisionedClaims)
	}

}

func (env *testEnv) getPodBindings(t *testing.T, name, node string, pod *v1.Pod) []*bindingInfo {
	cache := env.internalBinder.podBindingCache
	return cache.GetBindings(pod, node)
}

func (env *testEnv) validateAssume(t *testing.T, name string, pod *v1.Pod, bindings []*bindingInfo, provisionings []*v1.PersistentVolumeClaim) {
	// TODO: Check binding cache

	// Check pv cache
	pvCache := env.internalBinder.pvCache
	for _, b := range bindings {
		pv, err := pvCache.GetPV(b.pv.Name)
		if err != nil {
			t.Errorf("Test %q failed: GetPV %q returned error: %v", name, b.pv.Name, err)
			continue
		}
		if pv.Spec.ClaimRef == nil {
			t.Errorf("Test %q failed: PV %q ClaimRef is nil", name, b.pv.Name)
			continue
		}
		if pv.Spec.ClaimRef.Name != b.pvc.Name {
			t.Errorf("Test %q failed: expected PV.ClaimRef.Name %q, got %q", name, b.pvc.Name, pv.Spec.ClaimRef.Name)
		}
		if pv.Spec.ClaimRef.Namespace != b.pvc.Namespace {
			t.Errorf("Test %q failed: expected PV.ClaimRef.Namespace %q, got %q", name, b.pvc.Namespace, pv.Spec.ClaimRef.Namespace)
		}
	}

	// Check pvc cache
	pvcCache := env.internalBinder.pvcCache
	for _, p := range provisionings {
		pvcKey := getPVCName(p)
		pvc, err := pvcCache.GetPVC(pvcKey)
		if err != nil {
			t.Errorf("Test %q failed: GetPVC %q returned error: %v", name, pvcKey, err)
			continue
		}
		if pvc.Annotations[annSelectedNode] != nodeLabelValue {
			t.Errorf("Test %q failed: expected annSelectedNode of pvc %q to be %q, but got %q", name, pvcKey, nodeLabelValue, pvc.Annotations[annSelectedNode])
		}
	}
}

func (env *testEnv) validateFailedAssume(t *testing.T, name string, pod *v1.Pod, bindings []*bindingInfo, provisionings []*v1.PersistentVolumeClaim) {
	// All PVs have been unmodified in cache
	pvCache := env.internalBinder.pvCache
	for _, b := range bindings {
		pv, _ := pvCache.GetPV(b.pv.Name)
		// PV could be nil if it's missing from cache
		if pv != nil && pv != b.pv {
			t.Errorf("Test %q failed: PV %q was modified in cache", name, b.pv.Name)
		}
	}

	// Check pvc cache
	pvcCache := env.internalBinder.pvcCache
	for _, p := range provisionings {
		pvcKey := getPVCName(p)
		pvc, err := pvcCache.GetPVC(pvcKey)
		if err != nil {
			t.Errorf("Test %q failed: GetPVC %q returned error: %v", name, pvcKey, err)
			continue
		}
		if pvc.Annotations[annSelectedNode] != "" {
			t.Errorf("Test %q failed: expected annSelectedNode of pvc %q empty, but got %q", name, pvcKey, pvc.Annotations[annSelectedNode])
		}
	}
}

func (env *testEnv) validateBind(
	t *testing.T,
	name string,
	pod *v1.Pod,
	expectedPVs []*v1.PersistentVolume,
	expectedAPIPVs []*v1.PersistentVolume) {

	// Check pv cache
	pvCache := env.internalBinder.pvCache
	for _, pv := range expectedPVs {
		cachedPV, err := pvCache.GetPV(pv.Name)
		if err != nil {
			t.Errorf("Test %q failed: GetPV %q returned error: %v", name, pv.Name, err)
		}
		if !reflect.DeepEqual(cachedPV, pv) {
			t.Errorf("Test %q failed: cached PV check failed [A-expected, B-got]:\n%s", name, diff.ObjectDiff(pv, cachedPV))
		}
	}

	// Check reactor for API updates
	if err := env.reactor.checkVolumes(expectedAPIPVs); err != nil {
		t.Errorf("Test %q failed: API reactor validation failed: %v", name, err)
	}
}

func (env *testEnv) validateProvision(
	t *testing.T,
	name string,
	pod *v1.Pod,
	expectedPVCs []*v1.PersistentVolumeClaim,
	expectedAPIPVCs []*v1.PersistentVolumeClaim) {

	// Check pvc cache
	pvcCache := env.internalBinder.pvcCache
	for _, pvc := range expectedPVCs {
		cachedPVC, err := pvcCache.GetPVC(getPVCName(pvc))
		if err != nil {
			t.Errorf("Test %q failed: GetPVC %q returned error: %v", name, getPVCName(pvc), err)
		}
		if !reflect.DeepEqual(cachedPVC, pvc) {
			t.Errorf("Test %q failed: cached PVC check failed [A-expected, B-got]:\n%s", name, diff.ObjectDiff(pvc, cachedPVC))
		}
	}

	// Check reactor for API updates
	if err := env.reactor.checkClaims(expectedAPIPVCs); err != nil {
		t.Errorf("Test %q failed: API reactor validation failed: %v", name, err)
	}
}

const (
	pvcUnbound = iota
	pvcPrebound
	pvcBound
)

func makeTestPVC(name, size string, pvcBoundState int, pvName, resourceVersion string, className *string) *v1.PersistentVolumeClaim {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       "testns",
			UID:             types.UID("pvc-uid"),
			ResourceVersion: resourceVersion,
			SelfLink:        testapi.Default.SelfLink("pvc", name),
			Annotations:     map[string]string{},
		},
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceName(v1.ResourceStorage): resource.MustParse(size),
				},
			},
			StorageClassName: className,
		},
	}

	switch pvcBoundState {
	case pvcBound:
		metav1.SetMetaDataAnnotation(&pvc.ObjectMeta, annBindCompleted, "yes")
		fallthrough
	case pvcPrebound:
		pvc.Spec.VolumeName = pvName
	}
	return pvc
}

func makeBadPVC() *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "bad-pvc",
			Namespace:       "testns",
			UID:             types.UID("pvc-uid"),
			ResourceVersion: "1",
			// Don't include SefLink, so that GetReference will fail
		},
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceName(v1.ResourceStorage): resource.MustParse("1G"),
				},
			},
			StorageClassName: &waitClass,
		},
	}
}

func makeTestPV(name, node, capacity, version string, boundToPVC *v1.PersistentVolumeClaim, className string) *v1.PersistentVolume {
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			ResourceVersion: version,
		},
		Spec: v1.PersistentVolumeSpec{
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): resource.MustParse(capacity),
			},
			StorageClassName: className,
		},
	}
	if node != "" {
		pv.Spec.NodeAffinity = getVolumeNodeAffinity(nodeLabelKey, node)
	}

	if boundToPVC != nil {
		pv.Spec.ClaimRef = &v1.ObjectReference{
			Name:      boundToPVC.Name,
			Namespace: boundToPVC.Namespace,
			UID:       boundToPVC.UID,
		}
		metav1.SetMetaDataAnnotation(&pv.ObjectMeta, annBoundByController, "yes")
	}

	return pv
}

func makePod(pvcs []*v1.PersistentVolumeClaim) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "testns",
		},
	}

	volumes := []v1.Volume{}
	for i, pvc := range pvcs {
		pvcVol := v1.Volume{
			Name: fmt.Sprintf("vol%v", i),
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc.Name,
				},
			},
		}
		volumes = append(volumes, pvcVol)
	}
	pod.Spec.Volumes = volumes
	pod.Spec.NodeName = "node1"
	return pod
}

func makePodWithoutPVC() *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "testns",
		},
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}
	return pod
}

func makeBinding(pvc *v1.PersistentVolumeClaim, pv *v1.PersistentVolume) *bindingInfo {
	return &bindingInfo{pvc: pvc, pv: pv}
}

func addProvisionAnn(pvc *v1.PersistentVolumeClaim) *v1.PersistentVolumeClaim {
	res := pvc.DeepCopy()
	// Add provision related annotations
	res.Annotations[annSelectedNode] = nodeLabelValue

	return res
}

func TestFindPodVolumesWithoutProvisioning(t *testing.T) {
	scenarios := map[string]struct {
		// Inputs
		pvs     []*v1.PersistentVolume
		podPVCs []*v1.PersistentVolumeClaim
		// If nil, use pod PVCs
		cachePVCs []*v1.PersistentVolumeClaim
		// If nil, makePod with podPVCs
		pod *v1.Pod

		// Expected podBindingCache fields
		expectedBindings []*bindingInfo

		// Expected return values
		expectedUnbound bool
		expectedBound   bool
		shouldFail      bool
	}{
		"no-volumes": {
			pod:             makePod(nil),
			expectedUnbound: true,
			expectedBound:   true,
		},
		"no-pvcs": {
			pod:             makePodWithoutPVC(),
			expectedUnbound: true,
			expectedBound:   true,
		},
		"pvc-not-found": {
			cachePVCs:       []*v1.PersistentVolumeClaim{},
			podPVCs:         []*v1.PersistentVolumeClaim{boundPVC},
			expectedUnbound: false,
			expectedBound:   false,
			shouldFail:      true,
		},
		"bound-pvc": {
			podPVCs:         []*v1.PersistentVolumeClaim{boundPVC},
			pvs:             []*v1.PersistentVolume{pvBound},
			expectedUnbound: true,
			expectedBound:   true,
		},
		"bound-pvc,pv-not-exists": {
			podPVCs:         []*v1.PersistentVolumeClaim{boundPVC},
			expectedUnbound: false,
			expectedBound:   false,
			shouldFail:      true,
		},
		"prebound-pvc": {
			podPVCs:         []*v1.PersistentVolumeClaim{preboundPVC},
			pvs:             []*v1.PersistentVolume{pvNode1aBound},
			expectedUnbound: true,
			expectedBound:   true,
		},
		"unbound-pvc,pv-same-node": {
			podPVCs:          []*v1.PersistentVolumeClaim{unboundPVC},
			pvs:              []*v1.PersistentVolume{pvNode2, pvNode1a, pvNode1b},
			expectedBindings: []*bindingInfo{binding1a},
			expectedUnbound:  true,
			expectedBound:    true,
		},
		"unbound-pvc,pv-different-node": {
			podPVCs:         []*v1.PersistentVolumeClaim{unboundPVC},
			pvs:             []*v1.PersistentVolume{pvNode2},
			expectedUnbound: false,
			expectedBound:   true,
		},
		"two-unbound-pvcs": {
			podPVCs:          []*v1.PersistentVolumeClaim{unboundPVC, unboundPVC2},
			pvs:              []*v1.PersistentVolume{pvNode1a, pvNode1b},
			expectedBindings: []*bindingInfo{binding1a, binding1b},
			expectedUnbound:  true,
			expectedBound:    true,
		},
		"two-unbound-pvcs,order-by-size": {
			podPVCs:          []*v1.PersistentVolumeClaim{unboundPVC2, unboundPVC},
			pvs:              []*v1.PersistentVolume{pvNode1a, pvNode1b},
			expectedBindings: []*bindingInfo{binding1a, binding1b},
			expectedUnbound:  true,
			expectedBound:    true,
		},
		"two-unbound-pvcs,partial-match": {
			podPVCs:          []*v1.PersistentVolumeClaim{unboundPVC, unboundPVC2},
			pvs:              []*v1.PersistentVolume{pvNode1a},
			expectedBindings: []*bindingInfo{binding1a},
			expectedUnbound:  false,
			expectedBound:    true,
		},
		"one-bound,one-unbound": {
			podPVCs:          []*v1.PersistentVolumeClaim{unboundPVC, boundPVC},
			pvs:              []*v1.PersistentVolume{pvBound, pvNode1a},
			expectedBindings: []*bindingInfo{binding1a},
			expectedUnbound:  true,
			expectedBound:    true,
		},
		"one-bound,one-unbound,no-match": {
			podPVCs:         []*v1.PersistentVolumeClaim{unboundPVC, boundPVC},
			pvs:             []*v1.PersistentVolume{pvBound, pvNode2},
			expectedUnbound: false,
			expectedBound:   true,
		},
		"one-prebound,one-unbound": {
			podPVCs:          []*v1.PersistentVolumeClaim{unboundPVC, preboundPVC},
			pvs:              []*v1.PersistentVolume{pvNode1a, pvNode1b},
			expectedBindings: []*bindingInfo{binding1a},
			expectedUnbound:  true,
			expectedBound:    true,
		},
		"immediate-bound-pvc": {
			podPVCs:         []*v1.PersistentVolumeClaim{immediateBoundPVC},
			pvs:             []*v1.PersistentVolume{pvBoundImmediate},
			expectedUnbound: true,
			expectedBound:   true,
		},
		"immediate-bound-pvc-wrong-node": {
			podPVCs:         []*v1.PersistentVolumeClaim{immediateBoundPVC},
			pvs:             []*v1.PersistentVolume{pvBoundImmediateNode2},
			expectedUnbound: true,
			expectedBound:   false,
		},
		"immediate-unbound-pvc": {
			podPVCs:         []*v1.PersistentVolumeClaim{immediateUnboundPVC},
			expectedUnbound: false,
			expectedBound:   false,
			shouldFail:      true,
		},
		"immediate-unbound-pvc,delayed-mode-bound": {
			podPVCs:         []*v1.PersistentVolumeClaim{immediateUnboundPVC, boundPVC},
			pvs:             []*v1.PersistentVolume{pvBound},
			expectedUnbound: false,
			expectedBound:   false,
			shouldFail:      true,
		},
		"immediate-unbound-pvc,delayed-mode-unbound": {
			podPVCs:         []*v1.PersistentVolumeClaim{immediateUnboundPVC, unboundPVC},
			expectedUnbound: false,
			expectedBound:   false,
			shouldFail:      true,
		},
	}

	// Set feature gate
	utilfeature.DefaultFeatureGate.Set("VolumeScheduling=true")
	defer utilfeature.DefaultFeatureGate.Set("VolumeScheduling=false")

	testNode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
			Labels: map[string]string{
				nodeLabelKey: "node1",
			},
		},
	}

	for name, scenario := range scenarios {
		glog.V(5).Infof("Running test case %q", name)

		// Setup
		testEnv := newTestBinder(t)
		testEnv.initVolumes(scenario.pvs, scenario.pvs)

		// a. Init pvc cache
		if scenario.cachePVCs == nil {
			scenario.cachePVCs = scenario.podPVCs
		}
		testEnv.initClaims(scenario.cachePVCs, scenario.cachePVCs)

		// b. Generate pod with given claims
		if scenario.pod == nil {
			scenario.pod = makePod(scenario.podPVCs)
		}

		// Execute
		unboundSatisfied, boundSatisfied, err := testEnv.binder.FindPodVolumes(scenario.pod, testNode)

		// Validate
		if !scenario.shouldFail && err != nil {
			t.Errorf("Test %q failed: returned error: %v", name, err)
		}
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		if boundSatisfied != scenario.expectedBound {
			t.Errorf("Test %q failed: expected boundSatsified %v, got %v", name, scenario.expectedBound, boundSatisfied)
		}
		if unboundSatisfied != scenario.expectedUnbound {
			t.Errorf("Test %q failed: expected unboundSatsified %v, got %v", name, scenario.expectedUnbound, unboundSatisfied)
		}
		testEnv.validatePodCache(t, name, testNode.Name, scenario.pod, scenario.expectedBindings, nil)
	}
}

func TestFindPodVolumesWithProvisioning(t *testing.T) {
	scenarios := map[string]struct {
		// Inputs
		pvs     []*v1.PersistentVolume
		podPVCs []*v1.PersistentVolumeClaim
		// If nil, use pod PVCs
		cachePVCs []*v1.PersistentVolumeClaim
		// If nil, makePod with podPVCs
		pod *v1.Pod

		// Expected podBindingCache fields
		expectedBindings   []*bindingInfo
		expectedProvisions []*v1.PersistentVolumeClaim

		// Expected return values
		expectedUnbound bool
		expectedBound   bool
		shouldFail      bool
	}{
		"one-provisioned": {
			podPVCs:            []*v1.PersistentVolumeClaim{provisionedPVC},
			expectedProvisions: []*v1.PersistentVolumeClaim{provisionedPVC},
			expectedUnbound:    true,
			expectedBound:      true,
		},
		"two-unbound-pvcs,one-matched,one-provisioned": {
			podPVCs:            []*v1.PersistentVolumeClaim{unboundPVC, provisionedPVC},
			pvs:                []*v1.PersistentVolume{pvNode1a},
			expectedBindings:   []*bindingInfo{binding1a},
			expectedProvisions: []*v1.PersistentVolumeClaim{provisionedPVC},
			expectedUnbound:    true,
			expectedBound:      true,
		},
		"one-bound,one-provisioned": {
			podPVCs:            []*v1.PersistentVolumeClaim{boundPVC, provisionedPVC},
			pvs:                []*v1.PersistentVolume{pvBound},
			expectedProvisions: []*v1.PersistentVolumeClaim{provisionedPVC},
			expectedUnbound:    true,
			expectedBound:      true,
		},
		"immediate-unbound-pvc": {
			podPVCs:         []*v1.PersistentVolumeClaim{immediateUnboundPVC},
			expectedUnbound: false,
			expectedBound:   false,
			shouldFail:      true,
		},
		"one-immediate-bound,one-provisioned": {
			podPVCs:            []*v1.PersistentVolumeClaim{immediateBoundPVC, provisionedPVC},
			pvs:                []*v1.PersistentVolume{pvBoundImmediate},
			expectedProvisions: []*v1.PersistentVolumeClaim{provisionedPVC},
			expectedUnbound:    true,
			expectedBound:      true,
		},
		"invalid-provisioner": {
			podPVCs:         []*v1.PersistentVolumeClaim{noProvisionerPVC},
			expectedUnbound: false,
			expectedBound:   true,
		},
		"volume-topology-unsatisfied": {
			podPVCs:         []*v1.PersistentVolumeClaim{topoMismatchPVC},
			expectedUnbound: false,
			expectedBound:   true,
		},
	}

	// Set VolumeScheduling and DynamicProvisioningScheduling feature gate
	utilfeature.DefaultFeatureGate.Set("VolumeScheduling=true,DynamicProvisioningScheduling=true")
	defer utilfeature.DefaultFeatureGate.Set("VolumeScheduling=false,DynamicProvisioningScheduling=false")

	testNode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
			Labels: map[string]string{
				nodeLabelKey: "node1",
			},
		},
	}

	for name, scenario := range scenarios {
		// Setup
		testEnv := newTestBinder(t)
		testEnv.initVolumes(scenario.pvs, scenario.pvs)

		// a. Init pvc cache
		if scenario.cachePVCs == nil {
			scenario.cachePVCs = scenario.podPVCs
		}
		testEnv.initClaims(scenario.cachePVCs, scenario.cachePVCs)

		// b. Generate pod with given claims
		if scenario.pod == nil {
			scenario.pod = makePod(scenario.podPVCs)
		}

		// Execute
		unboundSatisfied, boundSatisfied, err := testEnv.binder.FindPodVolumes(scenario.pod, testNode)

		// Validate
		if !scenario.shouldFail && err != nil {
			t.Errorf("Test %q failed: returned error: %v", name, err)
		}
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		if boundSatisfied != scenario.expectedBound {
			t.Errorf("Test %q failed: expected boundSatsified %v, got %v", name, scenario.expectedBound, boundSatisfied)
		}
		if unboundSatisfied != scenario.expectedUnbound {
			t.Errorf("Test %q failed: expected unboundSatsified %v, got %v", name, scenario.expectedUnbound, unboundSatisfied)
		}
		testEnv.validatePodCache(t, name, testNode.Name, scenario.pod, scenario.expectedBindings, scenario.expectedProvisions)
	}
}

func TestAssumePodVolumes(t *testing.T) {
	scenarios := map[string]struct {
		// Inputs
		podPVCs         []*v1.PersistentVolumeClaim
		pvs             []*v1.PersistentVolume
		bindings        []*bindingInfo
		provisionedPVCs []*v1.PersistentVolumeClaim

		// Expected return values
		shouldFail              bool
		expectedBindingRequired bool
		expectedAllBound        bool

		// if nil, use bindings
		expectedBindings []*bindingInfo
	}{
		"all-bound": {
			podPVCs:          []*v1.PersistentVolumeClaim{boundPVC},
			pvs:              []*v1.PersistentVolume{pvBound},
			expectedAllBound: true,
		},
		"prebound-pvc": {
			podPVCs: []*v1.PersistentVolumeClaim{preboundPVC},
			pvs:     []*v1.PersistentVolume{pvNode1a},
		},
		"one-binding": {
			podPVCs:  []*v1.PersistentVolumeClaim{unboundPVC},
			bindings: []*bindingInfo{binding1a},
			pvs:      []*v1.PersistentVolume{pvNode1a},
			expectedBindingRequired: true,
		},
		"two-bindings": {
			podPVCs:  []*v1.PersistentVolumeClaim{unboundPVC, unboundPVC2},
			bindings: []*bindingInfo{binding1a, binding1b},
			pvs:      []*v1.PersistentVolume{pvNode1a, pvNode1b},
			expectedBindingRequired: true,
		},
		"pv-already-bound": {
			podPVCs:  []*v1.PersistentVolumeClaim{unboundPVC},
			bindings: []*bindingInfo{binding1aBound},
			pvs:      []*v1.PersistentVolume{pvNode1aBound},
			expectedBindingRequired: false,
			expectedBindings:        []*bindingInfo{},
		},
		"claimref-failed": {
			podPVCs:                 []*v1.PersistentVolumeClaim{unboundPVC},
			bindings:                []*bindingInfo{binding1a, bindingBad},
			pvs:                     []*v1.PersistentVolume{pvNode1a, pvNode1b},
			shouldFail:              true,
			expectedBindingRequired: true,
		},
		"tmpupdate-failed": {
			podPVCs:                 []*v1.PersistentVolumeClaim{unboundPVC},
			bindings:                []*bindingInfo{binding1a, binding1b},
			pvs:                     []*v1.PersistentVolume{pvNode1a},
			shouldFail:              true,
			expectedBindingRequired: true,
		},
		"one-binding, one-pvc-provisioned": {
			podPVCs:                 []*v1.PersistentVolumeClaim{unboundPVC, provisionedPVC},
			bindings:                []*bindingInfo{binding1a},
			pvs:                     []*v1.PersistentVolume{pvNode1a},
			provisionedPVCs:         []*v1.PersistentVolumeClaim{provisionedPVC},
			expectedBindingRequired: true,
		},
		"one-binding, one-provision-tmpupdate-failed": {
			podPVCs:                 []*v1.PersistentVolumeClaim{unboundPVC, provisionedPVCHigherVersion},
			bindings:                []*bindingInfo{binding1a},
			pvs:                     []*v1.PersistentVolume{pvNode1a},
			provisionedPVCs:         []*v1.PersistentVolumeClaim{provisionedPVC2},
			shouldFail:              true,
			expectedBindingRequired: true,
		},
	}

	for name, scenario := range scenarios {
		glog.V(5).Infof("Running test case %q", name)

		// Setup
		testEnv := newTestBinder(t)
		testEnv.initClaims(scenario.podPVCs, scenario.podPVCs)
		pod := makePod(scenario.podPVCs)
		testEnv.initPodCache(pod, "node1", scenario.bindings, scenario.provisionedPVCs)
		testEnv.initVolumes(scenario.pvs, scenario.pvs)

		// Execute
		allBound, bindingRequired, err := testEnv.binder.AssumePodVolumes(pod, "node1")

		// Validate
		if !scenario.shouldFail && err != nil {
			t.Errorf("Test %q failed: returned error: %v", name, err)
		}
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		if scenario.expectedBindingRequired != bindingRequired {
			t.Errorf("Test %q failed: returned unexpected bindingRequired: %v", name, bindingRequired)
		}
		if scenario.expectedAllBound != allBound {
			t.Errorf("Test %q failed: returned unexpected allBound: %v", name, allBound)
		}
		if scenario.expectedBindings == nil {
			scenario.expectedBindings = scenario.bindings
		}
		if scenario.shouldFail {
			testEnv.validateFailedAssume(t, name, pod, scenario.expectedBindings, scenario.provisionedPVCs)
		} else {
			testEnv.validateAssume(t, name, pod, scenario.expectedBindings, scenario.provisionedPVCs)
		}
	}
}

func TestBindPodVolumes(t *testing.T) {
	scenarios := map[string]struct {
		// Inputs
		bindings  []*bindingInfo
		cachedPVs []*v1.PersistentVolume
		// if nil, use cachedPVs
		apiPVs []*v1.PersistentVolume

		provisionedPVCs []*v1.PersistentVolumeClaim
		cachedPVCs      []*v1.PersistentVolumeClaim
		// if nil, use cachedPVCs
		apiPVCs []*v1.PersistentVolumeClaim

		// Expected return values
		shouldFail  bool
		expectedPVs []*v1.PersistentVolume
		// if nil, use expectedPVs
		expectedAPIPVs []*v1.PersistentVolume

		expectedPVCs []*v1.PersistentVolumeClaim
		// if nil, use expectedPVCs
		expectedAPIPVCs []*v1.PersistentVolumeClaim
	}{
		"all-bound": {},
		"not-fully-bound": {
			bindings: []*bindingInfo{},
		},
		"one-binding": {
			bindings:    []*bindingInfo{binding1aBound},
			cachedPVs:   []*v1.PersistentVolume{pvNode1a},
			expectedPVs: []*v1.PersistentVolume{pvNode1aBound},
		},
		"two-bindings": {
			bindings:    []*bindingInfo{binding1aBound, binding1bBound},
			cachedPVs:   []*v1.PersistentVolume{pvNode1a, pvNode1b},
			expectedPVs: []*v1.PersistentVolume{pvNode1aBound, pvNode1bBound},
		},
		"api-update-failed": {
			bindings:       []*bindingInfo{binding1aBound, binding1bBound},
			cachedPVs:      []*v1.PersistentVolume{pvNode1a, pvNode1b},
			apiPVs:         []*v1.PersistentVolume{pvNode1a, pvNode1bBoundHigherVersion},
			expectedPVs:    []*v1.PersistentVolume{pvNode1aBound, pvNode1b},
			expectedAPIPVs: []*v1.PersistentVolume{pvNode1aBound, pvNode1bBoundHigherVersion},
			shouldFail:     true,
		},
		"one-provisioned-pvc": {
			provisionedPVCs: []*v1.PersistentVolumeClaim{addProvisionAnn(provisionedPVC)},
			cachedPVCs:      []*v1.PersistentVolumeClaim{provisionedPVC},
			expectedPVCs:    []*v1.PersistentVolumeClaim{addProvisionAnn(provisionedPVC)},
		},
		"provision-api-update-failed": {
			provisionedPVCs: []*v1.PersistentVolumeClaim{addProvisionAnn(provisionedPVC), addProvisionAnn(provisionedPVC2)},
			cachedPVCs:      []*v1.PersistentVolumeClaim{provisionedPVC, provisionedPVC2},
			apiPVCs:         []*v1.PersistentVolumeClaim{provisionedPVC, provisionedPVCHigherVersion},
			expectedPVCs:    []*v1.PersistentVolumeClaim{addProvisionAnn(provisionedPVC), provisionedPVC2},
			expectedAPIPVCs: []*v1.PersistentVolumeClaim{addProvisionAnn(provisionedPVC), provisionedPVCHigherVersion},
			shouldFail:      true,
		},
		"bingding-succeed, provision-api-update-failed": {
			bindings:        []*bindingInfo{binding1aBound},
			cachedPVs:       []*v1.PersistentVolume{pvNode1a},
			expectedPVs:     []*v1.PersistentVolume{pvNode1aBound},
			provisionedPVCs: []*v1.PersistentVolumeClaim{addProvisionAnn(provisionedPVC), addProvisionAnn(provisionedPVC2)},
			cachedPVCs:      []*v1.PersistentVolumeClaim{provisionedPVC, provisionedPVC2},
			apiPVCs:         []*v1.PersistentVolumeClaim{provisionedPVC, provisionedPVCHigherVersion},
			expectedPVCs:    []*v1.PersistentVolumeClaim{addProvisionAnn(provisionedPVC), provisionedPVC2},
			expectedAPIPVCs: []*v1.PersistentVolumeClaim{addProvisionAnn(provisionedPVC), provisionedPVCHigherVersion},
			shouldFail:      true,
		},
	}
	for name, scenario := range scenarios {
		glog.V(5).Infof("Running test case %q", name)

		// Setup
		testEnv := newTestBinder(t)
		pod := makePod(nil)
		if scenario.apiPVs == nil {
			scenario.apiPVs = scenario.cachedPVs
		}
		if scenario.apiPVCs == nil {
			scenario.apiPVCs = scenario.cachedPVCs
		}
		testEnv.initVolumes(scenario.cachedPVs, scenario.apiPVs)
		testEnv.initClaims(scenario.cachedPVCs, scenario.apiPVCs)
		testEnv.assumeVolumes(t, name, "node1", pod, scenario.bindings, scenario.provisionedPVCs)

		// Execute
		err := testEnv.binder.BindPodVolumes(pod)

		// Validate
		if !scenario.shouldFail && err != nil {
			t.Errorf("Test %q failed: returned error: %v", name, err)
		}
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		if scenario.expectedAPIPVs == nil {
			scenario.expectedAPIPVs = scenario.expectedPVs
		}
		if scenario.expectedAPIPVCs == nil {
			scenario.expectedAPIPVCs = scenario.expectedPVCs
		}
		testEnv.validateBind(t, name, pod, scenario.expectedPVs, scenario.expectedAPIPVs)
		testEnv.validateProvision(t, name, pod, scenario.expectedPVCs, scenario.expectedAPIPVCs)
	}
}

func TestFindAssumeVolumes(t *testing.T) {
	// Set feature gate
	utilfeature.DefaultFeatureGate.Set("VolumeScheduling=true")
	defer utilfeature.DefaultFeatureGate.Set("VolumeScheduling=false")

	// Test case
	podPVCs := []*v1.PersistentVolumeClaim{unboundPVC}
	pvs := []*v1.PersistentVolume{pvNode2, pvNode1a, pvNode1c}

	// Setup
	testEnv := newTestBinder(t)
	testEnv.initVolumes(pvs, pvs)
	testEnv.initClaims(podPVCs, podPVCs)
	pod := makePod(podPVCs)

	testNode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
			Labels: map[string]string{
				nodeLabelKey: "node1",
			},
		},
	}

	// Execute
	// 1. Find matching PVs
	unboundSatisfied, _, err := testEnv.binder.FindPodVolumes(pod, testNode)
	if err != nil {
		t.Errorf("Test failed: FindPodVolumes returned error: %v", err)
	}
	if !unboundSatisfied {
		t.Errorf("Test failed: couldn't find PVs for all PVCs")
	}
	expectedBindings := testEnv.getPodBindings(t, "before-assume", testNode.Name, pod)

	// 2. Assume matches
	allBound, bindingRequired, err := testEnv.binder.AssumePodVolumes(pod, testNode.Name)
	if err != nil {
		t.Errorf("Test failed: AssumePodVolumes returned error: %v", err)
	}
	if allBound {
		t.Errorf("Test failed: detected unbound volumes as bound")
	}
	if !bindingRequired {
		t.Errorf("Test failed: binding not required")
	}
	testEnv.validateAssume(t, "assume", pod, expectedBindings, nil)
	// After assume, claimref should be set on pv
	expectedBindings = testEnv.getPodBindings(t, "after-assume", testNode.Name, pod)

	// 3. Find matching PVs again
	// This should always return the original chosen pv
	// Run this many times in case sorting returns different orders for the two PVs.
	t.Logf("Testing FindPodVolumes after Assume")
	for i := 0; i < 50; i++ {
		unboundSatisfied, _, err := testEnv.binder.FindPodVolumes(pod, testNode)
		if err != nil {
			t.Errorf("Test failed: FindPodVolumes returned error: %v", err)
		}
		if !unboundSatisfied {
			t.Errorf("Test failed: couldn't find PVs for all PVCs")
		}
		testEnv.validatePodCache(t, "after-assume", testNode.Name, pod, expectedBindings, nil)
	}
}
