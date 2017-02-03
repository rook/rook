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
package object

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:     "object",
	Aliases: []string{"obj"},
	Short:   "Performs commands and operations on object stores in the cluster",
}

func init() {
	Cmd.AddCommand(createCmd)
	Cmd.AddCommand(connectionCmd)
	Cmd.AddCommand(bucketCmd)
	Cmd.AddCommand(userCmd)
}
