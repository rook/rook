package cephclient

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/quantum/castle/pkg/cephd"
)

// connect to the ceph cluster with the given cluster name and user name
func ConnectToCluster(clusterName, user, configFilePath string) (*cephd.Conn, error) {
	log.Printf("connecting to cluster %s with user %s and config file %s", clusterName, user, configFilePath)
	conn, err := cephd.NewConnWithClusterAndUser(clusterName, user)
	if err != nil {
		return nil, fmt.Errorf("failed to create rados connection for cluster %s and user %s: %+v", clusterName, user, err)
	}

	if err = conn.ReadConfigFile(configFilePath); err != nil {
		return nil, fmt.Errorf("failed to read config file for cluster %s from %s: %+v", clusterName, configFilePath, err)
	}

	if err = conn.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to cluster %s: %+v", clusterName, err)
	}

	log.Printf("connected to cluster %s", clusterName)
	return conn, nil
}

// run a single mon_command
func RunMonCommand(conn *cephd.Conn, command []byte) error {
	buf, info, err := conn.MonCommand(command)
	if err != nil {
		return fmt.Errorf("mon_command failed: %+v", err)
	}
	var resp map[string]interface{}
	err = json.Unmarshal(buf, &resp)
	if err != nil {
		return fmt.Errorf("unmarhsall failed: %+v.  raw buffer response: %s", err, string(buf[:]))
	}

	log.Printf("mon_command success! info: %s, resp: %+v", info, resp)
	return nil
}
