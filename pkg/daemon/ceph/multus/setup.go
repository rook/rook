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
	"errors"
	"fmt"
	"os"

	"github.com/containernetworking/plugins/pkg/ns"
)

func Setup() error {
	logger.Info("determining multus ip")
	multusIpStr, found := os.LookupEnv(multusIpEnv)
	if !found {
		return fmt.Errorf("environment variable %s not set.", multusIpEnv)
	}
	logger.Infof("multus ip: %s", multusIpStr)

	logger.Info("determining multus link")
	multusLinkName, found := os.LookupEnv(multusLinkEnv)
	if !found {
		return fmt.Errorf("environment variable %s not set.", multusLinkEnv)
	}
	logger.Infof("multus link: %s", multusLinkName)

	logger.Info("checking if interface has already been migrated")
	migrated, _, err := checkMigration(multusIpStr)
	if err != nil {
		return err
	}
	if migrated {
		logger.Info("interface already migrated. exiting.")
		return nil
	}

	logger.Info("determining holder network namespace id")
	holderNS, err := determineHolderNS()
	if err != nil {
		return err
	}

	logger.Info("determining new multus interface name")
	newLinkName, err := determineNewLinkName()
	if err != nil {
		return err
	}

	logger.Info("determining multus ip address configuration")
	multusIP, err := determineMultusIPConfig(holderNS, multusIpStr, multusLinkName)
	if err != nil {
		return err
	}

	logger.Info("migrating interface to host network namespace")
	hostNS, err := ns.GetCurrentNS()
	if err != nil {
		return err
	}
	err = migrateInterface(hostNS, holderNS, multusLinkName, newLinkName)
	if err != nil {
		return err
	}

	logger.Info("setting up interface on host network namespace")
	// When the interface is moved to the host network namespace, the IP address isn't carried with it,
	// so the interface needs to be reconfigured after it has been moved.
	// The IP address is therefore passed to set up the interface.
	err = setupInterface(newLinkName, multusIP)
	if err != nil {
		return err
	}

	logger.Info("verifying that the interface has been migrated")
	migrated, _, err = checkMigration(multusIpStr)
	if err != nil {
		return err
	}
	if migrated {
		logger.Info("interface migration verified. ")
	} else {
		return errors.New("interface migration not validated.")
	}

	logger.Info("interface migration complete!")

	return nil
}
