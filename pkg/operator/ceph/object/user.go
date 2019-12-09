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
	"encoding/json"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

const (
	RGWErrorNone = iota
	RGWErrorUnknown
	RGWErrorNotFound
	RGWErrorBadData
	RGWErrorParse
)

// An ObjectUser defines the details of an object store user.
type ObjectUser struct {
	UserID      string  `json:"userId"`
	DisplayName *string `json:"displayName"`
	Email       *string `json:"email"`
	AccessKey   *string `json:"accessKey"`
	SecretKey   *string `json:"secretKey"`
}

// ListUsers lists the object pool users.
func ListUsers(c *Context) ([]string, int, error) {
	result, err := runAdminCommand(c, "user", "list")
	if err != nil {
		return nil, RGWErrorUnknown, errors.Wrapf(err, "failed to list users")
	}

	var s []string
	if err := json.Unmarshal([]byte(result), &s); err != nil {
		return nil, RGWErrorParse, errors.Wrapf(err, "failed to read users info result=%s", result)
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

func decodeUser(data string) (*ObjectUser, int, error) {
	var user rgwUserInfo
	err := json.Unmarshal([]byte(data), &user)
	if err != nil {
		return nil, RGWErrorParse, errors.Wrapf(err, "Failed to unmarshal json")
	}

	rookUser := ObjectUser{UserID: user.UserID, DisplayName: &user.DisplayName, Email: &user.Email}

	if len(user.Keys) > 0 {
		rookUser.AccessKey = &user.Keys[0].AccessKey
		rookUser.SecretKey = &user.Keys[0].SecretKey
	}

	return &rookUser, RGWErrorNone, nil
}

// GetUser returns the user with the given ID.
func GetUser(c *Context, id string) (*ObjectUser, int, error) {
	logger.Infof("Getting user: %s", id)

	// note: err is set for non-existent user but result output is also empty
	result, err := runAdminCommand(c, "user", "info", "--uid", id)
	if len(result) == 0 {
		return nil, RGWErrorNotFound, errors.New("warn: user not found")
	}
	if err != nil {
		return nil, RGWErrorUnknown, errors.Wrapf(err, "radosgw-admin command err")
	}
	return decodeUser(result)
}

// CreateUser creates a new user with the information given.
func CreateUser(c *Context, user ObjectUser) (*ObjectUser, int, error) {
	logger.Infof("Creating user: %s", user.UserID)

	if strings.TrimSpace(user.UserID) == "" {
		return nil, RGWErrorBadData, errors.New("userId cannot be empty")
	}

	if user.DisplayName == nil {
		return nil, RGWErrorBadData, errors.New("displayName is required")
	}

	args := []string{
		"user",
		"create",
		"--uid", user.UserID,
		"--display-name", *user.DisplayName,
	}

	if user.Email != nil {
		args = append(args, "--email", *user.Email)
	}

	result, err := runAdminCommand(c, args...)
	if err != nil {
		return nil, RGWErrorUnknown, errors.Wrapf(err, "failed to create user")
	}

	if strings.HasPrefix(result, "could not create user: unable to create user, user: ") && strings.HasSuffix(result, " exists") {
		return nil, RGWErrorBadData, errors.New("user already exists")
	}

	if strings.HasPrefix(result, "could not create user: unable to create user, email: ") && strings.HasSuffix(result, " is the email address an existing user") {
		return nil, RGWErrorBadData, errors.New("email already in use")
	}

	return decodeUser(result)
}

// UpdateUser updates the user whose ID matches the user.
func UpdateUser(c *Context, user ObjectUser) (*ObjectUser, int, error) {
	logger.Infof("Updating user: %s", user.UserID)

	args := []string{"user", "modify", "--uid", user.UserID}

	if user.DisplayName != nil {
		args = append(args, "--display-name", *user.DisplayName)
	}
	if user.Email != nil {
		args = append(args, "--email", *user.Email)
	}

	body, err := runAdminCommand(c, args...)
	if err != nil {
		return nil, RGWErrorUnknown, errors.Wrapf(err, "failed to update user")
	}

	if body == "could not modify user: unable to modify user, user not found" {
		return nil, RGWErrorNotFound, errors.New("user not found")
	}

	return decodeUser(body)
}

// DeleteUser deletes the user with the given ID.
func DeleteUser(c *Context, id string, opts ...string) (string, int, error) {
	logger.Infof("Deleting user: %s", id)
	args := []string{"user", "rm", "--uid", id}
	if opts != nil {
		args = append(args, opts...)
	}
	result, err := runAdminCommand(c, args...)
	if err != nil {
		return "", RGWErrorUnknown, errors.Wrapf(err, "failed to delete user")
	}
	if result == "unable to remove user, user does not exist" {
		return "", RGWErrorNotFound, errors.Wrapf(err, "user %q does not exist so cannot delete", id)
	}

	return result, RGWErrorNone, nil
}

func SetQuotaUserBucketMax(c *Context, id string, max int) (string, int, error) {
	logger.Infof("Setting user %q max buckets to %d", id, max)
	args := []string{"--quota-scope", "user", "--max-buckets", strconv.Itoa(max)}
	result, errCode, err := setUserQuota(c, id, args)
	if errCode != RGWErrorNone {
		err = errors.Wrapf(err, "failed setting bucket max")
	}
	return result, errCode, err
}

func setUserQuota(c *Context, id string, args []string) (string, int, error) {
	args = append([]string{"quota", "set", "--uid", id}, args...)
	result, err := runAdminCommand(c, args...)
	if err != nil {
		err = errors.Wrapf(err, "failed to set max buckets for user")
	}
	return result, RGWErrorNone, err
}

func LinkUser(c *Context, id, bucket string) (string, int, error) {
	logger.Infof("Linking (user: %s) (bucket: %s)", id, bucket)
	args := []string{"bucket", "link", "--uid", id, "--bucket", bucket}
	result, err := runAdminCommand(c, args...)
	if err != nil {
		return "", RGWErrorUnknown, err
	}
	if strings.Contains(result, "bucket entry point user mismatch") {
		return "", RGWErrorNotFound, err
	}
	return result, RGWErrorNone, nil
}

func UnlinkUser(c *Context, id, bucket string) (string, int, error) {
	logger.Info("Unlinking (user: %s) (bucket: %s)", id, bucket)
	args := []string{"bucket", "unlink", "--uid", id, "--bucket", bucket}
	result, err := runAdminCommand(c, args...)
	if err != nil {
		return "", RGWErrorUnknown, err
	}
	if strings.Contains(result, "bucket entry point user mismatch") {
		return "", RGWErrorNotFound, err
	}
	return result, RGWErrorNone, nil
}
