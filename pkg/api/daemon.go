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
)

const (
	registerMetricsRetryMs = 5000
)

type Config struct {
	Port        int
	ClusterInfo *mon.ClusterInfo
	ClusterHandler
}

func Run(context *clusterd.Context, config *Config) error {
	// write the latest config to the config dir
	if err := mon.GenerateAdminConnectionConfig(context, config.ClusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config. %+v", err)
	}

	ServeRoutes(context, config)
	return nil
}

func ServeRoutes(context *clusterd.Context, config *Config) {
	// set up routes and start HTTP server for REST API
	h := newHandler(context, config)

	// register metrics collection in a goroutine so it does not block the start up of the API server.
	go func() {
		if err := h.RegisterMetrics(registerMetricsRetryMs); err != nil {
			logger.Errorf("API server init error: %+v", err)
		}
	}()
	defer h.Shutdown()

	r := newRouter(h.GetRoutes())
	if err := http.ListenAndServe(fmt.Sprintf(":%d", config.Port), r); err != nil {
		logger.Errorf("API server error: %+v", err)
	}
}
