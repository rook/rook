/*
Copyright 2017 The Rook Authors. All rights reserved.

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
	"net/http"
	"os"
	"time"

	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

const (
	initObjectStoreTimeout = 60
)

var (
	displayNameFlag string
	emailFlag       string
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Performs commands and operations on object store users in the cluster",
}

func init() {
	userCmd.AddCommand(userListCmd)
	userListCmd.RunE = listUsersEntry
	userCmd.AddCommand(userGetCmd)
	userGetCmd.RunE = getUserEntry
	userCmd.AddCommand(userDeleteCmd)
	userDeleteCmd.RunE = deleteUserEntry
	userCmd.AddCommand(userCreateCmd)
	userCreateCmd.RunE = createUserEntry
	userCmd.AddCommand(userUpdateCmd)
	userUpdateCmd.RunE = updateUserEntry

	userCreateCmd.Flags().StringVar(&emailFlag, "email", "", "An email address for the user")

	userUpdateCmd.Flags().StringVar(&emailFlag, "email", "", "An email address for the user")
	userUpdateCmd.Flags().StringVar(&displayNameFlag, "display-name", "", "A display name for the user")
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "Gets a listing with details of all object users in the cluster",
}

func listUsersEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := flags.VerifyRequiredFlags(cmd, []string{}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClientWithTimeout(initObjectStoreTimeout * time.Second)
	out, err := listUsers(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func listUsers(c client.RookRestClient) (string, error) {
	users, err := c.ListObjectUsers()
	if err != nil {
		return "", fmt.Errorf("failed to get users: %+v", err)
	}

	if len(users) == 0 {
		return "", nil
	}

	var buffer bytes.Buffer
	w := rook.NewTableWriter(&buffer)

	fmt.Fprintln(w, "USER ID\tDISPLAY NAME\tEMAIL")

	for _, u := range users {
		fmt.Fprintf(w, "%s\t%s\t%s\n", u.UserID, *u.DisplayName, *u.Email)
	}

	w.Flush()
	return buffer.String(), nil
}

var userGetCmd = &cobra.Command{
	Use:   "get [User ID]",
	Short: "Gets object user info",
}

func getUserEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if len(args) == 0 {
		return fmt.Errorf("Missing required argument User ID")
	}

	if len(args) > 1 {
		return fmt.Errorf("Too many arguments")
	}

	c := rook.NewRookNetworkRestClient()
	out, err := getUser(c, args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func getUser(c client.RookRestClient, id string) (string, error) {
	user, err := c.GetObjectUser(id)

	if client.IsHttpNotFound(err) {
		return "", fmt.Errorf("Unable to find user %s", id)
	}

	if err != nil {
		return "", fmt.Errorf("failed to get user: %+v", err)
	}

	var buffer bytes.Buffer
	w := rook.NewTableWriter(&buffer)

	fmt.Fprintf(w, "User ID:\t%s\n", user.UserID)
	fmt.Fprintf(w, "Display Name:\t%s\n", *user.DisplayName)
	fmt.Fprintf(w, "Email:\t%s\n", *user.Email)
	fmt.Fprintf(w, "Access Key:\t%s\n", *user.AccessKey)
	fmt.Fprintf(w, "Secret Key:\t%s\n", *user.SecretKey)

	w.Flush()
	return buffer.String(), nil
}

var userDeleteCmd = &cobra.Command{
	Use:   "delete [User ID]",
	Short: "Deletes the object user",
}

func deleteUserEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if len(args) == 0 {
		return fmt.Errorf("Missing required argument User ID")
	}

	if len(args) > 1 {
		return fmt.Errorf("Too many arguments")
	}

	c := rook.NewRookNetworkRestClient()
	out, err := deleteUser(c, args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func deleteUser(c client.RookRestClient, id string) (string, error) {
	err := c.DeleteObjectUser(id)

	if client.IsHttpNotFound(err) {
		return "", fmt.Errorf("Unable to find user %s", id)
	}

	if !client.IsHttpStatusCode(err, http.StatusNoContent) {
		return "", fmt.Errorf("failed to delete user: %+v", err)
	}
	return "User deleted\n", nil
}

var userCreateCmd = &cobra.Command{
	Use:   "create [User ID] [Display Name]",
	Short: "Creates an object store user",
}

func createUserEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if len(args) == 0 {
		return fmt.Errorf("Missing required arguments User ID and Display Name")
	}

	if len(args) == 1 {
		return fmt.Errorf("Missing required argument Display Name")
	}

	if len(args) > 2 {
		return fmt.Errorf("Too many arguments")
	}

	user := model.ObjectUser{UserID: args[0], DisplayName: &args[1]}

	if cmd.Flags().Lookup("email").Changed {
		user.Email = &emailFlag
	}

	c := rook.NewRookNetworkRestClientWithTimeout(initObjectStoreTimeout * time.Second)
	out, err := createUser(c, user)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func createUser(c client.RookRestClient, user model.ObjectUser) (string, error) {
	createdUser, err := c.CreateObjectUser(user)

	if client.IsHttpStatusCode(err, http.StatusUnprocessableEntity) {
		restErr := err.(client.RookRestError)
		return "", fmt.Errorf(string(restErr.Body))
	}

	if err != nil {
		return "", fmt.Errorf("failed to create user: %+v", err)
	}

	return fmt.Sprintf("User Created\n\n%s", outputUser(*createdUser)), nil
}

var userUpdateCmd = &cobra.Command{
	Use:   "update [User ID]",
	Short: "Updates an object store user",
}

func updateUserEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if len(args) == 0 {
		return fmt.Errorf("Missing required argument User ID")
	}

	if len(args) > 1 {
		return fmt.Errorf("Too many arguments")
	}

	user := model.ObjectUser{UserID: args[0]}

	if cmd.Flags().Lookup("display-name").Changed {
		user.DisplayName = &displayNameFlag
	}
	if cmd.Flags().Lookup("email").Changed {
		user.Email = &emailFlag
	}

	c := rook.NewRookNetworkRestClient()
	out, err := updateUser(c, user)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func updateUser(c client.RookRestClient, user model.ObjectUser) (string, error) {
	updatedUser, err := c.UpdateObjectUser(user)

	if client.IsHttpNotFound(err) {
		return "", fmt.Errorf("Unable to find user %s", user.UserID)
	}

	if err != nil {
		return "", fmt.Errorf("failed to update user: %+v", err)
	}

	return fmt.Sprintf("User Created\n\n%s", outputUser(*updatedUser)), nil
}

func outputUser(user model.ObjectUser) string {
	var buffer bytes.Buffer
	w := rook.NewTableWriter(&buffer)

	fmt.Fprintf(w, "User ID:\t%s\n", user.UserID)
	fmt.Fprintf(w, "Display Name:\t%s\n", *user.DisplayName)
	fmt.Fprintf(w, "Email:\t%s\n", *user.Email)
	fmt.Fprintf(w, "Access Key:\t%s\n", *user.AccessKey)
	fmt.Fprintf(w, "Secret Key:\t%s\n", *user.SecretKey)

	w.Flush()
	return buffer.String()
}
