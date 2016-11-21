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
package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/google/uuid"
)

const (
	maxNodeIDLength = 12
	idFilename      = "id"
)

// To ensure a constant node ID between runs, the id will be persisted in the data directory.
func LoadPersistedNodeID(dataDir string) (string, error) {
	id, save, err := loadNodeID(dataDir)
	if err != nil {
		return "", fmt.Errorf("failed to load node id. %+v", err)
	}

	if save {
		err = saveNodeID(dataDir, id)
		if err != nil {
			return "", fmt.Errorf("failed to save node id. %+v", err)
		}
	}

	return id, nil
}

// Attempt to load the node from the data dir. If not found,
// generate a new random id
func loadNodeID(dataDir string) (string, bool, error) {
	// first read the id from the cache
	buf, err := ioutil.ReadFile(path.Join(dataDir, idFilename))
	if err == nil {
		// successfully loaded the id from the file
		id := strings.TrimSpace(string(buf))
		if len(id) == 0 {
			return "", false, fmt.Errorf("the id cannot be empty")
		}

		logger.Infof("loaded id from data dir")
		return id, false, nil
	} else if !os.IsNotExist(err) {
		return "", false, fmt.Errorf("unexpected error reading node id. %+v", err)
	}

	// as the last resort, generate a random id
	logger.Infof("generating a random id")
	return randomID(), true, nil
}

func randomID() string {
	u := strings.Replace(uuid.New().String(), "-", "", -1)
	return trimNodeID(u)
}

func saveNodeID(dataDir, id string) error {
	// cache the requested discovery URL
	if err := ioutil.WriteFile(path.Join(dataDir, idFilename), []byte(id), 0644); err != nil {
		return err
	}

	return nil
}

func trimNodeID(id string) string {
	// Trim the node ID to a length that is statistically unlikely to collide with another node in the cluster
	// while allowing us to use an ID that is both unique and succinct.
	// Using the birthday collision algorithm, if we have a length of 12 hex characters, that gives us
	// 16^12 possibilities. If we have a cluster with 1,000 nodes, we have a likelihood with node IDs
	// colliding in less than 1 in a billion clusters.
	id = strings.TrimSpace(id)
	if len(id) <= maxNodeIDLength {
		return id
	}

	return id[0:maxNodeIDLength]
}
