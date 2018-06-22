/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package operator to manage Minio object storage.
package minio

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/rook/rook/pkg/clusterd"
	"k8s.io/api/core/v1"
)

// Operator type for managing object storage.
type Operator struct {
	context    *clusterd.Context
	rookImage  string
	controller *MinioController
}

// New creates an operator instance.
func New(context *clusterd.Context, rookImage string) *Operator {
	minioController := NewMinioController(context, rookImage)

	return &Operator{
		context:    context,
		rookImage:  rookImage,
		controller: minioController,
	}
}

// Run the operator instance.
func (o *Operator) Run() error {
	signalChan := make(chan os.Signal, 1)
	stopChan := make(chan struct{})
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// Watch for changes to the object stores.
	o.controller.StartWatch(v1.NamespaceAll, stopChan)
	logger.Infof("Started watch for minio object stores")

	for {
		select {
		case <-signalChan:
			logger.Infof("shutdown signal received, exiting...")
			close(stopChan)
			return nil
		}
	}
}
