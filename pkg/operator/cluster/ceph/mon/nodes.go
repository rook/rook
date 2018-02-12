package mon

import (
	"fmt"

	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getNodes detect the nodes that are available for new mons to start.
func (c *Cluster) getNodes() ([]v1.Node, error) {
	availableNodes, err := c.getAvailableMonNodes()
	if err != nil {
		return nil, err
	}

	if len(availableNodes) == 0 {
		return nil, fmt.Errorf("no nodes are available for mons")
	}

	return availableNodes, nil
}

func (c *Cluster) getAvailableMonNodes() ([]v1.Node, error) {
	nodeOptions := metav1.ListOptions{}
	nodeOptions.TypeMeta.Kind = "Node"
	nodes, err := c.context.Clientset.CoreV1().Nodes().List(nodeOptions)
	if err != nil {
		return nil, err
	}

	// get the nodes that have mons assigned
	nodesInUse, err := k8sutil.GetNodesWithApp(c.context.Clientset, c.Namespace, appName)
	if err != nil {
		logger.Warningf("could not get nodes with mons. %+v", err)
		nodesInUse = util.NewSet()
	}

	// choose nodes for the new mons that don't have mons currently
	availableNodes := []v1.Node{}
	for _, node := range nodes.Items {
		if !nodesInUse.Contains(node.Name) && validNode(node, c.placement) {
			availableNodes = append(availableNodes, node)
		}
	}

	return availableNodes, nil
}
