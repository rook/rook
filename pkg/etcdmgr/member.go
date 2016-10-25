package etcdmgr

import (
	"errors"
	"fmt"
	"log"

	"github.com/rook/rook/pkg/etcdmgr/bootstrap"
	ctx "golang.org/x/net/context"
)

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
