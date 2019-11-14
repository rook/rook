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

/*
Package clusterdisruption implements the controller for ensuring that drains occus in a safe manner.
The design and purpose for clusterdisruption management is found at:
https://github.com/rook/rook/blob/master/design/ceph/ceph-managed-disruptionbudgets.md
*/
package clusterdisruption
