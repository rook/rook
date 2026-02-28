package admin

import (
	"context"
	"fmt"
	"net/http"
)

// validateSubuserAcess - Return whether the given subuser access value is valid as input parameter
func (s SubuserSpec) validateSubuserAccess() bool {
	a := s.Access
	return a == "" ||
		a == SubuserAccessRead ||
		a == SubuserAccessWrite ||
		a == SubuserAccessReadWrite ||
		a == SubuserAccessFull
}

func makeInvalidSubuserAccessLevelError(spec SubuserSpec) error {
	return fmt.Errorf("invalid subuser access level %q", spec.Access)
}

// The following are the subuser API functions.
//
// We need to explain the omission of ?subuser in the API path common
// to all three functions.
//
// According to the docs, this has to be included to select the
// subuser operation, but we already have subuser as a parameter with
// a value (and make sure it's not empty and thus included by
// validating the SubuserSpec). The presence of this parameter
// triggers the subuser operation.
//
// If we add the subuser with the empty value the API call fails as
// having an invalid signature (and it is semantically wrong as we
// then have *two* values for the subuser name, an empty one an the
// relevant one, the upstream code does not seem to handle that case
// gracefully).

// CreateSubuser - https://docs.ceph.com/en/latest/radosgw/adminops/#create-subuser
func (api *API) CreateSubuser(ctx context.Context, user User, subuser SubuserSpec) error {
	if user.ID == "" {
		return errMissingUserID
	}
	if subuser.Name == "" {
		return errMissingSubuserID
	}
	if !subuser.validateSubuserAccess() {
		return makeInvalidSubuserAccessLevelError(subuser)
	}
	v := valueToURLParams(user, []string{"uid"})
	addToURLParams(&v, subuser, []string{"subuser", "access", "access-key", "secret-key", "generate-secret", "gen-access-key", "key-type"})
	_, err := api.call(ctx, http.MethodPut, "/user", v)
	if err != nil {
		return err
	}

	return nil
}

// RemoveSubuser - https://docs.ceph.com/en/latest/radosgw/adminops/#remove-subuser
func (api *API) RemoveSubuser(ctx context.Context, user User, subuser SubuserSpec) error {
	if user.ID == "" {
		return errMissingUserID
	}
	if subuser.Name == "" {
		return errMissingSubuserID
	}

	v := valueToURLParams(user, []string{"uid"})
	addToURLParams(&v, subuser, []string{"subuser", "purge-keys"})
	_, err := api.call(ctx, http.MethodDelete, "/user", v)
	if err != nil {
		return err
	}

	return nil
}

// ModifySubuser - https://docs.ceph.com/en/latest/radosgw/adminops/#modify-subuser
func (api *API) ModifySubuser(ctx context.Context, user User, subuser SubuserSpec) error {
	if user.ID == "" {
		return errMissingUserID
	}
	if subuser.Name == "" {
		return errMissingSubuserID
	}
	if !subuser.validateSubuserAccess() {
		return makeInvalidSubuserAccessLevelError(subuser)
	}

	v := valueToURLParams(user, []string{"uid"})
	addToURLParams(&v, subuser, []string{"subuser", "access", "secret", "generate-secret", "key-type"})
	_, err := api.call(ctx, http.MethodPost, "/user", v)
	if err != nil {
		return err
	}

	return nil
}
