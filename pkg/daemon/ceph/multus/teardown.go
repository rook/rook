package multus

import (
	"github.com/vishvananda/netlink"
)

func Teardown() error {

	logger.Info("Checking if interface has already been removed")
	migrated, linkName, err := checkMigration()
	if err != nil {
		logger.Error(err)
		return err
	}
	if !migrated {
		logger.Info("Interface already removed. Exiting.")
		return nil
	}

	logger.Info("Removing interface")
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return err
	}

	return netlink.LinkDel(link)
}
