package cmd

import (
	"fmt"
	"log"

	"github.com/quantum/castle/pkg/util"
	"github.com/spf13/cobra"
)

var (
	content string
)

var writeCmd = &cobra.Command{
	Use:   "write",
	Short: "Writes the given object and content to ceph storage",
}

func init() {
	commonIOCmdInit(writeCmd)

	writeCmd.Flags().StringVar(&content, "content", "", "content to write to the specified object (required)")

	writeCmd.MarkFlagRequired("content")

	writeCmd.RunE = writeData
}

func writeData(cmd *cobra.Command, args []string) error {
	if err := util.VerifyRequiredFlags(cmd, []string{"content"}); err != nil {
		return err
	}

	ioctx, err := prepareIOContext()
	if err != nil {
		return err
	}

	// write the object with the given name and content
	log.Printf("performing write")
	if err := ioctx.WriteFull(objectName, []byte(content)); err != nil {
		return fmt.Errorf("failed to write object %s: %+v", objectName, err)
	}
	log.Printf("successfully wrote object %s. content: '%s'", objectName, content)

	return nil
}
