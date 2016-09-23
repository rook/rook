package bootstrap

import (
	"log"

	"github.com/coreos/etcd/client"
	"github.com/coreos/etcd/pkg/types"
	ctx "golang.org/x/net/context"
)

type EtcdMgrContext interface {
	Client(initialNodes []string) (client.Client, error)
	MembersAPI() (client.MembersAPI, error)
	KeysAPI() (client.KeysAPI, error)
	Members() ([]string, types.URLsMap, error)
}

type Context struct {
	ClusterToken string
}

func (e *Context) Client(initialNodes []string) (client.Client, error) {
	c, err := client.New(client.Config{
		Endpoints:               initialNodes,
		Transport:               client.DefaultTransport,
		HeaderTimeoutPerRequest: DefaultClientTimeout,
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// MembersAPI returns an instance of MembersAPI for the current etcd cluster
func (e *Context) MembersAPI() (client.MembersAPI, error) {
	members, _, err := e.Members()
	if err != nil {
		return nil, err
	}
	c, err := e.Client(members)
	if err != nil {
		return nil, err
	}
	mAPI := client.NewMembersAPI(c)
	return mAPI, nil
}

func (e *Context) KeysAPI() (client.KeysAPI, error) {
	members, _, err := e.Members()
	if err != nil {
		return nil, err
	}
	c, err := e.Client(members)
	if err != nil {
		return nil, err
	}
	kAPI := client.NewKeysAPI(c)
	return kAPI, nil
}

func (e *Context) Members() ([]string, types.URLsMap, error) {
	urlsMap := types.URLsMap{}
	var nodes []string
	initialNodes, err := GetCurrentNodesFromDiscovery(e.ClusterToken)
	if err != nil {
		log.Println("error in GetCurrentNodesFromDiscovery")
		return nodes, urlsMap, err
	}

	c, err := e.Client(initialNodes)
	if err != nil {
		return nodes, urlsMap, err
	}

	mAPI := client.NewMembersAPI(c)
	members, err := mAPI.List(ctx.Background())
	if err != nil {
		return nodes, urlsMap, err
	}

	for _, member := range members {
		urls, err := types.NewURLs(member.PeerURLs)
		if err != nil {
			return nodes, urlsMap, err
		}
		urlsMap[member.ID] = urls
		// ClientURLs of a member is a url which is used by this member to listen to the clients' requests. This url could be used
		// to create etcd client objects. In some use cases, multiple urls might be used, but we don't use that pattern.
		nodes = append(nodes, member.ClientURLs...)
	}
	return nodes, urlsMap, nil
}
