package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// AddUserCap adds the capabilities for a user.
//
// On Success, it returns the updated list of UserCaps for the user.
func (api *API) AddUserCap(ctx context.Context, uid, userCap string) ([]UserCapSpec, error) {
	if uid == "" {
		return nil, errMissingUserID
	}
	if userCap == "" {
		return nil, errMissingUserCap
	}

	user := User{ID: uid, UserCaps: userCap}
	body, err := api.call(ctx, http.MethodPut, "/user?caps", valueToURLParams(user, []string{"uid", "user-caps"}))
	if err != nil {
		return nil, err
	}

	var ref []UserCapSpec
	err = json.Unmarshal(body, &ref)
	if err != nil {
		return nil, fmt.Errorf("%s. %s. %w", unmarshalError, string(body), err)
	}

	return ref, nil
}

// RemoveUserCap removes the capabilities from a user.
//
// On Success, it returns the updated list of UserCaps for the user.
func (api *API) RemoveUserCap(ctx context.Context, uid, userCap string) ([]UserCapSpec, error) {
	if uid == "" {
		return nil, errMissingUserID
	}
	if userCap == "" {
		return nil, errMissingUserCap
	}

	user := User{ID: uid, UserCaps: userCap}
	body, err := api.call(ctx, http.MethodDelete, "/user?caps", valueToURLParams(user, []string{"uid", "user-caps"}))
	if err != nil {
		return nil, err
	}

	var ref []UserCapSpec
	err = json.Unmarshal(body, &ref)
	if err != nil {
		return nil, fmt.Errorf("%s. %s. %w", unmarshalError, string(body), err)
	}

	return ref, nil
}
