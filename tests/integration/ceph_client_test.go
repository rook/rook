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

package integration

import (
	"time"

	"github.com/stretchr/testify/require"
)

// Smoke Test for Client CRD
func (suite *SmokeSuite) TestCreateClient() {
	logger.Infof("Create Client Smoke Test")

	clientName := "client1"
	caps := map[string]string{
		"mon": "allow rwx",
		"mgr": "allow rwx",
		"osd": "allow rwx",
	}
	err := suite.helper.UserClient.Create(clientName, suite.namespace, caps)
	require.Nil(suite.T(), err)

	clientFound := false

	for i := 0; i < 30; i++ {
		clients, _ := suite.helper.UserClient.Get(suite.namespace, "client."+clientName)
		if clients != "" {
			clientFound = true
		}

		if clientFound {
			break
		}

		logger.Infof("Waiting for client to appear")
		time.Sleep(2 * time.Second)
	}

	require.Equal(suite.T(), true, clientFound, "client not found")

	logger.Infof("Update Client Smoke Test")
	newcaps := map[string]string{
		"mon": "allow r",
		"mgr": "allow rw",
		"osd": "allow *",
	}
	caps, _ = suite.helper.UserClient.Update(suite.namespace, clientName, newcaps)

	require.Equal(suite.T(), "allow r", caps["mon"], "wrong caps")
	require.Equal(suite.T(), "allow rw", caps["mgr"], "wrong caps")
	require.Equal(suite.T(), "allow *", caps["osd"], "wrong caps")
}
