/*
Copyright 2023 The Rook Authors. All rights reserved.

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

package multus

import (
	"github.com/rook/rook/cmd/rook/userfacing/multus/validation"
	"github.com/spf13/cobra"
)

func init() {
	Cmd.AddCommand(
		validation.Cmd,
	)
}

// Cmd is the 'multus' CLI command
var Cmd = &cobra.Command{
	Use:   "multus",
	Short: "Get help configuring Multus for compatibility with Rook",
	Long: `
Get help configuring Multus for compatibility with Rook.

Though intended for Multus, these utilities should support any multi-net
provider that implements the Kubernetes Network Plumbing Working Group's
Multi-Network Spec: https://github.com/k8snetworkplumbingwg/multi-net-spec
`,
}
