package cmd

import (
	"fmt"
	"log"

	"github.com/quantum/castle/pkg/cephclient"
	"github.com/quantum/castle/pkg/cephd"
	"github.com/spf13/cobra"
)

var (
	ioPoolName string
	objectName string
)

func commonIOCmdInit(cmd *cobra.Command) {
	cmd.Flags().StringVar(&ioPoolName, "pool-name", "defaultPool", "name of storage pool to use (required)")
	cmd.Flags().StringVar(&objectName, "object-name", "defaultObj", "name of object to use (required)")

	cmd.MarkFlagRequired("pool-name")
	cmd.MarkFlagRequired("object-name")
}

func prepareIOContext() (*cephd.IOContext, error) {
	// connect to the cluster with the client.admin creds
	adminConn, err := cephclient.ConnectToCluster(clusterName, "client.admin", configFilePath)
	if err != nil {
		return nil, err
	}
	defer adminConn.Shutdown()

	log.Printf("listing pools")
	pools, err := cephclient.ListPools(adminConn)
	if err != nil {
		return nil, fmt.Errorf("failed to list pools: %+v", err)
	}
	poolExists := false
	for _, p := range pools {
		if p.Name == ioPoolName {
			poolExists = true
			break
		}
	}
	if !poolExists {
		log.Printf("making pool %s", ioPoolName)
		if err := cephclient.CreatePool(adminConn, ioPoolName); err != nil {
			return nil, fmt.Errorf("failed to make pool %s: %+v", ioPoolName, err)
		}
	}

	// open an IO context that can be used to perform IO operations
	log.Printf("opening IO context")
	ioctx, err := adminConn.OpenIOContext(ioPoolName)
	if err != nil {
		return nil, fmt.Errorf("failed to open IO context: %+v", err)
	}

	return ioctx, nil
}
