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
package api

import (
	"fmt"
	"net/http"

	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	monop "github.com/rook/rook/pkg/operator/mon"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/kubernetes/pkg/api"
)

const (
	registerMetricsRetryMs = 5000
)

type Config struct {
	context     *clusterd.Context
	port        int
	clusterInfo *mon.ClusterInfo
	namespace   string
	versionTag  string
	hostNetwork bool
}

func NewConfig(context *clusterd.Context, port int, clusterInfo *mon.ClusterInfo, namespace, versionTag string, hostNetwork bool) *Config {
	return &Config{
		context:     context,
		port:        port,
		clusterInfo: clusterInfo,
		namespace:   namespace,
		versionTag:  versionTag,
		hostNetwork: hostNetwork,
	}
}

func Run(context *clusterd.Context, c *Config) error {
	// write the latest config to the config dir
	if err := mon.GenerateAdminConnectionConfig(context, c.clusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config. %+v", err)
	}

	go WatchMonConfig(context, c)
	ServeRoutes(context, c)
	return nil
}

func WatchMonConfig(context *clusterd.Context, c *Config) {
	opts := metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(api.ObjectNameField, monop.EndpointConfigMapName).String(),
	}
	w, err := context.Clientset.Core().ConfigMaps(c.namespace).Watch(opts)
	if err != nil {
		logger.Errorf("API server init error: %+v", err)
	}
	defer w.Stop()

	for {
		e := <-w.ResultChan()
		if e.Type == watch.Modified {
			// write the latest config to the config dir
			if err := mon.GenerateAdminConnectionConfig(context, c.clusterInfo); err != nil {
				logger.Errorf("failed to write connection config. %+v", err)
				return
			}
		}
	}
}

func ServeRoutes(context *clusterd.Context, c *Config) {
	// set up routes and start HTTP server for REST API
	h := newHandler(context, c)

	// register metrics collection in a goroutine so it does not block the start up of the API server.
	go func() {
		if err := h.RegisterMetrics(registerMetricsRetryMs); err != nil {
			logger.Errorf("API server init error: %+v", err)
		}
	}()
	defer h.Shutdown()

	r := newRouter(h.GetRoutes())
	if err := http.ListenAndServe(fmt.Sprintf(":%d", c.port), r); err != nil {
		logger.Errorf("API server error: %+v", err)
	}
}
