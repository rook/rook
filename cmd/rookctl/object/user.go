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
	"strings"

	"github.com/rook/rook/cmd/rookctl/rook"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/rook/client"
	"github.com/spf13/cobra"
)

var (
	displayNameFlag string
	emailFlag       string
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Performs commands and operations on object store users",
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
	Use:   "list [ObjectStore]",
	Short: "Gets a listing of all users in the object store",
}

func listUsersEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := checkObjectArgs(args, []string{}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := listUsers(c, args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func listUsers(c client.RookRestClient, storeName string) (string, error) {
	users, err := c.ListObjectUsers(storeName)
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
	Use:   "get [ObjectStore] [UserID]",
	Short: "Gets a user from the object store",
}

func getUserEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := checkObjectArgs(args, []string{"[UserID]"}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := getUser(c, args[0], args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func getUser(c client.RookRestClient, storeName, id string) (string, error) {
	user, err := c.GetObjectUser(storeName, id)

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
	Use:   "delete [ObjectStore] [UserID]",
	Short: "Deletes the user from the object store",
}

func deleteUserEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()

	if err := checkObjectArgs(args, []string{"[UserID]"}); err != nil {
		return err
	}

	c := rook.NewRookNetworkRestClient()
	out, err := deleteUser(c, args[0], args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func deleteUser(c client.RookRestClient, storeName, id string) (string, error) {
	err := c.DeleteObjectUser(storeName, id)

	if client.IsHttpNotFound(err) {
		return "", fmt.Errorf("Unable to find user %s", id)
	}

	if !client.IsHttpStatusCode(err, http.StatusNoContent) {
		return "", fmt.Errorf("failed to delete user: %+v", err)
	}
	return "User deleted\n", nil
}

var userCreateCmd = &cobra.Command{
	Use:   "create [ObjectStore] [UserID] [Display Name]",
	Short: "Creates a user in the object store",
}

func createUserEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()
	if err := checkObjectArgs(args, []string{"[UserID]", "[Display Name]"}); err != nil {
		return err
	}

	user := model.ObjectUser{UserID: args[1], DisplayName: &args[2]}

	if cmd.Flags().Lookup("email").Changed {
		user.Email = &emailFlag
	}

	c := rook.NewRookNetworkRestClient()
	out, err := createUser(c, args[0], user)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func checkObjectArgs(args, expectedArgs []string) error {
	// expect to have the object store name followed by the expectedArgs
	if len(args) == 1+len(expectedArgs) {
		return nil
	}
	if len(args) > 1+len(expectedArgs) {
		return fmt.Errorf("Too many arguments")
	}

	return fmt.Errorf("Missing at least one argument in: [ObjectStore] %s", strings.Join(expectedArgs, " "))
}

func createUser(c client.RookRestClient, storeName string, user model.ObjectUser) (string, error) {
	createdUser, err := c.CreateObjectUser(storeName, user)

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
	Use:   "update [ObjectStore] [UserID]",
	Short: "Updates a user in the object store",
}

func updateUserEntry(cmd *cobra.Command, args []string) error {
	rook.SetupLogging()
	if err := checkObjectArgs(args, []string{"[UserID]"}); err != nil {
		return err
	}

	user := model.ObjectUser{UserID: args[1]}

	if cmd.Flags().Lookup("display-name").Changed {
		user.DisplayName = &displayNameFlag
	}
	if cmd.Flags().Lookup("email").Changed {
		user.Email = &emailFlag
	}

	c := rook.NewRookNetworkRestClient()
	out, err := updateUser(c, args[0], user)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(out)
	return nil
}

func updateUser(c client.RookRestClient, storeName string, user model.ObjectUser) (string, error) {
	updatedUser, err := c.UpdateObjectUser(storeName, user)

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
