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
	"syscall"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/util/exec"
)

const (
	RGWErrorNone = iota
	RGWErrorUnknown
	RGWErrorNotFound
	RGWErrorBadData
	RGWErrorParse
	ErrorCodeFileExists = 17
)

// An ObjectUser defines the details of an object store user.
type ObjectUser struct {
	UserID      string  `json:"userId"`
	DisplayName *string `json:"displayName"`
	Email       *string `json:"email"`
	AccessKey   *string `json:"accessKey"`
	SecretKey   *string `json:"secretKey"`
	SystemUser  bool    `json:"systemuser"`
}

// ListUsers lists the object pool users.
func ListUsers(c *Context) ([]string, int, error) {
	result, err := runAdminCommand(c, true, "user", "list")
	if err != nil {
		return nil, RGWErrorUnknown, errors.Wrap(err, "failed to list users")
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
		return nil, RGWErrorParse, errors.Wrapf(err, "failed to unmarshal json. %s", data)
	}

	rookUser := ObjectUser{UserID: user.UserID, DisplayName: &user.DisplayName, Email: &user.Email}

	if len(user.Keys) > 0 {
		rookUser.AccessKey = &user.Keys[0].AccessKey
		rookUser.SecretKey = &user.Keys[0].SecretKey
	} else {
		return nil, RGWErrorBadData, errors.New("AccessKey and SecretKey are missing")
	}

	return &rookUser, RGWErrorNone, nil
}

// GetUser returns the user with the given ID.
func GetUser(c *Context, id string) (*ObjectUser, int, error) {
	logger.Debugf("getting s3 user %q", id)
	// note: err is set for non-existent user but result output is also empty
	result, err := runAdminCommand(c, false, "user", "info", "--uid", id)
	if strings.Contains(result, "no user info saved") {
		return nil, RGWErrorNotFound, errors.New("warn: s3 user not found")
	}
	if err != nil {
		return nil, RGWErrorUnknown, errors.Wrapf(err, "radosgw-admin command err. %s", result)
	}
	match, err := extractJSON(result)
	if err != nil {
		return nil, RGWErrorParse, errors.Wrap(err, "failed to get json")
	}
	return decodeUser(match)
}

// CreateUser creates a new user with the information given.
func CreateUser(c *Context, user ObjectUser) (*ObjectUser, int, error) {
	logger.Debugf("creating s3 user %q", user.UserID)

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

	if user.SystemUser {
		args = append(args, "--system")
	}

	result, err := runAdminCommand(c, true, args...)
	if err != nil {
		if strings.Contains(result, "could not create user: unable to create user, user: ") {
			return nil, ErrorCodeFileExists, errors.New("s3 user already exists")
		}

		if strings.Contains(result, "could not create user: unable to create user, email: ") && strings.Contains(result, " is the email address an existing user") {
			return nil, RGWErrorBadData, errors.New("email already in use")
		}
		// We don't know what happened
		return nil, RGWErrorUnknown, errors.Wrap(err, "failed to create s3 user")
	}
	return decodeUser(result)
}

// UpdateUser updates the user whose ID matches the user.
func UpdateUser(c *Context, user ObjectUser) (*ObjectUser, int, error) {
	logger.Infof("updating s3 user %q", user.UserID)

	args := []string{"user", "modify", "--uid", user.UserID}

	if user.DisplayName != nil {
		args = append(args, "--display-name", *user.DisplayName)
	}
	if user.Email != nil {
		args = append(args, "--email", *user.Email)
	}

	body, err := runAdminCommand(c, false, args...)
	if err != nil {
		return nil, RGWErrorUnknown, errors.Wrap(err, "failed to update s3 user")
	}

	if body == "could not modify user: unable to modify user, user not found" {
		return nil, RGWErrorNotFound, errors.New("s3 user not found")
	}
	match, err := extractJSON(body)
	if err != nil {
		return nil, RGWErrorParse, errors.Wrap(err, "failed to get json")
	}

	return decodeUser(match)
}

func ListUserBuckets(c *Context, id string, opts ...string) (string, error) {

	args := []string{"bucket", "list", "--uid", id}
	if opts != nil {
		args = append(args, opts...)
	}

	result, err := runAdminCommand(c, false, args...)

	return result, errors.Wrapf(err, "failed to list buckets for user uid=%q", id)
}

// DeleteUser deletes the user with the given ID.
func DeleteUser(c *Context, id string, opts ...string) (string, error) {
	args := []string{"user", "rm", "--uid", id}
	if opts != nil {
		args = append(args, opts...)
	}
	result, err := runAdminCommand(c, false, args...)
	if err != nil {
		// If User does not exist return success
		if code, ok := exec.ExitStatus(err); ok && code == int(syscall.ENOENT) {
			return result, nil
		}

		res, innerErr := ListUserBuckets(c, id)
		if innerErr == nil && res != "" && res != "[]" {
			return result, errors.Wrapf(err, "s3 user uid=%q have following buckets %q", id, res)
		}
	}

	return result, errors.Wrapf(err, "failed to delete s3 user uid=%q", id)
}

// SetQuotaUserBucketMax will set maximum bucket quota for a user
func SetQuotaUserBucketMax(c *Context, id string, max int) (string, error) {
	logger.Infof("Setting user %q max buckets to %d", id, max)
	args := []string{"--quota-scope", "user", "--max-buckets", strconv.Itoa(max)}
	result, err := setUserQuota(c, id, args)
	if err != nil {
		err = errors.Wrap(err, "failed setting bucket max")
	}
	return result, err
}

func setUserQuota(c *Context, id string, args []string) (string, error) {
	args = append([]string{"quota", "set", "--uid", id}, args...)
	result, err := runAdminCommand(c, false, args...)
	if err != nil {
		err = errors.Wrap(err, "failed to set max buckets for user")
	}
	return result, err
}

// LinkUser will link a user to a bucket
func LinkUser(c *Context, id, bucket string) (string, int, error) {
	logger.Infof("Linking (user: %s) (bucket: %s)", id, bucket)
	args := []string{"bucket", "link", "--uid", id, "--bucket", bucket}
	result, err := runAdminCommand(c, false, args...)
	if err != nil {
		return "", RGWErrorUnknown, err
	}
	if strings.Contains(result, "bucket entry point user mismatch") {
		return "", RGWErrorNotFound, err
	}
	return result, RGWErrorNone, nil
}

// EnableUserQuota will allows to enable quota defined for a user
func EnableUserQuota(c *Context, id string) (string, error) {
	logger.Debug("Enabling user quota for %q", id)
	args := []string{"quota", "enable", "--quota-scope", "user", "--uid", id}
	result, err := runAdminCommand(c, false, args...)
	if err != nil {
		err = errors.Wrap(err, "failed to enable quota for the user")
	}
	return result, err

}

// SetQuotaUserObject allows to set maximum limit on objects for a user
func SetQuotaUserObjectMax(c *Context, id string, maxobjects string) (string, error) {
	logger.Debugf("Setting user %q max objects to %s", id, maxobjects)
	args := []string{"--quota-scope", "user", "--max-objects", maxobjects}
	result, err := setUserQuota(c, id, args)
	if err != nil {
		err = errors.Wrap(err, "failed setting object max")
	}
	return result, err
}

// SetQuotaUserMaxSize allows to set maximum size for a user
func SetQuotaUserMaxSize(c *Context, id string, maxsize string) (string, error) {
	logger.Debugf("Setting user %q max size to %s", id, maxsize)
	args := []string{"--quota-scope", "user", "--max-size", maxsize}
	result, err := setUserQuota(c, id, args)
	if err != nil {
		err = errors.Wrap(err, "failed setting max size")
	}
	return result, err
}
