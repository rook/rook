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
package clusterd

import (
	etcd "github.com/coreos/etcd/client"
	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/proc"
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

	// Refresh the service
	HandleRefresh(e *RefreshEvent)
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

	// A value indicating the desired logging/tracing level
	LogLevel capnslog.LogLevel

	// The full path to a config file that can be used to override generated settings
	ConfigFileOverride string

	// Information about the network for this machine and its cluster
	NetworkInfo NetworkInfo
}

// The context for running a rook daemon.
type DaemonContext struct {
	// The implementation of executing a console command
	Executor exec.Executor

	// The process manager for launching a process
	ProcMan *proc.ProcManager

	// The root configuration directory used by services
	ConfigDir string

	// A value indicating the desired logging/tracing level
	LogLevel capnslog.LogLevel

	// The full path to a config file that can be used to override generated settings
	ConfigFileOverride string
}

func copyContext(c *Context) *Context {
	return &Context{
		Services:           c.Services,
		NodeID:             c.NodeID,
		EtcdClient:         c.EtcdClient,
		Executor:           c.Executor,
		ProcMan:            c.ProcMan,
		Inventory:          c.Inventory,
		ConfigDir:          c.ConfigDir,
		LogLevel:           c.LogLevel,
		ConfigFileOverride: c.ConfigFileOverride,
	}
}

func NewDaemonContext(dataDir, cephConfigOverride string, logLevel capnslog.LogLevel) *DaemonContext {
	executor := &exec.CommandExecutor{}
	return &DaemonContext{
		ProcMan:            proc.New(executor),
		Executor:           executor,
		ConfigDir:          dataDir,
		ConfigFileOverride: cephConfigOverride,
		LogLevel:           logLevel,
	}
}

// Convert a context to a daemon context
func ToContext(context *DaemonContext) *Context {
	return &Context{Executor: context.Executor, ProcMan: context.ProcMan, ConfigDir: context.ConfigDir, LogLevel: context.LogLevel, ConfigFileOverride: context.ConfigFileOverride}
}

func ToDaemonContext(context *Context) *DaemonContext {
	return &DaemonContext{ProcMan: context.ProcMan, Executor: context.Executor, ConfigDir: context.ConfigDir, LogLevel: context.LogLevel, ConfigFileOverride: context.ConfigFileOverride}
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
