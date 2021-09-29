/*
Copyright 2021 The Rook Authors. All rights reserved.

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
package multus

import (
	"fmt"
	"os"

	"github.com/vishvananda/netlink"
)

func Teardown() error {
	logger.Info("determining multus ip")
	multusIpStr, found := os.LookupEnv(multusIpEnv)
	if !found {
		return fmt.Errorf("environment variable %s not set.", multusIpEnv)
	}

	logger.Info("checking if interface has already been removed")
	migrated, linkName, err := checkMigration(multusIpStr)
	if err != nil {
		return err
	}
	if !migrated {
		logger.Info("interface already removed. exiting.")
		return nil
	}

	logger.Info("removing interface")
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return err
	}

	return netlink.LinkDel(link)
}
