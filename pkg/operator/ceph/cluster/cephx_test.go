/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package cluster

import "testing"

func Test_genKeyring(t *testing.T) {
	twoCapsKeyring := `[client.them]
	key = themkey==
	caps any = "thing"
`
	adminRotatorKeyring := `[client.admin-rotator]
	key = adminrotatorkey==
	caps mds = "allow *"
	caps mon = "allow *"
	caps osd = "allow *"
	caps mgr = "allow *"
`

	tests := []struct {
		name       string
		clientName string
		clientKey  string
		clientCaps []string
		wantErr    bool
		want       string
	}{
		{"no caps", "client.me", "mekey==", []string{}, false, "[client.me]\n	key = mekey=="},
		{"no caps, no key", "client.me", "", []string{}, true, ""},
		{"one cap", "client.you", "youkey==", []string{"bad"}, true, ""},
		{"two caps", "client.them", "themkey==", []string{"any", "thing"}, false, twoCapsKeyring},
		{"three caps", "client.batman", "batmankey==", []string{"some", "thing", "else"}, true, ""},
		{"admin rotator caps", "client.admin-rotator", "adminrotatorkey==", adminKeyAccessCaps, false, adminRotatorKeyring},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := genKeyring(tt.clientName, tt.clientKey, tt.clientCaps)
			if (err != nil) != tt.wantErr {
				t.Errorf("genKeyring() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("genKeyring() = %v, want %v", got, tt.want)
			}
		})
	}
}
