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
package rgw

import (
	"encoding/json"
	"fmt"

	"strings"

	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
)

const (
	RGWErrorNone     = iota
	RGWErrorUnknown  = iota
	RGWErrorNotFound = iota
	RGWErrorBadData  = iota
	RGWErrorParse    = iota
)

func ListUsers(context *clusterd.Context, getClusterInfo func() (*mon.ClusterInfo, error)) ([]string, int, error) {
	result, err := RunAdminCommand(context, getClusterInfo, "user", "list")
	if err != nil {
		return nil, RGWErrorUnknown, fmt.Errorf("failed to list users: %+v", err)
	}

	var s []string
	if err := json.Unmarshal([]byte(result), &s); err != nil {
		return nil, RGWErrorParse, fmt.Errorf("failed to read users info. %+v, result=%s", err, result)
	}

	return s, RGWErrorNone, nil
}

type rgwUserInfo struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Keys        []struct {
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
	}
}

func decodeUser(data string) (*model.ObjectUser, int, error) {
	var user rgwUserInfo
	err := json.Unmarshal([]byte(data), &user)
	if err != nil {
		return nil, RGWErrorParse, fmt.Errorf("Failed to unmarshal json: %+v", err)
	}

	rookUser := model.ObjectUser{UserID: user.UserID, DisplayName: &user.DisplayName, Email: &user.Email}

	if len(user.Keys) > 0 {
		rookUser.AccessKey = &user.Keys[0].AccessKey
		rookUser.SecretKey = &user.Keys[0].SecretKey
	}

	return &rookUser, RGWErrorNone, nil
}

func GetUser(context *clusterd.Context, id string, getClusterInfo func() (*mon.ClusterInfo, error)) (*model.ObjectUser, int, error) {
	logger.Infof("Getting user: %s", id)

	result, err := RunAdminCommand(context, getClusterInfo, "user", "info", "--uid", id)
	if err != nil {
		return nil, RGWErrorUnknown, fmt.Errorf("failed to get users: %+v", err)
	}
	if result == "could not fetch user info: no user info saved" {
		return nil, RGWErrorNotFound, fmt.Errorf("user not found")
	}
	return decodeUser(result)
}

func CreateUser(context *clusterd.Context, user model.ObjectUser, getClusterInfo func() (*mon.ClusterInfo, error)) (*model.ObjectUser, int, error) {
	logger.Infof("Creating user: %s", user.UserID)

	if strings.TrimSpace(user.UserID) == "" {
		return nil, RGWErrorBadData, fmt.Errorf("userId cannot be empty")
	}

	if user.DisplayName == nil {
		return nil, RGWErrorBadData, fmt.Errorf("displayName is required")
	}

	args := []string{
		"--uid", user.UserID,
		"--display-name", *user.DisplayName,
	}

	if user.Email != nil {
		args = append(args, "--email", *user.Email)
	}

	result, err := RunAdminCommand(context, getClusterInfo, "user", "create", args...)
	if err != nil {
		return nil, RGWErrorUnknown, fmt.Errorf("failed to create user: %+v", err)
	}

	if strings.HasPrefix(result, "could not create user: unable to create user, user: ") && strings.HasSuffix(result, " exists") {
		return nil, RGWErrorBadData, fmt.Errorf("user already exists")
	}

	if strings.HasPrefix(result, "could not create user: unable to create user, email: ") && strings.HasSuffix(result, " is the email address an existing user") {
		return nil, RGWErrorBadData, fmt.Errorf("email already in use")
	}

	return decodeUser(result)
}

func UpdateUser(context *clusterd.Context, user model.ObjectUser, getClusterInfo func() (*mon.ClusterInfo, error)) (*model.ObjectUser, int, error) {
	logger.Infof("Updating user: %s", user.UserID)

	args := []string{"--uid", user.UserID}

	if user.DisplayName != nil {
		args = append(args, "--display-name", *user.DisplayName)
	}
	if user.Email != nil {
		args = append(args, "--email", *user.Email)
	}

	body, err := RunAdminCommand(context, getClusterInfo, "user", "modify", args...)
	if err != nil {
		return nil, RGWErrorUnknown, fmt.Errorf("failed to update user: %+v", err)
	}

	if body == "could not modify user: unable to modify user, user not found" {
		return nil, RGWErrorNotFound, fmt.Errorf("user not found")
	}

	return decodeUser(body)
}

func DeleteUser(context *clusterd.Context, id string, getClusterInfo func() (*mon.ClusterInfo, error)) (string, int, error) {
	logger.Infof("Deleting user: %s", id)
	result, err := RunAdminCommand(context, getClusterInfo, "user", "rm", "--uid", id)
	if err != nil {
		return "", RGWErrorUnknown, fmt.Errorf("failed to delete user: %+v", err)
	}

	if result == "unable to remove user, user does not exist" {
		return "", RGWErrorNotFound, fmt.Errorf("failed to delete user: %+v", err)
	}

	return result, RGWErrorNone, nil
}
