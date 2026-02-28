// Copyright 2018 The prometheus-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package monitoring

// GroupName is set to var instead of const, since this provides the ability for clients importing the module -
// github.com/prometheus-operator/prometheus-operator/pkg/apis to manage the operator's objects in a different
// API group
//
// Use `ldflags` in the client side, e.g.:
// go run -ldflags="-s -X github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring.GroupName=monitoring.example.com" ./example/client/.
var (
	GroupName = "monitoring.coreos.com"
)
