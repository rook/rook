package main

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/quantum/castle/pkg/castlectl/client"
	"github.com/quantum/castle/pkg/util/display"
	"github.com/quantum/castle/pkg/util/flags"
	"github.com/quantum/castle/pkg/util/proc"
	"github.com/quantum/castle/pkg/util/sys"
	"github.com/spf13/cobra"
)

var blockListCmd = &cobra.Command{
	Use:   "ls",
	Short: "Gets a listing with details of all block images in the cluster and their locally mapped devices",
}

func init() {
	blockListCmd.RunE = listBlocksEntry
}

func listBlocksEntry(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	c := client.NewCastleNetworkRestClient(client.GetRestURL(apiServerEndpoint), http.DefaultClient)
	e := &proc.CommandExecutor{}
	out, err := listBlocks(rbdSysBusPathDefault, c, e)
	if err != nil {
		return err
	}

	fmt.Println(out)
	return nil
}

func listBlocks(rbdSysBusPath string, c client.CastleRestClient, executor proc.Executor) (string, error) {
	images, err := c.GetBlockImages()
	if err != nil {
		return "", fmt.Errorf("failed to get block images: %+v", err)
	}

	if len(images) == 0 {
		return "", nil
	}

	// for each image returned from the client API call, look up local details
	for i := range images {
		image := &(images[i])

		// look up local device and mount point, ignoring errors
		devPath, _ := findDevicePath(image.Name, image.PoolName, rbdSysBusPath)
		dev := strings.TrimPrefix(devPath, devicePathPrefix)
		var mountPoint string
		if dev != "" {
			mountPoint, _ = sys.GetDeviceMountPoint(dev, executor)
		}

		image.Device = dev
		image.MountPoint = mountPoint
	}

	var buffer bytes.Buffer
	w := NewTableWriter(&buffer)

	fmt.Fprintln(w, "NAME\tPOOL\tSIZE\tDEVICE\tMOUNT")

	for _, i := range images {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", i.Name, i.PoolName, display.BytesToString(i.Size), i.Device, i.MountPoint)
	}

	w.Flush()
	return buffer.String(), nil
}
