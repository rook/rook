/*
Copyright 2017 The Rook Authors. All rights reserved.

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
	"fmt"
	"time"

	"github.com/stretchr/testify/require"
)

// Smoke Test for pool Resizing
func (suite *SmokeSuite) TestPoolResize() {
	logger.Infof("Pool Resize Smoke Test")

	poolName := "testpool"
	err := suite.helper.PoolClient.Create(poolName, suite.namespace, 1)
	require.Nil(suite.T(), err)

	poolFound := false

	// Wait for pool to appear
	for i := 0; i < 10; i++ {
		pools, err := suite.helper.PoolClient.ListCephPools(suite.namespace)
		require.Nil(suite.T(), err)
		for _, p := range pools {
			if p.Name != poolName {
				continue
			}
			poolFound = true
		}
		if poolFound {
			break
		}

		logger.Infof("Waiting for pool to appear")
		time.Sleep(2 * time.Second)
	}

	require.Equal(suite.T(), true, poolFound, "pool not found")

	err = suite.helper.PoolClient.Update(poolName, suite.namespace, 3)
	require.Nil(suite.T(), err)

	poolFound = false
	// Wait for pool resize to happen
	for i := 0; i < 10; i++ {

		details, err := suite.helper.PoolClient.GetCephPoolDetails(suite.namespace, poolName)
		require.Nil(suite.T(), err)
		if details.Size > 1 {
			logger.Infof("pool %s size got updated", poolName)
			require.Equal(suite.T(), 3, int(details.Size))
			poolFound = true
			break
		}
		logger.Infof("pool %s size not updated yet. details: %+v", poolName, details)

		logger.Infof("Waiting for pool %s resize to happen", poolName)
		time.Sleep(2 * time.Second)
	}

	require.Equal(suite.T(), true, poolFound, fmt.Sprintf("pool %s not found", poolName))
}
