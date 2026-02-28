package kmip

import (
	"context"
)

// 4.26

type DiscoverVersionsRequestPayload struct {
	ProtocolVersion []ProtocolVersion
}

type DiscoverVersionsResponsePayload struct {
	ProtocolVersion []ProtocolVersion
}

type DiscoverVersionsHandler struct {
	SupportedVersions []ProtocolVersion
}

func (h *DiscoverVersionsHandler) HandleItem(_ context.Context, req *Request) (item *ResponseBatchItem, err error) {
	var payload DiscoverVersionsRequestPayload

	err = req.DecodePayload(&payload)
	if err != nil {
		return nil, err
	}

	var respPayload DiscoverVersionsResponsePayload

	if len(payload.ProtocolVersion) == 0 {
		respPayload.ProtocolVersion = h.SupportedVersions
	} else {
		for _, v := range h.SupportedVersions {
			for _, cv := range payload.ProtocolVersion {
				if cv == v {
					respPayload.ProtocolVersion = append(respPayload.ProtocolVersion, v)
					break
				}
			}
		}
	}

	return &ResponseBatchItem{
		ResponsePayload: respPayload,
	}, nil
}
