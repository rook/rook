package kmip

type Authentication struct {
	Credential []Credential
}

type Nonce struct {
	NonceID    []byte
	NonceValue []byte
}

type ProtocolVersion struct {
	ProtocolVersionMajor int
	ProtocolVersionMinor int
}

type MessageExtension struct {
	VendorIdentification string
	CriticalityIndicator bool
	VendorExtension      interface{}
}

type Attributes struct {
	Attributes []Attribute
}
