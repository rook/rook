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
package main

import (
	"fmt"
	"github.com/rook/rook/pkg/daemon/discover"
	"github.com/spf13/cobra"
)

var discoverCmd = &cobra.Command{
	Use:    "discover",
	Short:  "Discover devices",
	Hidden: true,
}

func init() {
	discoverCmd.RunE = startDiscover
}

func startDiscover(cmd *cobra.Command, args []string) error {
	setLogLevel()

	logStartupInfo(discoverCmd.Flags())

	clientset, _, rookClientset, err := getClientset()
	if err != nil {
		terminateFatal(fmt.Errorf("failed to init k8s client. %+v\n", err))
	}

	context := createContext()
	context.Clientset = clientset
	context.RookClientset = rookClientset

	err = discover.Run(context)
	if err != nil {
		terminateFatal(err)
	}

	return nil
}
