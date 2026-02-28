package admin

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	// ErrUserExists - Attempt to create existing user
	ErrUserExists errorReason = "UserAlreadyExists"

	// ErrNoSuchUser - User does not exist
	ErrNoSuchUser errorReason = "NoSuchUser"

	// ErrInvalidAccessKey - Invalid access key specified
	ErrInvalidAccessKey errorReason = "InvalidAccessKey"

	// ErrInvalidSecretKey - Invalid secret key specified
	ErrInvalidSecretKey errorReason = "InvalidSecretKey"

	// ErrInvalidKeyType - Invalid key type specified
	ErrInvalidKeyType errorReason = "InvalidKeyType"

	// ErrKeyExists - Provided access key exists and belongs to another user
	ErrKeyExists errorReason = "KeyExists"

	// ErrEmailExists - Provided email address exists
	ErrEmailExists errorReason = "EmailExists"

	// ErrInvalidCapability - Attempt to remove an invalid admin capability
	ErrInvalidCapability errorReason = "InvalidCapability"

	// ErrSubuserExists - Specified subuser exists
	ErrSubuserExists errorReason = "SubuserExists"

	// ErrNoSuchSubUser - SubUser does not exist
	ErrNoSuchSubUser errorReason = "NoSuchSubUser"

	// ErrInvalidAccess - Invalid subuser access specified
	ErrInvalidAccess errorReason = "InvalidAccess"

	// ErrIndexRepairFailed - Bucket index repair failed
	ErrIndexRepairFailed errorReason = "IndexRepairFailed"

	// ErrBucketNotEmpty - Attempted to delete non-empty bucket
	ErrBucketNotEmpty errorReason = "BucketNotEmpty"

	// ErrObjectRemovalFailed - Unable to remove objects
	ErrObjectRemovalFailed errorReason = "ObjectRemovalFailed"

	// ErrBucketUnlinkFailed - Unable to unlink bucket from specified user
	ErrBucketUnlinkFailed errorReason = "BucketUnlinkFailed"

	// ErrBucketLinkFailed - Unable to link bucket to specified user
	ErrBucketLinkFailed errorReason = "BucketLinkFailed"

	// ErrNoSuchObject - Specified object does not exist
	ErrNoSuchObject errorReason = "NoSuchObject"

	// ErrIncompleteBody - Either bucket was not specified for a bucket policy request or bucket and object were not specified for an object policy request.
	ErrIncompleteBody errorReason = "IncompleteBody"

	// ErrNoSuchCap - User does not possess specified capability
	ErrNoSuchCap errorReason = "NoSuchCap"

	// ErrInternalError - Internal server error.
	ErrInternalError errorReason = "InternalError"

	// ErrAccessDenied - Access denied.
	ErrAccessDenied errorReason = "AccessDenied"

	// ErrNoSuchBucket - Bucket does not exist.
	ErrNoSuchBucket errorReason = "NoSuchBucket"

	// ErrNoSuchKey - No such access key.
	ErrNoSuchKey errorReason = "NoSuchKey"

	// ErrInvalidArgument - Invalid argument.
	ErrInvalidArgument errorReason = "InvalidArgument"

	// ErrUnknown - reports an unknown error
	ErrUnknown errorReason = "Unknown"

	// ErrSignatureDoesNotMatch - the query to the API has invalid parameters
	ErrSignatureDoesNotMatch errorReason = "SignatureDoesNotMatch"

	unmarshalError = "failed to unmarshal radosgw http response"
)

var (
	errMissingUserID          = errors.New("missing user ID")
	errMissingSubuserID       = errors.New("missing subuser ID")
	errMissingUserAccessKey   = errors.New("missing user access key")
	errMissingUserDisplayName = errors.New("missing user display name")
	errMissingUserCap         = errors.New("missing user capabilities")
	errMissingBucketID        = errors.New("missing bucket ID")
	errMissingBucket          = errors.New("missing bucket")
	errMissingUserBucket      = errors.New("missing bucket")
	errUnsupportedKeyType     = errors.New("unsupported key type")
)

// errorReason is the reason of the error
type errorReason string

// statusError is the API response when an error occurs
type statusError struct {
	Code      string `json:"Code,omitempty"`
	RequestID string `json:"RequestId,omitempty"`
	HostID    string `json:"HostId,omitempty"`
}

func handleStatusError(decodedResponse []byte) error {
	statusError := statusError{}
	err := json.Unmarshal(decodedResponse, &statusError)
	if err != nil {
		return fmt.Errorf("%s. %s. %w", unmarshalError, string(decodedResponse), err)
	}

	return statusError
}

func (e errorReason) Error() string { return string(e) }

// Is determines whether the error is known to be reported
func (e statusError) Is(target error) bool { return target == errorReason(e.Code) }

// Error returns non-empty string if there was an error.
func (e statusError) Error() string { return fmt.Sprintf("%s %s %s", e.Code, e.RequestID, e.HostID) }
