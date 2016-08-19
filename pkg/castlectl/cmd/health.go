package cmd

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/quantum/castle/pkg/cephclient"
	"github.com/quantum/castle/pkg/util"
	"github.com/spf13/cobra"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Shows the health of the ceph cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := util.VerifyRequiredFlags(cmd, []string{}); err != nil {
			return err
		}

		// connect to the cluster with the client.admin creds
		adminConn, err := cephclient.ConnectToCluster(clusterName, "client.admin", configFilePath)
		if err != nil {
			return err
		}
		defer adminConn.Shutdown()

		cmdVal := "health"
		command, err := json.Marshal(map[string]interface{}{
			"prefix": cmdVal,
			"format": "json",
			"detail": true,
		})
		if err != nil {
			return fmt.Errorf("command %s marshall failed: %+v", cmdVal, err)
		}

		buf, info, err := adminConn.MonCommand(command)
		if err != nil {
			return fmt.Errorf("mon_command failed: %+v", err)
		}

		log.Printf("health response: info: %s, buffer: %s", info, string(buf[:]))

		return nil
	},
}
