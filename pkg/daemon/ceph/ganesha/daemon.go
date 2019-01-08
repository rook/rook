/*
Copyright 2019 The Rook Authors. All rights reserved.

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
package ganesha

import (
	"fmt"
	"os"

	"github.com/rook/rook/pkg/clusterd"
)

func Run(context *clusterd.Context, name string) error {

	// create the run dir for dbus to start
	if err := os.Mkdir("/run/dbus", 0755); err != nil {
		logger.Errorf("Couldn't create /run/dbus: %+v", err)
	}

	logger.Infof("Starting dbus daemon")
	if err := context.Executor.ExecuteCommand(false, "", "dbus-daemon", "--system", "--nopidfile"); err != nil {
		logger.Errorf("Failed to start dbus daemon: %+v", err)
	}

	// Run the ganesha process. If the process exits, the Rook process will exit and the pod will be restarted.
	// For debug logging, add the params: "-N", "NIV_DEBUG"
	logger.Infof("running ganesha server %s", name)
	if err := context.Executor.ExecuteCommand(false, "", "ganesha.nfsd", "-F", "-L", "STDOUT"); err != nil {
		return fmt.Errorf("failed to run ganesha. %+v", err)
	}

	return fmt.Errorf("ganesha exited for an unknown reason")
}
