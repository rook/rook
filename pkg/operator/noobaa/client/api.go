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

// Package client implements noobaa RPC API client.
package client

import "github.com/coreos/pkg/capnslog"

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "noobaa/client")

// CreateSystemAPI is a noobaa api call to create the system
type CreateSystemAPI struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Response struct {
		RPCResponseBody `json:",inline"`
		Reply           struct {
			Token string `json:"token"`
		} `json:"reply"`
	} `json:"-"`
}

func (r *CreateSystemAPI) build() (*RPCRequestBody, RPCResponse) {
	return &RPCRequestBody{API: "system_api", Method: "create_system", Params: r}, &r.Response
}

// ListAccountsAPI is a noobaa api call to list all accounts
type ListAccountsAPI struct {
	Response struct {
		RPCResponseBody `json:",inline"`
		Reply           struct {
			Accounts []struct {
				Name       string `json:"name"`
				Email      string `json:"email"`
				AccessKeys []struct {
					AccessKey string `json:"access_key"`
					SecretKey string `json:"secret_key"`
				} `json:"access_keys"`
			} `json:"accounts"`
		} `json:"reply"`
	} `json:"-"`
}

func (r *ListAccountsAPI) build() (*RPCRequestBody, RPCResponse) {
	return &RPCRequestBody{API: "account_api", Method: "list_accounts", Params: r}, &r.Response
}

// Compile time assertions that APIs implement RPCRequest interface
var _ RPCRequest = &CreateSystemAPI{}
var _ RPCRequest = &ListAccountsAPI{}
