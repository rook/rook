package kmip

import (
	"context"
)

// DestroyRequestPayload ////////////////////////////////////////
type DestroyRequestPayload struct {
	UniqueIdentifier string
}

// DestroyResponsePayload
type DestroyResponsePayload struct {
	UniqueIdentifier string
}

type DestroyHandler struct {
	Destroy func(ctx context.Context, payload *DestroyRequestPayload) (*DestroyResponsePayload, error)
}

func (h *DestroyHandler) HandleItem(ctx context.Context, req *Request) (*ResponseBatchItem, error) {
	var payload DestroyRequestPayload

	err := req.DecodePayload(&payload)
	if err != nil {
		return nil, err
	}

	respPayload, err := h.Destroy(ctx, &payload)
	if err != nil {
		return nil, err
	}

	// req.Key = respPayload.Key

	return &ResponseBatchItem{
		ResponsePayload: respPayload,
	}, nil
}
