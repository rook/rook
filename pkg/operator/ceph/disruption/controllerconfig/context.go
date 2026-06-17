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

package controllerconfig

import (
	"context"

	"github.com/rook/rook/pkg/clusterd"
)

// Context passed to the controller when associating it with the manager.
type Context struct {
	ClusterdContext  *clusterd.Context
	OpManagerContext context.Context
}
