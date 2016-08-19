package cmd

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/quantum/castle/pkg/cephclient"
	"github.com/quantum/castle/pkg/util"
	"github.com/spf13/cobra"
)

var monCommandVal string

func init() {
	monCommandCmd.Flags().StringVar(&monCommandVal, "mon-command-value", "", "mon_commmand to run (required)")

	monCommandCmd.MarkFlagRequired("monitor-name")

	monCommandCmd.RunE = monCommand
}

var monCommandCmd = &cobra.Command{
	Use:   "mon-cmd",
	Short: "Runs the given arbitrary mon_command with no arguments",
}

func monCommand(cmd *cobra.Command, args []string) error {
	if err := util.VerifyRequiredFlags(cmd, []string{"mon-command-value"}); err != nil {
		return err
	}

	// connect to the cluster with the client.admin creds
	adminConn, err := cephclient.ConnectToCluster(clusterName, "client.admin", configFilePath)
	if err != nil {
		return err
	}
	defer adminConn.Shutdown()

	cmdVal := monCommandVal
	command, err := json.Marshal(map[string]interface{}{
		"prefix": cmdVal,
		"format": "json",
	})
	if err != nil {
		return fmt.Errorf("command %s marshall failed: %+v", cmdVal, err)
	}

	buf, info, err := adminConn.MonCommand(command)
	if err != nil {
		return fmt.Errorf("mon_command failed: %+v", err)
	}

	log.Printf("response: info: %s, buffer: %s", info, string(buf[:]))
	return nil
}
