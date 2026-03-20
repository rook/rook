/*
Copyright 2026 The Rook Authors. All rights reserved.

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
	"context"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/pkg/errors"
)

// GetAccount retrieves account information from RGW using the admin ops API.
func GetAccount(ctx context.Context, adminOpsContext *AdminOpsContext, accountID string) (admin.Account, error) {
	if accountID == "" {
		return admin.Account{}, errors.New("account ID cannot be empty")
	}

	account, err := adminOpsContext.AdminOpsClient.GetAccount(ctx, accountID)
	if err != nil {
		return admin.Account{}, errors.Wrapf(err, "failed to get account %q", accountID)
	}

	return account, nil
}

// CreateAccount creates a new RGW account using the admin ops API.
// If account.ID is empty, Ceph will auto-generate the account ID.
func CreateAccount(ctx context.Context, adminOpsContext *AdminOpsContext, account admin.Account) (admin.Account, error) {
	if account.Name == "" {
		return admin.Account{}, errors.New("account name cannot be empty")
	}

	createdAccount, err := adminOpsContext.AdminOpsClient.CreateAccount(ctx, account)
	if err != nil {
		return admin.Account{}, errors.Wrapf(err, "failed to create account %q", account.Name)
	}

	return createdAccount, nil
}

// ModifyAccount modifies an existing RGW account.
func ModifyAccount(ctx context.Context, adminOpsContext *AdminOpsContext, account admin.Account) (admin.Account, error) {
	if account.ID == "" {
		return admin.Account{}, errors.New("account ID cannot be empty")
	}

	modifiedAccount, err := adminOpsContext.AdminOpsClient.ModifyAccount(ctx, account)
	if err != nil {
		return admin.Account{}, errors.Wrapf(err, "failed to modify account %q", account.ID)
	}

	return modifiedAccount, nil
}

// DeleteAccount removes an RGW account using the admin ops API.
func DeleteAccount(ctx context.Context, adminOpsContext *AdminOpsContext, accountID string) error {
	if accountID == "" {
		return errors.New("account ID cannot be empty")
	}

	err := adminOpsContext.AdminOpsClient.DeleteAccount(ctx, accountID)
	if err != nil {
		return errors.Wrapf(err, "failed to delete account %q", accountID)
	}

	return nil
}

// CreateAccountRootUser creates a root user for the given RGW account using the admin ops API.
func CreateAccountRootUser(ctx context.Context, adminOpsContext *AdminOpsContext, user admin.User) (admin.User, error) {
	if user.ID == "" {
		return admin.User{}, errors.New("user ID cannot be empty")
	}
	if user.AccountID == "" {
		return admin.User{}, errors.New("account ID cannot be empty")
	}

	createdUser, err := adminOpsContext.AdminOpsClient.CreateUser(ctx, user)
	if err != nil {
		return admin.User{}, errors.Wrapf(err, "failed to create root user %q for account %q", user.ID, user.AccountID)
	}

	return createdUser, nil
}

// GetAccountRootUser retrieves a root user from RGW using the admin ops API.
func GetAccountRootUser(ctx context.Context, adminOpsContext *AdminOpsContext, userID string) (admin.User, error) {
	if userID == "" {
		return admin.User{}, errors.New("user ID cannot be empty")
	}

	user, err := adminOpsContext.AdminOpsClient.GetUser(ctx, admin.User{ID: userID})
	if err != nil {
		return admin.User{}, errors.Wrapf(err, "failed to get root user %q", userID)
	}

	return user, nil
}

// ModifyAccountRootUser modifies an existing root user in RGW using the admin ops API.
func ModifyAccountRootUser(ctx context.Context, adminOpsContext *AdminOpsContext, user admin.User) (admin.User, error) {
	if user.ID == "" {
		return admin.User{}, errors.New("user ID cannot be empty")
	}

	modifiedUser, err := adminOpsContext.AdminOpsClient.ModifyUser(ctx, user)
	if err != nil {
		return admin.User{}, errors.Wrapf(err, "failed to modify root user %q", user.ID)
	}

	return modifiedUser, nil
}

// DeleteAccountRootUser removes a root user from RGW using the admin ops API.
func DeleteAccountRootUser(ctx context.Context, adminOpsContext *AdminOpsContext, userID string) error {
	if userID == "" {
		return errors.New("user ID cannot be empty")
	}

	err := adminOpsContext.AdminOpsClient.RemoveUser(ctx, admin.User{ID: userID})
	if err != nil {
		return errors.Wrapf(err, "failed to delete root user %q", userID)
	}

	return nil
}
