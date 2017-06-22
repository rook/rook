/*
Copyright 2016 The Rook Authors. All rights reserved.

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
package client

import (
	"fmt"
	"strings"

	"github.com/rook/rook/pkg/model"
)

func VerifyClientAccessInfo(clientAccessInfo model.ClientAccessInfo) error {
	if clientAccessInfo.MonAddresses == nil || len(clientAccessInfo.MonAddresses) == 0 {
		return fmt.Errorf("missing mon addresses: %+v", clientAccessInfo)
	}

	if clientAccessInfo.UserName == "" || clientAccessInfo.SecretKey == "" {
		return fmt.Errorf("missing user/secret: %v", clientAccessInfo)
	}

	return nil
}

func ProcessMonAddresses(clientAccessInfo model.ClientAccessInfo) []string {
	monAddrs := make([]string, len(clientAccessInfo.MonAddresses))
	for i, addr := range clientAccessInfo.MonAddresses {
		lastIndex := strings.LastIndex(addr, "/")
		if lastIndex == -1 {
			lastIndex = len(addr)
		}
		monAddrs[i] = addr[0:lastIndex]
	}

	return monAddrs
}
