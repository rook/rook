package kmip

import (
	"context"

	"github.com/ansel1/merry"

	"github.com/gemalto/kmip-go/kmip14"
)

// TODO: should request and response payloads implement validation?
// Sort of makes sense to run validation over the request at this level, at least for spec
// compliance, though perhaps handlers may want to be more relaxed with validation.
//
// Should the response object run through validation?  What is a valid response may change as
// the spec changes.  Maybe this should just be handled by spec compliance tests.

// 4.1
//
// This operation requests the server to generate a new symmetric key as a Managed Cryptographic Object.
// This operation is not used to create a Template object (see Register operation, Section 4.3).
//
// The request contains information about the type of object being created, and some of the attributes to be
// assigned to the object (e.g., Cryptographic Algorithm, Cryptographic Length, etc.). This information MAY be
// specified by the names of Template objects that already exist.
//
// The response contains the Unique Identifier of the created object. The server SHALL copy the Unique Identifier
// returned by this operation into the ID Placeholder variable.

// CreateRequestPayload 4.1 Table 163
//
// TemplateAttribute MUST include CryptographicAlgorithm (3.4) and CryptographicUsageMask (3.19).
type CreateRequestPayload struct {
	ObjectType        kmip14.ObjectType
	TemplateAttribute TemplateAttribute
}

// CreateResponsePayload 4.1 Table 164
type CreateResponsePayload struct {
	ObjectType        kmip14.ObjectType
	UniqueIdentifier  string
	TemplateAttribute *TemplateAttribute
}

type CreateHandler struct {
	Create func(ctx context.Context, payload *CreateRequestPayload) (*CreateResponsePayload, error)
}

func (h *CreateHandler) HandleItem(ctx context.Context, req *Request) (*ResponseBatchItem, error) {
	var payload CreateRequestPayload

	err := req.DecodePayload(&payload)
	if err != nil {
		return nil, err
	}

	respPayload, err := h.Create(ctx, &payload)
	if err != nil {
		return nil, err
	}

	var ok bool

	idAttr := respPayload.TemplateAttribute.GetTag(kmip14.TagUniqueIdentifier)

	req.IDPlaceholder, ok = idAttr.AttributeValue.(string)
	if !ok {
		return nil, merry.Errorf("invalid response returned by CreateHandler: unique identifier tag in attributes should have been a string, was %t", idAttr.AttributeValue)
	}

	return &ResponseBatchItem{
		ResponsePayload: respPayload,
	}, nil
}
