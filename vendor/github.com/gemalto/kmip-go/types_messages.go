package kmip

import (
	"time"

	"github.com/gemalto/kmip-go/kmip14"
)

// 7.1

type RequestMessage struct {
	RequestHeader RequestHeader
	BatchItem     []RequestBatchItem
}

type ResponseMessage struct {
	ResponseHeader ResponseHeader
	BatchItem      []ResponseBatchItem
}

// 7.2

type RequestHeader struct {
	ProtocolVersion              ProtocolVersion
	MaximumResponseSize          int    `ttlv:",omitempty"`
	ClientCorrelationValue       string `ttlv:",omitempty"`
	ServerCorrelationValue       string `ttlv:",omitempty"`
	AsynchronousIndicator        bool   `ttlv:",omitempty"`
	AttestationCapableIndicator  bool   `ttlv:",omitempty"`
	AttestationType              []kmip14.AttestationType
	Authentication               *Authentication
	BatchErrorContinuationOption kmip14.BatchErrorContinuationOption `ttlv:",omitempty"`
	BatchOrderOption             bool                                `ttlv:",omitempty"`
	TimeStamp                    *time.Time
	BatchCount                   int
}

type RequestBatchItem struct {
	Operation         kmip14.Operation
	UniqueBatchItemID []byte `ttlv:",omitempty"`
	RequestPayload    interface{}
	MessageExtension  *MessageExtension `ttlv:",omitempty"`
}

type ResponseHeader struct {
	ProtocolVersion        ProtocolVersion
	TimeStamp              time.Time
	Nonce                  *Nonce
	AttestationType        []kmip14.AttestationType
	ClientCorrelationValue string `ttlv:",omitempty"`
	ServerCorrelationValue string `ttlv:",omitempty"`
	BatchCount             int
}

type ResponseBatchItem struct {
	Operation                    kmip14.Operation `ttlv:",omitempty"`
	UniqueBatchItemID            []byte           `ttlv:",omitempty"`
	ResultStatus                 kmip14.ResultStatus
	ResultReason                 kmip14.ResultReason `ttlv:",omitempty"`
	ResultMessage                string              `ttlv:",omitempty"`
	AsynchronousCorrelationValue []byte              `ttlv:",omitempty"`
	ResponsePayload              interface{}         `ttlv:",omitempty"`
	MessageExtension             *MessageExtension
}
