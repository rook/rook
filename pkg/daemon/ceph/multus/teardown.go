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
	"os"

	"github.com/pkg/errors"
)

func Teardown() error {
	logger.Info("cleaning up multus link from host network namespace")

	multusIpStr, found := os.LookupEnv(multusIpEnv)
	if !found {
		return errors.Errorf("failed to get value for environment variable %q", multusIpEnv)
	}

	migrated, linkName, err := checkMigration(multusIpStr)
	if err != nil {
		return errors.Wrap(err, "failed to check if the interface has already been removed")
	}
	if !migrated {
		logger.Info("interface already removed; exiting")
		return nil
	}

	logger.Info("removing interface %q", linkName)
	err = removeInterface(linkName)
	if err != nil {
		return errors.Wrapf(err, "failed to remove multus interface %q", linkName)
	}

	return nil
}
