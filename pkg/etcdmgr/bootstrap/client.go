package bootstrap

import (
	"errors"
	"fmt"
	"log"
	"os"
)

// GetEtcdClients bootstraps an embedded etcd instance and returns a list of
// current etcd cluster's client URLs. (entrypoint, when it's used as a library)
func GetEtcdClients(configDir, token, ipAddr, nodeID string) ([]string, error) {
	// GODEBUG setting forcing use of Go's resolver
	os.Setenv("GODEBUG", "netdns=go+1")

	full, err, currentNodes := isQuorumFull(token)
	//currentNodes, err := GetCurrentNodesFromDiscovery(token)
	if err != nil {
		return []string{}, errors.New("error querying discovery service")
	}
	log.Println("current etcd cluster nodes: ", currentNodes)

	localURL := fmt.Sprintf("http://%s:%d", ipAddr, DefaultClientPort)

	// Is it a restart scenario?
	restart := false
	log.Println("current localURL: ", localURL)
	for _, node := range currentNodes {
		if node == localURL {
			log.Println("restart scenario detected.")
			restart = true
		}
	}

	if full && !restart {
		log.Println("quorum is already formed, returning current cluster members: ", currentNodes)
		return currentNodes, nil
	}

	log.Println("quorum is not complete, creating a new embedded etcd member...")
	conf, err := generateConfig(configDir, ipAddr, nodeID)
	if err != nil {
		return []string{}, err
	}
	log.Println("conf:", conf)

	factory := EmbeddedEtcdFactory{}
	ee, err := factory.NewEmbeddedEtcd(token, conf, true)
	if err != nil {
		return nil, err
	}

	return ee.Server.Cluster().ClientURLs(), nil
}
