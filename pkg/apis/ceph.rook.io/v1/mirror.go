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

// HasPeers returns whether the RBD mirror daemon has peer and should connect to it
func (m *MirroringPeerSpec) HasPeers() bool {
	return len(m.SecretNames) != 0
}

func (m *FSMirroringSpec) SnapShotScheduleEnabled() bool {
	return len(m.SnapshotSchedules) != 0
}
