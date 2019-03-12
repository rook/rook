/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package integration

import (
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Smoke Test for Mon failover - Test check the following operations for the Mon failover in order
// Delete mon pod, Wait for new mon pod
func (suite *SmokeSuite) TestMonFailover() {
	logger.Infof("Mon Failover Smoke Test")

	opts := metav1.ListOptions{LabelSelector: "app=rook-ceph-mon"}
	deployments, err := suite.k8sh.Clientset.AppsV1().Deployments(suite.namespace).List(opts)
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), 3, len(deployments.Items))

	monToKill := deployments.Items[0].Name
	logger.Infof("Killing mon %s", monToKill)
	propagation := metav1.DeletePropagationForeground
	delOptions := &metav1.DeleteOptions{PropagationPolicy: &propagation}
	err = suite.k8sh.Clientset.AppsV1().Deployments(suite.namespace).Delete(monToKill, delOptions)
	require.Nil(suite.T(), err)

	// Wait for the health check to start a new monitor
	for i := 0; i < 10; i++ {
		deployments, err := suite.k8sh.Clientset.AppsV1().Deployments(suite.namespace).List(opts)
		require.Nil(suite.T(), err)

		// Make sure the old mon is not still alive
		foundOldMon := false
		for _, mon := range deployments.Items {
			if mon.Name == monToKill {
				foundOldMon = true
			}
		}

		// Check if we have three monitors
		if foundOldMon {
			logger.Infof("Waiting for old monitor to stop")
		} else {
			logger.Infof("Waiting for a new monitor to start")
			if len(deployments.Items) == 3 {
				var newMons []string
				for _, mon := range deployments.Items {
					newMons = append(newMons, mon.Name)
				}
				logger.Infof("Found a new monitor! monitors=%v", newMons)
				return
			}

			assert.Equal(suite.T(), 2, len(deployments.Items))
		}

		time.Sleep(8 * time.Second)
	}

	require.Fail(suite.T(), "giving up waiting for a new monitor")
}
