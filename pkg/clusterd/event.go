package clusterd

const (
	RefreshEventName       = "refresh"
	AddNodeEventName       = "add-node"
	RemoveNodeEventName    = "remove-node"
	UnhealthyNodeEventName = "unhealthy-node"
)

// LeaderEvent interface implemented by all events
type LeaderEvent interface {
	Name() string
	Context() *Context
}

// Refresh event. The leader should update all the nodes in the cluster in case something changed.
type RefreshEvent struct {
	context *Context
	nodeID  string
}

func NewRefreshEvent(c *Context) *RefreshEvent {
	return &RefreshEvent{context: c}
}

func (e *RefreshEvent) Name() string {
	return RefreshEventName
}
func (e *RefreshEvent) NodeID() string {
	return e.nodeID
}
func (e *RefreshEvent) Context() *Context {
	return e.context
}

// AddNode event
type AddNodeEvent struct {
	nodeID  string
	context *Context
}

func NewAddNodeEvent(c *Context, nodeID string) *AddNodeEvent {
	return &AddNodeEvent{context: c, nodeID: nodeID}
}
func (e *AddNodeEvent) Name() string {
	return AddNodeEventName
}
func (e *AddNodeEvent) Node() string {
	return e.nodeID
}
func (e *AddNodeEvent) Context() *Context {
	return e.context
}

// RemoveNode event. All services must be immediately rebalanced off this node.
type RemoveNodeEvent struct {
	nodeID  string
	context *Context
}

func NewRemoveNodeEvent(c *Context, nodeID string) *RemoveNodeEvent {
	return &RemoveNodeEvent{context: c, nodeID: nodeID}
}
func (e *RemoveNodeEvent) Name() string {
	return RemoveNodeEventName
}
func (e *RemoveNodeEvent) Node() string {
	return e.nodeID
}
func (e *RemoveNodeEvent) Context() *Context {
	return e.context
}

// UnhealthyNode event. The node has not heartbeated recently.
type UnhealthyNodeEvent struct {
	nodes   []*UnhealthyNode
	context *Context
}

func NewUnhealthyNodeEvent(c *Context, nodes []*UnhealthyNode) *UnhealthyNodeEvent {
	return &UnhealthyNodeEvent{context: c, nodes: nodes}
}
func (e *UnhealthyNodeEvent) Name() string {
	return UnhealthyNodeEventName
}
func (e *UnhealthyNodeEvent) Nodes() []*UnhealthyNode {
	return e.nodes
}
func (e *UnhealthyNodeEvent) Context() *Context {
	return e.context
}

type UnhealthyNode struct {
	AgeSeconds int
	NodeID     string
}
