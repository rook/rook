package kmip

// CreateKeyPairRequestPayload
// 4.2 Create Key Pair
// This operation requests the server to generate a new public/private key pair
// and register the two corresponding new Managed Cryptographic Objects.
//
// The request contains attributes to be assigned to the objects (e.g.,
// Cryptographic Algorithm, Cryptographic Length, etc.). Attributes and Template
// Names MAY be specified for both keys at the same time by specifying a Common
// Template-Attribute object in the request. Attributes not common to both keys
// (e.g., Name, Cryptographic Usage Mask) MAY be specified using the Private Key
// Template-Attribute and Public Key Template-Attribute objects in the request,
// which take precedence over the Common Template-Attribute object.
//
// The Template Managed Object is deprecated as of version 1.3 of this
// specification and MAY be removed from subsequent versions of the
// specification. Individual Attributes SHOULD be used in operations which
// currently support use of a Name within a Template-Attribute to reference a
// Template.
//
// For the Private Key, the server SHALL create a Link attribute of Link Type
// Public Key pointing to the Public Key. For the Public Key, the server SHALL
// create a Link attribute of Link Type Private Key pointing to the Private Key.
// The response contains the Unique Identifiers of both created objects. The ID
// Placeholder value SHALL be set to the Unique Identifier of the Private Key.
type CreateKeyPairRequestPayload struct {
	CommonTemplateAttribute     *TemplateAttribute
	PrivateKeyTemplateAttribute *TemplateAttribute
	PublicKeyTemplateAttribute  *TemplateAttribute
}

type CreateKeyPairResponsePayload struct {
	PrivateKeyUniqueIdentifier  string
	PublicKeyUniqueIdentifier   string
	PrivateKeyTemplateAttribute *TemplateAttribute
	PublicKeyTemplateAttribute  *TemplateAttribute
}
