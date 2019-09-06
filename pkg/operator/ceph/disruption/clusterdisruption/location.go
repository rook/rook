/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package clusterdisruption

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	cephClient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileClusterDisruption) getOsdDataList(request reconcile.Request) ([]OsdData, error) {
	osdDeploymentList := &appsv1.DeploymentList{}
	namespaceListOpts := client.InNamespace(request.Namespace)
	err := r.client.List(context.TODO(), osdDeploymentList, client.MatchingLabels{k8sutil.AppAttr: osd.AppName}, namespaceListOpts)
	if err != nil {
		return nil, fmt.Errorf("could not list osd deployments: %+v", err)
	}

	osds := make([]OsdData, 0)

	for _, deployment := range osdDeploymentList.Items {
		osdData := OsdData{Deployment: deployment}
		labels := deployment.Spec.Template.ObjectMeta.GetLabels()
		osdID, ok := labels[osd.OsdIdLabelKey]
		if !ok {
			return nil, fmt.Errorf("osd %s was not labeled with ", deployment.ObjectMeta.Name)
		}
		osdIDInt, err := strconv.Atoi(osdID)
		if err != nil {
			return nil, fmt.Errorf("osd id not an int: %s", osdID)
		}
		crushMeta, err := r.osdCrushLocationMap.Get(request.Namespace, osdIDInt)
		if err != nil {
			return nil, fmt.Errorf("could not fetch info from ceph for osd %s", osdID)
		}
		osdData.CrushMeta = crushMeta
		osds = append(osds, osdData)

	}
	return osds, nil
}

// OsdData stores the deployment and the Crush Data of the osd together
type OsdData struct {
	Deployment appsv1.Deployment
	CrushMeta  *cephClient.CrushFindResult
}

// OSDCrushLocationMap is used to maintain a cache of map of osd id to cephClientCrushhFindResults
// the crush position of osds wrt to the failureDomain is not expected to change often, so a default Resync period
// of half an hour is used, but if a use case arises where this is needed, ResyncPeriod should be made smaller.
type OSDCrushLocationMap struct {
	ResyncPeriod       time.Duration
	Context            *clusterd.Context
	clusterLocationMap map[string]map[int]cachedOSDLocation
	mux                sync.Mutex
}

type cachedOSDLocation struct {
	result     *cephClient.CrushFindResult
	lastSynced time.Time
}

// Get takes an osd id and returns a CrushFindResult from cache
func (o *OSDCrushLocationMap) Get(clusterNamespace string, id int) (*cephClient.CrushFindResult, error) {
	o.mux.Lock()
	defer o.mux.Unlock()
	if o.ResyncPeriod == 0 {
		o.ResyncPeriod = 30 * time.Minute
	}

	// initialize clusterLocationMap
	if len(o.clusterLocationMap) == 0 {
		o.clusterLocationMap = make(map[string]map[int]cachedOSDLocation)
	}
	locationMap, ok := o.clusterLocationMap[clusterNamespace]
	// initialize namespace map
	if !ok {
		o.clusterLocationMap[clusterNamespace] = make(map[int]cachedOSDLocation)
	}

	// sync of osd id not found in clusterNamespace
	osdLocation, ok := locationMap[id]
	if !ok {
		osdResult, err := o.get(clusterNamespace, id)
		if err != nil {
			return nil, fmt.Errorf("failed to run `find` on osd %d in cluster %s: %v", id, clusterNamespace, err)
		}
		o.clusterLocationMap[clusterNamespace][id] = cachedOSDLocation{result: osdResult, lastSynced: time.Now()}
		return osdResult, nil
	}

	// sync if not synced for longer than ResyncPeriod
	if time.Since(osdLocation.lastSynced) > o.ResyncPeriod {
		osdResult, err := o.get(clusterNamespace, id)
		if err != nil {
			return nil, fmt.Errorf("failed to run `find` on osd %d in cluster %s: %v", id, clusterNamespace, err)
		}
		o.clusterLocationMap[clusterNamespace][id] = cachedOSDLocation{result: osdResult, lastSynced: time.Now()}
		return osdResult, nil
	}

	return osdLocation.result, nil

}

// uncached version
func (o *OSDCrushLocationMap) get(clusterNamespace string, id int) (*cephClient.CrushFindResult, error) {
	osdResult, err := cephClient.FindOSDInCrushMap(o.Context, clusterNamespace, id)
	if err != nil {
		return nil, fmt.Errorf("failed running find on osd %d: %+v", id, err)
	}
	o.clusterLocationMap[clusterNamespace][id] = cachedOSDLocation{
		result:     osdResult,
		lastSynced: time.Now(),
	}
	return osdResult, nil
}

func getOSDsForNodes(osdDataList []OsdData, nodeList []*corev1.Node, failureDomainType string) ([]OsdData, error) {
	nodeOsdDataList := make([]OsdData, 0)
	for _, node := range nodeList {
		if node == nil {
			logger.Warningf("node in nodelist was nil")
			continue
		}
		topologyLabelMap := map[string]string{
			"host":   corev1.LabelHostname,
			"zone":   corev1.LabelZoneFailureDomain,
			"region": corev1.LabelZoneRegion,
		}
		failureDomainLabel, ok := topologyLabelMap[failureDomainType]
		if !ok {
			return nil, fmt.Errorf("invalid failure domain %s cannot manage PDBs for OSDs", failureDomainType)
		}
		nodeLabels := node.ObjectMeta.GetLabels()
		for _, osdData := range osdDataList {
			secondaryCrushHostname := osdData.CrushMeta.Host
			crushFailureDomain, ok := osdData.CrushMeta.Location[failureDomainType]
			if !ok && secondaryCrushHostname == "" {
				return nil, fmt.Errorf("could not find the CrushFindResult.Location['%s'] for %s", failureDomainType, osdData.Deployment.ObjectMeta.Name)
			}
			nodeFailureDomain, ok := nodeLabels[failureDomainLabel]
			if !ok {
				return nil, fmt.Errorf("could not find the %s label on node %s", failureDomainLabel, node.ObjectMeta.Name)
			}
			if cephClient.IsNormalizedCrushNameEqual(nodeFailureDomain, crushFailureDomain) || cephClient.IsNormalizedCrushNameEqual(secondaryCrushHostname, crushFailureDomain) {
				nodeOsdDataList = append(nodeOsdDataList, osdData)
			}
		}
	}
	return nodeOsdDataList, nil
}

func getFailureDomainMapForOsds(osdDataList []OsdData, failureDomainType string) (map[string][]OsdData, error) {
	failureDomainMap := make(map[string][]OsdData, 0)
	unfoundOSDs := make([]string, 0)
	var err error
	for _, osdData := range osdDataList {
		failureDomainValue, ok := osdData.CrushMeta.Location[failureDomainType]
		if !ok {
			logger.Errorf("failureDomain type %s not associated with %s", failureDomainType, osdData.Deployment.ObjectMeta.Name)
			unfoundOSDs = append(unfoundOSDs, osdData.Deployment.ObjectMeta.Name)
		} else {
			if len(failureDomainMap[failureDomainValue]) == 0 {
				failureDomainMap[failureDomainValue] = make([]OsdData, 0)
			}
			failureDomainMap[failureDomainValue] = append(failureDomainMap[failureDomainValue], osdData)
		}
	}
	if len(unfoundOSDs) > 0 {
		err = fmt.Errorf("failure domain type %s not associated with osds: [%s]", failureDomainType, strings.Join(unfoundOSDs, ","))
	}
	return failureDomainMap, err
}

func getSortedOSDMapKeys(m map[string][]OsdData) []string {
	list := make([]string, len(m))
	count := 0
	for key := range m {
		list[count] = key
		count++
	}
	sort.Strings(list)
	return list
}
