package clusterd

import "time"

const (
	RefreshEventName    = "refresh"
	AddNodeEventName    = "add-node"
	RemoveNodeEventName = "remove-node"
	StaleNodeEventName  = "stale-node"
)

// LeaderEvent interface implemented by all events
type LeaderEvent interface {
	Name() string
	Context() *Context
}

// Refresh event. The leader should update all the nodes in the cluster in case something changed.
type RefreshEvent struct {
	context *Context
}

func NewRefreshEvent(c *Context) *RefreshEvent {
	return &RefreshEvent{context: c}
}

func (e *RefreshEvent) Name() string {
	return RefreshEventName
}
func (e *RefreshEvent) Context() *Context {
	return e.context
}

// AddNode event
type AddNodeEvent struct {
	nodes   []string
	context *Context
}

func NewAddNodeEvent(c *Context, nodes []string) *AddNodeEvent {
	return &AddNodeEvent{context: c, nodes: nodes}
}
func (e *AddNodeEvent) Name() string {
	return AddNodeEventName
}
func (e *AddNodeEvent) Nodes() []string {
	return e.nodes
}
func (e *AddNodeEvent) Context() *Context {
	return e.context
}

// RemoveNode event. All services must be immediately rebalanced off this node.
type RemoveNodeEvent struct {
	nodes   []string
	context *Context
}

func NewRemoveNodeEvent(c *Context, nodes []string) *RemoveNodeEvent {
	return &RemoveNodeEvent{context: c, nodes: nodes}
}
func (e *RemoveNodeEvent) Name() string {
	return RemoveNodeEventName
}
func (e *RemoveNodeEvent) Nodes() []string {
	return e.nodes
}
func (e *RemoveNodeEvent) Context() *Context {
	return e.context
}

// StaleNode event. The node has not heartbeated recently.
type StaleNodeEvent struct {
	nodes   []string
	context *Context
	age     time.Duration
}

func NewStaleNodeEvent(c *Context, nodes []string, age time.Duration) *StaleNodeEvent {
	return &StaleNodeEvent{context: c, nodes: nodes, age: age}
}
func (e *StaleNodeEvent) Name() string {
	return StaleNodeEventName
}
func (e *StaleNodeEvent) Nodes() []string {
	return e.nodes
}
func (e *StaleNodeEvent) Context() *Context {
	return e.context
}
func (e *StaleNodeEvent) Age() time.Duration {
	return e.age
}
