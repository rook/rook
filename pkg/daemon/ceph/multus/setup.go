package multus

import (
	"errors"

	"github.com/containernetworking/plugins/pkg/ns"
)

func Setup() error {

	logger.Info("Checking if interface has already been migrated")
	migrated, _, err := checkMigration()
	if err != nil {
		logger.Error(err)
		return err
	}
	if migrated {
		logger.Info("Interface already migrated. Exiting.")
		return nil
	}

	logger.Info("Determining holder network namespace id")
	holderNS, err := determineHolderNS()
	if err != nil {
		logger.Error(err)
		return err
	}

	logger.Info("Determining new multus interface name")
	newLinkName, err := determineNewLinkName()
	if err != nil {
		logger.Error(err)
		return err
	}

	// When the interface is moved to the host network namespace, the IP address isn't carried with it,
	// so the interface needs to be reconfigured after it has been moved.
	// The IP address is therefore saved before migrating the interface.
	logger.Info("Determining multus IP address configuration")
	multusIP, err := determineMultusIP(holderNS)
	if err != nil {
		logger.Error(err)
		return err
	}

	logger.Info("Migrating interface to host network namespace")
	hostNS, err := ns.GetCurrentNS()
	if err != nil {
		logger.Error(err)
		return err
	}
	err = migrateInterface(hostNS, holderNS, newLinkName)
	if err != nil {
		logger.Error(err)
		return err
	}

	logger.Info("Setting up interface on host network namespace")
	err = setupInterface(newLinkName, multusIP)
	if err != nil {
		logger.Error(err)
		return err
	}

	logger.Info("Verifying that the interface has been migrated")
	migrated, _, err = checkMigration()
	if err != nil {
		logger.Error(err)
		return err
	}
	if migrated {
		logger.Info("Interface migration verified. ")
	} else {
		logger.Error("Interface migration not validated.")
		return errors.New("Interface migration not validated.")
	}

	logger.Info("Interface migration complete!")

	return nil
}
