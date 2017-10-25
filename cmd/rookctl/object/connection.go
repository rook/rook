/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package object

import (
	"bytes"
	"fmt"
	"os"

	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/spf13/cobra"
)

const (
	FormatPretty    = "pretty"
	FormatEnvVar    = "env-var"
	AWSHost         = "AWS_HOST"
	AWSEndpoint     = "AWS_ENDPOINT"
	AWSAccessKey    = "AWS_ACCESS_KEY_ID"
	AWSSecretKey    = "AWS_SECRET_ACCESS_KEY"
	PrettyOutputFmt = "%s\t%s\t\n"
	ExportOutputFmt = "export %s=%s\n"
)

var (
	connOutputFormat string
)

var connectionCmd = &cobra.Command{
	Use:     "connection [ObjectStore] [UserID]",
	Short:   "Gets connection information that will allow a client to access object storage",
	Aliases: []string{"conn"},
}

func init() {
	connectionCmd.Flags().StringVar(&connOutputFormat, "format", FormatPretty,
		fmt.Sprintf("Format of connection output, (valid values: %s,%s)", FormatPretty, FormatEnvVar))

	connectionCmd.RunE = getConnectionInfoEntry
}

func getConnectionInfoEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := checkObjectArgs(args, []string{"[UserID]"}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := getConnectionInfo(c, args[0], args[1], connOutputFormat)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func getConnectionInfo(c client.RookRestClient, storeName, userID, format string) (string, error) {
	if format != FormatPretty && format != FormatEnvVar {
		return "", fmt.Errorf("invalid output format: %s", format)
	}

	connInfo, err := c.GetObjectStoreConnectionInfo(storeName)
	if err != nil {
		if client.IsHttpNotFound(err) {
			return "object store connection info is not ready, if \"object create\" has already been run, please be patient\n", nil
		}

		return "", fmt.Errorf("failed to get object store connection info: %+v", err)
	}

	user, err := c.GetObjectUser(storeName, userID)
	if err != nil {
		if client.IsHttpNotFound(err) {
			return fmt.Sprintf("Unable to find user %s\n", userID), nil
		}

		return "", fmt.Errorf("failed to get object store user info: %+v", err)
	}

	var buffer bytes.Buffer

	if format == FormatPretty {
		w := rook.NewTableWriter(&buffer)

		// write header columns
		fmt.Fprintln(w, "NAME\tVALUE")

		// write object store connection info
		fmt.Fprintf(w, PrettyOutputFmt, AWSHost, connInfo.Host)
		for _, port := range connInfo.Ports {
			fmt.Fprintf(w, PrettyOutputFmt, AWSEndpoint, fmt.Sprintf("%s:%d", connInfo.IPAddress, port))
		}
		fmt.Fprintf(w, PrettyOutputFmt, AWSAccessKey, *user.AccessKey)
		fmt.Fprintf(w, PrettyOutputFmt, AWSSecretKey, *user.SecretKey)

		w.Flush()
	} else if format == FormatEnvVar {
		buffer.WriteString(fmt.Sprintf(ExportOutputFmt, AWSHost, connInfo.Host))
		for _, port := range connInfo.Ports {
			buffer.WriteString(fmt.Sprintf(ExportOutputFmt, AWSEndpoint, fmt.Sprintf("%s:%d", connInfo.IPAddress, port)))
		}
		buffer.WriteString(fmt.Sprintf(ExportOutputFmt, AWSAccessKey, *user.AccessKey))
		buffer.WriteString(fmt.Sprintf(ExportOutputFmt, AWSSecretKey, *user.SecretKey))
	}

	return buffer.String(), nil
}
