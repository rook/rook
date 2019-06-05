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

package cassandra

import (
	"github.com/coreos/pkg/capnslog"
	"github.com/spf13/cobra"
)

// Cmd exports cobra command according to the cobra documentation.
var Cmd = &cobra.Command{
	Use:   "cassandra",
	Short: "Main command for cassandra controller pod.",
}

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "cassandracmd")
)

func init() {
	Cmd.AddCommand(operatorCmd, sidecarCmd)
}
