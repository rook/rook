package rook

import (
	"bytes"
	"fmt"
	"net/http"
	"os"

	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/display"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Outputs a summary of the status of the cluster",
}

func init() {
	statusCmd.RunE = getStatusEntry
}

func getStatusEntry(cmd *cobra.Command, args []string) error {

	c := client.NewRookNetworkRestClient(client.GetRestURL(apiServerEndpoint), http.DefaultClient)
	out, err := getStatus(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func getStatus(c client.RookRestClient) (string, error) {
	statusDetails, err := c.GetStatusDetails()
	if err != nil {
		return "", fmt.Errorf("failed to get status: %+v", err)
	}

	var buffer bytes.Buffer
	w := NewTableWriter(&buffer)

	// overall status
	buffer.WriteString(fmt.Sprintf("OVERALL STATUS: %s\n", model.HealthStatusToString(statusDetails.OverallStatus)))

	// summary messages
	buffer.WriteString("\n")
	buffer.WriteString("SUMMARY:\n")
	fmt.Fprintln(w, "SEVERITY\tMESSAGE")
	for _, sm := range statusDetails.SummaryMessages {
		fmt.Fprintf(w, "%s\t%s\n", model.HealthStatusToString(sm.Status), sm.Message)
	}
	w.Flush()

	// usage stats
	buffer.WriteString("\n")
	buffer.WriteString("USAGE:\n")
	fmt.Fprintln(w, "TOTAL\tUSED\tDATA\tAVAILABLE")
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
		display.BytesToString(statusDetails.Usage.TotalBytes),
		display.BytesToString(statusDetails.Usage.UsedBytes),
		display.BytesToString(statusDetails.Usage.DataBytes),
		display.BytesToString(statusDetails.Usage.AvailableBytes))
	w.Flush()

	// monitors
	buffer.WriteString("\n")
	buffer.WriteString("MONITORS:\n")
	fmt.Fprintln(w, "NAME\tADDRESS\tIN QUORUM\tSTATUS")
	for _, m := range statusDetails.Monitors {
		fmt.Fprintf(w, "%s\t%s\t%t\t%s\n", m.Name, m.Address, m.InQuorum, model.HealthStatusToString(m.Status))
	}
	w.Flush()

	// OSDs
	buffer.WriteString("\n")
	buffer.WriteString("OSDs:\n")
	fmt.Fprintln(w, "TOTAL\tUP\tIN\tFULL\tNEAR FULL")
	fmt.Fprintf(w, "%d\t%d\t%d\t%t\t%t\n", statusDetails.OSDs.Total, statusDetails.OSDs.NumberUp,
		statusDetails.OSDs.NumberIn, statusDetails.OSDs.Full, statusDetails.OSDs.NearFull)
	w.Flush()

	// placement groups
	buffer.WriteString("\n")
	buffer.WriteString(fmt.Sprintf("PLACEMENT GROUPS (%d total):\n", statusDetails.PGs.Total))
	fmt.Fprintln(w, "STATE\tCOUNT")
	for state, count := range statusDetails.PGs.StateCounts {
		fmt.Fprintf(w, "%s\t%d\n", state, count)
	}
	w.Flush()

	w.Flush()
	return buffer.String(), nil
}
