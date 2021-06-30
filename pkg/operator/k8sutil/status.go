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

package k8sutil

const (
	// ReadyStatus reflects the completeness of tasks for ceph related CRs
	ReadyStatus = "Ready"
	// ProcessingStatus reflects that the tasks are in progress for ceph related CRs
	ProcessingStatus = "Processing"
	// FailedStatus reflects that some task failed for ceph related CRs
	FailedStatus = "Failed"
	// ReconcilingStatus indicates the CR is reconciling
	ReconcilingStatus = "Reconciling"
	// ReconcileFailedStatus indicates a reconciliation failed
	ReconcileFailedStatus = "ReconcileFailed"
	// EmptyStatus indicates the object just got created
	EmptyStatus = ""
)
