/*
Copyright 2022 The Rook Authors. All rights reserved.

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

package kms

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitKMIP(t *testing.T) {
	type args struct {
		config map[string]string
	}
	tests := []struct {
		name string
		args args
		want *kmipKMS
		err  error
	}{
		{
			name: "endpoint not set",
			args: args{
				config: map[string]string{},
			},
			want: nil,
			err:  ErrKMIPEndpointNotSet,
		},
		{
			name: "ca cert not set",
			args: args{
				config: map[string]string{
					kmipEndpoint: "pykimp.local",
				},
			},
			want: nil,
			err:  ErrKMIPCACertNotSet,
		},
		{
			name: "client cert not set",
			args: args{
				config: map[string]string{
					kmipEndpoint: "pykimp.local",
					KmipCACert:   "abcd",
				},
			},
			want: nil,
			err:  ErrKMIPClientCertNotSet,
		},
		{
			name: "client key not set",
			args: args{
				config: map[string]string{
					kmipEndpoint:   "pykimp.local",
					KmipCACert:     "abcd",
					KmipClientCert: "abcd",
				},
			},
			want: nil,
			err:  ErrKMIPClientKeyNotSet,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := InitKMIP(tt.args.config)
			assert.Equal(t, tt.err, err)
		})
	}
}
