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
	"fmt"
	"testing"

	cephClient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetOSDNodeAssociation(t *testing.T) {

	hostNum := 5
	hostNames := make([]string, hostNum)
	for i := 0; i < hostNum; i++ {
		hostNames[i] = fmt.Sprintf("worker-%d", i)
	}

	zoneNum := hostNum
	zoneNames := make([]string, zoneNum)
	for i := 0; i < zoneNum; i++ {
		zoneNames[i] = fmt.Sprintf("zone-%d", i)
	}

	osdNum := hostNum * 3
	osdNames := make([]string, osdNum)
	for i := 0; i < osdNum; i++ {
		osdNames[i] = fmt.Sprintf("osd-%d", i)
	}

	osdDataList := []OsdData{
		{
			Deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: osdNames[0],
				},
			},
			CrushMeta: &cephClient.CrushFindResult{
				Location: map[string]string{
					"host": hostNames[0],
					"zone": zoneNames[0],
				},
			},
		},
		{
			Deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: osdNames[1],
				},
			},
			CrushMeta: &cephClient.CrushFindResult{
				Location: map[string]string{
					"host": hostNames[1],
					"zone": zoneNames[1],
				},
			},
		},
		{
			Deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: osdNames[2],
				},
			},
			CrushMeta: &cephClient.CrushFindResult{
				Location: map[string]string{
					"host": hostNames[1],
					"zone": zoneNames[2],
				},
			},
		},
		{
			Deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: osdNames[3],
				},
			},
			CrushMeta: &cephClient.CrushFindResult{
				Location: map[string]string{
					"host": hostNames[1],
					"zone": zoneNames[2],
				},
			},
		},
		{
			Deployment: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: osdNames[4],
				},
			},
			CrushMeta: &cephClient.CrushFindResult{
				Location: map[string]string{
					"host": hostNames[2],
					"zone": zoneNames[0],
				},
			},
		},
	}
	nodeList := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: hostNames[0],
				Labels: map[string]string{
					corev1.LabelHostname:          hostNames[0],
					corev1.LabelZoneFailureDomain: zoneNames[1],
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: hostNames[2],
				Labels: map[string]string{
					corev1.LabelHostname:          hostNames[2],
					corev1.LabelZoneFailureDomain: zoneNames[2],
				},
			},
		},
		nil,
	}

	// osds from hostName[0] - (0)
	// osds from hostNames[1] - (2,3,1)
	// osds from hostNames[2] - (4)

	// hosts from nodeList (0,2)
	osdDataSubset, err := getOSDsForNodes(osdDataList, nodeList, "host")
	assert.Nil(t, err)
	assert.Len(t, osdDataSubset, 2)

	failureDomainsMap, err := getFailureDomainMapForOsds(osdDataSubset, "host")
	assert.Nil(t, err)
	failureDomainSubset := getSortedOSDMapKeys(failureDomainsMap)
	assert.Equal(t, []string{hostNames[0], hostNames[2]}, failureDomainSubset)

	// osds from zone[0] - (0,4)
	// osds from zone[1] - (1)
	// osds from zone[2] - (2,3)

	// zones from nodeList (0,2)
	osdDataSubset, err = getOSDsForNodes(osdDataList, nodeList, "zone")
	assert.Nil(t, err)
	assert.Len(t, osdDataSubset, 3)

	failureDomainsMap, err = getFailureDomainMapForOsds(osdDataSubset, "zone")
	assert.Nil(t, err)

	failureDomainSubset = getSortedOSDMapKeys(failureDomainsMap)
	assert.Equal(t, []string{zoneNames[1], zoneNames[2]}, failureDomainSubset)

}
