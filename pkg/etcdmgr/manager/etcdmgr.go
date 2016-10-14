package manager

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/quantum/castle/pkg/etcdmgr/bootstrap"
	ctx "golang.org/x/net/context"
)

// GetEtcdClients bootstraps an embedded etcd instance and returns a list of
// current etcd cluster's client URLs. (entrypoint, when it's used as a library)
func GetEtcdClients(configDir, token, ipAddr, nodeID string) ([]string, error) {
	// GODEBUG setting forcing use of Go's resolver
	os.Setenv("GODEBUG", "netdns=go+1")

	full, err, currentNodes := bootstrap.IsQuorumFull(token)
	//currentNodes, err := bootstrap.GetCurrentNodesFromDiscovery(token)
	if err != nil {
		return []string{}, errors.New("error querying discovery service")
	}
	log.Println("current etcd cluster nodes: ", currentNodes)

	localURL := "http://" + ipAddr + ":" + bootstrap.DefaultClientPort
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
	conf, err := bootstrap.GenerateConfig(configDir, ipAddr, nodeID)
	if err != nil {
		return []string{}, err
	}
	log.Println("conf:", conf)

	factory := bootstrap.EmbeddedEtcdFactory{}
	ee, err := factory.NewEmbeddedEtcd(token, conf, true)
	if err != nil {
		return nil, err
	}

	return ee.Server.Cluster().ClientURLs(), nil
}

// GetEtcdClientsWithConfig bootstraps an embedded etcd instance based on a given config and returns
// a list of current etcd cluster's client urls. (used in CLI scenarios).
func GetEtcdClientsWithConfig(token string, config bootstrap.Config) ([]string, error) {
	// GODEBUG setting forcing use of Go's resolver
	os.Setenv("GODEBUG", "netdns=go+1")
	full, err, currentNodes := bootstrap.IsQuorumFull(token)
	if err != nil {
		return []string{}, errors.New("error in querying discovery service")
	}

	//TODO refactor + we assumed we only have one advertiseClient
	localURL := "http://" + config.AdvertiseClientURLs[0].Host
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
		log.Println("quorum is full, returning current quorum nodes...")
		log.Println("current nodes: ", currentNodes)
		return currentNodes, nil
	}

	log.Println("creating a new embedded etcd")
	factory := bootstrap.EmbeddedEtcdFactory{}
	ee, err := factory.NewEmbeddedEtcd(token, &config, true)
	if err != nil {
		return nil, err
	}
	return ee.Server.Cluster().ClientURLs(), nil
}

// AddMember adds a node to the etcd cluster as a member
func AddMember(context bootstrap.EtcdMgrContext, node string) error {
	log.Println("Adding a node to the etcd cluster as a member: ", node)
	mapi, err := context.MembersAPI()
	if err != nil {
		return fmt.Errorf("error in Context.MembersAPI(). %+v", err)
	}
	member, err := mapi.Add(ctx.Background(), node)
	if err != nil {
		return err
	}
	log.Println("New member added: ", *member)
	return nil
}

// RemoveMember removes a member from the etcd cluster
func RemoveMember(context bootstrap.EtcdMgrContext, node string) error {
	log.Println("removing a member from the etcd cluster: ", node)
	mapi, err := context.MembersAPI()
	if err != nil {
		return err
	}
	// Get the list of members.
	members, err := mapi.List(ctx.Background())
	if err != nil {
		return err
	}
	log.Println("members: ", members)
	var nodeID string
	for _, m := range members {
		// PeerURLs is taken from the etcd's terminology. It means what endpoints a member use to communicates by its peers.
		// In our implementation, we always use 1 endpoint for each member. In some cases, someone could bind to localhost
		// too which is not useful for us and we don't do that.
		if sliceContains(m.PeerURLs, node) {
			nodeID = m.ID
		}
	}
	if nodeID == "" {
		return errors.New("node not found")
	}
	err = mapi.Remove(ctx.Background(), nodeID)
	if err != nil {
		return err
	}
	log.Println("New member removed: ", node)
	return nil
}

func sliceContains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}
