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

package v1

func (s *ObjectStoreSpec) IsMultisite() bool {
	return s.Zone.Name != ""
}

func (s *ObjectStoreSpec) IsTLSEnabled() bool {
	return s.Gateway.SecurePort != 0 && s.Gateway.SSLCertificateRef != ""
}

func (s *ObjectStoreSpec) IsExternal() bool {
	return len(s.Gateway.ExternalRgwEndpoints) != 0
}

func (s *ObjectRealmSpec) IsPullRealm() bool {
	return s.Pull.Endpoint != ""
}
