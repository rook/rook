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
package etcd

import (
	"errors"
	"fmt"

	"github.com/rook/rook/pkg/etcd/bootstrap"
	ctx "golang.org/x/net/context"
)

// AddMember adds a node to the etcd cluster as a member
func AddMember(context bootstrap.EtcdMgrContext, node string) error {
	logger.Infof("Adding a node to the etcd cluster as a member: %s", node)
	mapi, err := context.MembersAPI()
	if err != nil {
		return fmt.Errorf("error in Context.MembersAPI(). %+v", err)
	}
	member, err := mapi.Add(ctx.Background(), node)
	if err != nil {
		return err
	}
	logger.Infof("New member added: %+v", *member)
	return nil
}

// RemoveMember removes a member from the etcd cluster
func RemoveMember(context bootstrap.EtcdMgrContext, node string) error {
	logger.Infof("removing a member from the etcd cluster: %s", node)
	mapi, err := context.MembersAPI()
	if err != nil {
		return err
	}
	// Get the list of members.
	members, err := mapi.List(ctx.Background())
	if err != nil {
		return err
	}
	logger.Infof("members: %+v", members)
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
	logger.Infof("New member removed: %s", node)
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
