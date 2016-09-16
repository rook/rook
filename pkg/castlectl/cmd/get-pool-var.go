package cmd

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/quantum/castle/pkg/cephclient"
	"github.com/quantum/castle/pkg/cephd"
	"github.com/quantum/castle/pkg/util"
	"github.com/spf13/cobra"
)

var (
	getPoolVarPoolName string
	getPoolVarVarName  string
)

func init() {
	getPoolVarCmd.Flags().StringVar(&getPoolVarPoolName, "pool-name", "defaultPool", "name of storage pool to use (required)")
	getPoolVarCmd.Flags().StringVar(&getPoolVarVarName, "variable", "size", "name of pool variable to get (required)")

	getPoolVarCmd.MarkFlagRequired("variable")

	getPoolVarCmd.RunE = getPoolVar
}

var getPoolVarCmd = &cobra.Command{
	Use:   "get-pool-var",
	Short: "Gets a configuration variable for the given pool",
}

func getPoolVar(cmd *cobra.Command, args []string) error {
	if err := util.VerifyRequiredFlags(cmd, []string{"pool-name", "variable"}); err != nil {
		return err
	}

	// connect to the cluster with the client.admin creds
	adminConn, err := cephclient.ConnectToCluster(cephd.New(), clusterName, "client.admin", configFilePath)
	if err != nil {
		return err
	}
	defer adminConn.Shutdown()

	cmdVal := "osd pool get"
	command, err := json.Marshal(map[string]interface{}{
		"prefix": cmdVal,
		"format": "json",
		"pool":   getPoolVarPoolName,
		"var":    getPoolVarVarName,
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
