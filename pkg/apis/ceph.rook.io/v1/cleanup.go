/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package v1

const (
	// SanitizeDataSourceZero uses /dev/zero as sanitize source
	SanitizeDataSourceZero SanitizeDataSourceProperty = "zero"

	// SanitizeDataSourceRandom uses `shred's default entropy source
	SanitizeDataSourceRandom SanitizeDataSourceProperty = "random"

	// SanitizeMethodComplete will sanitize everything on the disk
	SanitizeMethodComplete SanitizeMethodProperty = "complete"

	// SanitizeMethodQuick will sanitize metadata only on the disk
	SanitizeMethodQuick SanitizeMethodProperty = "quick"

	// DeleteDataDirOnHostsConfirmation represents the validation to destroy dataDirHostPath
	DeleteDataDirOnHostsConfirmation CleanupConfirmationProperty = "yes-really-destroy-data"
)

// HasDataDirCleanPolicy returns whether the cluster has a data dir policy
func (c *CleanupPolicySpec) HasDataDirCleanPolicy() bool {
	return c.Confirmation == DeleteDataDirOnHostsConfirmation
}

func (c *SanitizeMethodProperty) String() string {
	return string(*c)
}

func (c *SanitizeDataSourceProperty) String() string {
	return string(*c)
}
