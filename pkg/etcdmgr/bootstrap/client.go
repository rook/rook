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
package bootstrap

import "fmt"

// GetEtcdClients bootstraps an embedded etcd instance and returns a list of
// current etcd cluster's client URLs. (entrypoint, when it's used as a library)
func GetEtcdClients(configDir, token, ipAddr, nodeID string) ([]string, error) {

	full, currentNodes, err := isQuorumFull(token)
	if err != nil {
		return nil, fmt.Errorf("error querying discovery service. %+v", err)
	}
	logger.Infof("current etcd cluster nodes: %+v", currentNodes)

	localURL := fmt.Sprintf("http://%s:%d", ipAddr, DefaultClientPort)

	// Is it a restart scenario?
	restart := false
	logger.Infof("current localURL: %s", localURL)
	for _, node := range currentNodes {
		if node == localURL {
			logger.Infof("restart scenario detected.")
			restart = true
		}
	}

	if full && !restart {
		logger.Infof("quorum is already formed, returning current cluster members: %+v", currentNodes)
		return currentNodes, nil
	}

	logger.Infof("quorum is not complete, creating a new embedded etcd member...")
	conf, err := generateConfig(configDir, ipAddr, nodeID)
	if err != nil {
		return []string{}, err
	}
	logger.Infof("conf: %+v", conf)

	factory := EmbeddedEtcdFactory{}
	ee, err := factory.NewEmbeddedEtcd(token, conf, true)
	if err != nil {
		return nil, fmt.Errorf("failed to start embedded etcd. %+v", err)
	}

	return ee.Server.Cluster().ClientURLs(), nil
}
