package clusterd

import (
	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/util"
	"github.com/quantum/castle/pkg/util/exec"
	"github.com/quantum/castle/pkg/util/proc"
)

const (
	LeaderElectionKey = "orchestrator-leader"
)

type ClusterService struct {
	Name   string
	Leader ServiceLeader
	Agents []ServiceAgent
}

// Interface implemented by a service that has been elected leader
type ServiceLeader interface {
	// Get the list of etcd keys that when changed should trigger an orchestration
	RefreshKeys() []*RefreshKey

	// Start a go routine that watches for orchestration events related to the leader
	StartWatchEvents()

	// Get the events channel that the orchestration should write events to
	Events() chan LeaderEvent

	// Close the events channel when leadership is lost
	Close() error
}

type ServiceAgent interface {
	// Get the name of the service agent
	Name() string

	// Initialize the agents from the context, allowing them to store desired state in etcd before orchestration starts.
	Initialize(context *Context) error

	// Configure the service according to the changes requested by the leader
	ConfigureLocalService(context *Context) error

	// Remove a service that is no longer needed
	DestroyLocalService(context *Context) error
}

// The context for loading or applying the configuration state of a service.
type Context struct {
	// The registered services for cluster configuration
	Services []*ClusterService

	// The latest inventory information
	Inventory *inventory.Config

	// The local node ID
	NodeID string

	// The etcd client for get/set config values
	EtcdClient etcd.KeysAPI

	// The implementation of executing a console command
	Executor exec.Executor

	// The process manager for launching a process
	ProcMan *proc.ProcManager

	// The root configuration directory used by services
	ConfigDir string
}

func copyContext(c *Context) *Context {
	return &Context{
		Services:   c.Services,
		NodeID:     c.NodeID,
		EtcdClient: c.EtcdClient,
		Executor:   c.Executor,
		ProcMan:    c.ProcMan,
		Inventory:  c.Inventory,
		ConfigDir:  c.ConfigDir,
	}
}

func (c *Context) GetExecutor() exec.Executor {
	if c.Executor == nil {
		return &exec.CommandExecutor{}
	}

	return c.Executor
}

func (c *Context) GetEtcdClient() (etcd.KeysAPI, error) {
	if c.EtcdClient == nil {
		return util.GetEtcdClientFromEnv()
	}

	return c.EtcdClient, nil
}
