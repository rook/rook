package cmd

import (
	"fmt"
	"log"

	"github.com/quantum/castle/pkg/util"
	"github.com/spf13/cobra"
)

var readCmd = &cobra.Command{
	Use:   "read",
	Short: "Reads the given object from ceph storage",
}

func init() {
	commonIOCmdInit(readCmd)
	readCmd.RunE = readData
}

func readData(cmd *cobra.Command, args []string) error {
	if err := util.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	ioctx, err := prepareIOContext()
	if err != nil {
		return err
	}

	// read the object with the given name
	data_out := make([]byte, 100)
	log.Printf("performing read")
	_, err = ioctx.Read(objectName, data_out, 0)
	if err != nil {
		return fmt.Errorf("failed to read object %s: %+v", objectName, err)
	}
	log.Printf("successfully read object %s.  content: '%s'", objectName, string(data_out))

	return nil
}
