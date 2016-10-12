package bootstrap

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/coreos/etcd/client"
	"github.com/coreos/etcd/store"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

// QuorumFormed tries to connect to the etcd cluster. If it fails it interprets it as an incomplete quorum.
func QuorumFormed(currentNodes []string) bool {
	_, err := client.New(client.Config{
		Endpoints:               currentNodes,
		Transport:               client.DefaultTransport,
		HeaderTimeoutPerRequest: DefaultClientTimeout,
	})
	if err != nil {
		return false
	}
	return true
}

func IsQuorumFull(token string) (bool, error, []string) {
	res, err := QueryDiscoveryService(token + "/_config/size")
	if err != nil {
		return false, fmt.Errorf("cannot get discovery url cluster size: %v", err), []string{}
	}

	size, _ := strconv.ParseInt(*res.Node.Value, 10, 16)
	clusterSize := int(size)
	log.Println("cluster max size is: ", clusterSize)

	currentNodes, _ := GetCurrentNodesFromDiscovery(token)
	log.Println("currentNodes: ", currentNodes)
	if len(currentNodes) < clusterSize {
		return false, nil, []string{}
	}
	return true, nil, currentNodes

}

// QueryDiscoveryService reads a key from a discovery url.
func QueryDiscoveryService(token string) (*store.Event, error) {
	ctx, _ := context.WithTimeout(context.Background(), DefaultClientTimeout)
	resp, err := ctxhttp.Get(ctx, http.DefaultClient, token)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("status code %d from %q: %s", resp.StatusCode, token, body)
	}

	var res store.Event
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return nil, fmt.Errorf("invalid answer from %q: %v", token, err)
	}

	return &res, nil
}

func GetCurrentNodesFromDiscovery(token string) ([]string, error) {
	res, err := QueryDiscoveryService(token)
	if err != nil {
		return nil, err
	}
	nodes := make([]string, 0, len(res.Node.Nodes))
	for _, nn := range res.Node.Nodes {
		if nn.Value == nil {
			log.Printf("Skipping %q because no value exists", nn.Key)
		}
		clientPort, _ := strconv.Atoi(DefaultClientPort)
		n, err := newDiscoveryNode(*nn.Value, clientPort)
		if err != nil {
			log.Printf("invalid peer url %q in discovery service: %v", *nn.Value, err)
			continue
		}
		for _, node := range n {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

type Machine struct {
	client.Member
}

func newDiscoveryNode(namedPeerURLs string, clientPort int) ([]string, error) {
	urls := strings.Split(namedPeerURLs, ",")
	n := Machine{
		Member: client.Member{
			PeerURLs:   make([]string, 0, len(urls)),
			ClientURLs: make([]string, 0, len(urls)),
		},
	}
	for _, namedPeerURL := range urls {
		eqc := strings.SplitN(namedPeerURL, "=", 2)
		if n.Name != "" && n.Name != eqc[0] {
			return nil, fmt.Errorf("different names in %s", namedPeerURLs)
		}
		n.Name = eqc[0]
		colc := strings.SplitN(eqc[1], ":", 3)
		n.PeerURLs = append(n.PeerURLs, eqc[1])
		n.ClientURLs = append(n.ClientURLs, fmt.Sprintf("%s:%s:%d", colc[0], colc[1], clientPort))
	}

	return n.ClientURLs, nil
}
