package cmd

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/quantum/castle/pkg/cephclient"
	"github.com/quantum/castle/pkg/util"
	"github.com/spf13/cobra"
)

var (
	monitorName string
)

var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Pings the given ceph monitor name",
}

func init() {
	pingCmd.Flags().StringVar(&monitorName, "monitor-name", "mon1", "name of ceph monitor to ping (required)")

	pingCmd.MarkFlagRequired("monitor-name")

	pingCmd.RunE = pingMonitor
}

func pingMonitor(cmd *cobra.Command, args []string) error {
	if err := util.VerifyRequiredFlags(cmd, []string{"monitor-name"}); err != nil {
		return err
	}

	// connect to the cluster with the client.admin creds
	adminConn, err := cephclient.ConnectToCluster(clusterName, "client.admin", configFilePath)
	if err != nil {
		return err
	}
	defer adminConn.Shutdown()

	// ping the monitor to get the full ping response
	pingResp, err := adminConn.PingMonitor(monitorName)
	if err != nil {
		return fmt.Errorf("failed to ping monitor %s, err: %+v", monitorName, err)
	}

	// response is json, umarshall into a go data structure
	var resp map[string]interface{}
	err = json.Unmarshal([]byte(pingResp), &resp)
	if err != nil {
		return fmt.Errorf("failed to unmarshal monitor ping response: %+v", err)
	}

	log.Printf("full ping response: %s", pingResp)

	// get the status blob out of the ping response, then get the "state" string field
	status := resp["mon_status"].(map[string]interface{})
	log.Printf("monitor %s status state: %s", monitorName, status["state"].(string))
	return nil
}
