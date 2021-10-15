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
	gerrors "errors"
	"os"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/pkg/errors"
)

func Setup() error {
	logger.Info("starting multus interface migration")

	multusIpStr, found := os.LookupEnv(multusIpEnv)
	if !found {
		return errors.Errorf("failed to get value for environment variable %q", multusIpEnv)
	}
	logger.Infof("multus ip: %q", multusIpStr)

	multusLinkName, found := os.LookupEnv(multusLinkEnv)
	if !found {
		return errors.Errorf("failed to get value for environment variable %q", multusLinkEnv)
	}
	logger.Infof("multus link: %q", multusLinkName)

	migrated, _, err := checkMigration(multusIpStr)
	if err != nil {
		return errors.Wrapf(err, "failed to check if interface has already been migrated")
	}
	if migrated {
		logger.Info("interface already migrated. exiting.")
		return nil
	}

	holderNS, err := determineHolderNS()
	if err != nil {
		return errors.Wrapf(err, "failed to determine the holder network namespace id")
	}

	newLinkName, err := determineNewLinkName()
	if err != nil {
		return errors.Wrapf(err, "failed to determine the new multus interface name")
	}

	multusIP, err := determineMultusIPConfig(holderNS, multusIpStr, multusLinkName)
	if err != nil {
		return errors.Wrapf(err, "failed to determine the multus ip address configuration")
	}

	hostNS, err := ns.GetCurrentNS()
	if err != nil {
		return errors.Wrapf(err, "failed to get the host network namespace")
	}
	err = migrateInterface(hostNS, holderNS, multusLinkName, newLinkName)
	if err != nil {
		return errors.Wrapf(err, "failed to migrate the interface to the host network namespace")
	}

	logger.Info("setting up interface on host network namespace")
	// When the interface is moved to the host network namespace, the IP address isn't carried with it,
	// so the interface needs to be reconfigured after it has been moved.
	// The IP address is therefore passed to set up the interface.
	err = setupInterface(newLinkName, multusIP)
	if err != nil {
		return errors.Wrapf(err, "failed to set up the multus interface on the host network namespace")
	}

	migrated, _, err = checkMigration(multusIpStr)
	if err != nil {
		return errors.Wrapf(err, "failed to verify interface migration")
	}
	if migrated {
		logger.Info("interface migration verified")
	} else {
		return gerrors.New("interface migration not validated")
	}

	logger.Info("interface migration complete!")

	return nil
}
