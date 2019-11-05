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
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *SmokeSuite) getNonCanaryMonDeployments() ([]appsv1.Deployment, error) {
	opts := metav1.ListOptions{LabelSelector: "app=rook-ceph-mon"}
	deployments, err := suite.k8sh.Clientset.AppsV1().Deployments(suite.namespace).List(opts)
	if err != nil {
		return nil, err
	}
	nonCanaryMonDeployments := []appsv1.Deployment{}
	for _, deployment := range deployments.Items {
		if !strings.HasSuffix(deployment.GetName(), "-canary") {
			nonCanaryMonDeployments = append(nonCanaryMonDeployments, deployment)
		}
	}
	return nonCanaryMonDeployments, nil
}

// Smoke Test for Mon failover - Test check the following operations for the Mon failover in order
// Delete mon pod, Wait for new mon pod
func (suite *SmokeSuite) TestMonFailover() {
	logger.Infof("Mon Failover Smoke Test")

	deployments, err := suite.getNonCanaryMonDeployments()
	require.Nil(suite.T(), err)
	require.Equal(suite.T(), 3, len(deployments))

	monToKill := deployments[0].Name
	logger.Infof("Killing mon %s", monToKill)
	propagation := metav1.DeletePropagationForeground
	delOptions := &metav1.DeleteOptions{PropagationPolicy: &propagation}
	err = suite.k8sh.Clientset.AppsV1().Deployments(suite.namespace).Delete(monToKill, delOptions)
	require.Nil(suite.T(), err)

	// Wait for the health check to start a new monitor
	originalMonDeleted := false
	for i := 0; i < 30; i++ {
		deployments, err := suite.getNonCanaryMonDeployments()
		require.Nil(suite.T(), err)

		// Make sure the old mon is not still alive
		foundOldMon := false
		for _, mon := range deployments {
			if mon.Name == monToKill {
				foundOldMon = true
			}
		}

		// Check if we have three monitors
		if foundOldMon {
			if originalMonDeleted {
				// Depending on the state of the orchestration, the operator might trigger
				// re-creation of the deleted mon. In this case, consider the test successful
				// rather than wait for the failover which will never occur.
				logger.Infof("Original mon created again, no need to wait for mon failover")
				return
			}
			logger.Infof("Waiting for old monitor to stop")
		} else {
			logger.Infof("Waiting for a new monitor to start")
			originalMonDeleted = true
			if len(deployments) == 3 {
				var newMons []string
				for _, mon := range deployments {
					newMons = append(newMons, mon.Name)
				}
				logger.Infof("Found a new monitor! monitors=%v", newMons)
				return
			}

			assert.Equal(suite.T(), 2, len(deployments))
		}

		time.Sleep(5 * time.Second)
	}

	require.Fail(suite.T(), "giving up waiting for a new monitor")
}
