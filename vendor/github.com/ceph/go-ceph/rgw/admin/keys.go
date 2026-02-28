package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// CreateKey will generate new keys or add specified to keyring
// https://docs.ceph.com/en/latest/radosgw/adminops/#create-key
func (api *API) CreateKey(ctx context.Context, key UserKeySpec) (*[]UserKeySpec, error) {
	switch key.KeyType {
	case "swift":
		if key.SubUser == "" {
			return nil, errMissingSubuserID
		}
	case "s3", "": /* s3 key-type is regarded as default */
		if key.UID == "" {
			return nil, errMissingUserID
		}
	default:
		return nil, errUnsupportedKeyType
	}

	body, err := api.call(ctx, http.MethodPut, "/user?key", valueToURLParams(key, []string{"uid", "subuser", "access-key", "secret-key", "key-type", "generate-key"}))
	if err != nil {
		return nil, err
	}

	ref := []UserKeySpec{}
	err = json.Unmarshal(body, &ref)
	if err != nil {
		return nil, fmt.Errorf("%s. %s. %w", unmarshalError, string(body), err)
	}

	return &ref, nil
}

// RemoveKey will remove an existing key
// https://docs.ceph.com/en/latest/radosgw/adminops/#remove-key
// KeySpec.SecretKey parameter shouldn't be provided and will be ignored
func (api *API) RemoveKey(ctx context.Context, key UserKeySpec) error {
	switch key.KeyType {
	case "swift":
		if key.SubUser == "" {
			return errMissingSubuserID
		}
	case "s3", "": /* s3 key-type is regarded as default */
		if key.UID == "" {
			return errMissingUserID
		}

		if key.AccessKey == "" {
			return errMissingUserAccessKey
		}
	default:
		return errUnsupportedKeyType
	}

	_, err := api.call(ctx, http.MethodDelete, "/user?key", valueToURLParams(key, []string{"uid", "subuser", "access-key", "key-type"}))
	if err != nil {
		return err
	}

	return nil
}
