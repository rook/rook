package kmip

import (
	"math/big"

	"github.com/gemalto/kmip-go/kmip14"
	"github.com/gemalto/kmip-go/ttlv"
)

// 2.1 Base Objects
//
// These objects are used within the messages of the protocol, but are not objects managed by the key
// management system. They are components of Managed Objects.

// Attribute 2.1.1 Table 2
//
// An Attribute object is a structure (see Table 2) used for sending and receiving Managed Object attributes.
// The Attribute Name is a text-string that is used to identify the attribute. The Attribute Index is an index
// number assigned by the key management server. The Attribute Index is used to identify the particular instance.
// Attribute Indices SHALL start with 0. The Attribute Index of an attribute SHALL NOT change when other instances
// are added or deleted. Single-instance Attributes (attributes which an object MAY only have at most one instance
// thereof) SHALL have an Attribute Index of 0. The Attribute Value is either a primitive data type or structured
// object, depending on the attribute.
//
// When an Attribute structure is used to specify or return a particular instance of an Attribute and the Attribute
// Index is not specified it SHALL be assumed to be 0.
type Attribute struct {
	// AttributeName should contain the canonical name of a tag, e.g. "Cryptographic Algorithm"
	AttributeName string
	// AttributeIndex is typically 0 when clients use this struct to create objects or add attributes.  Clients
	// only need to set this if modifying or deleting an existing attribute.
	AttributeIndex int `ttlv:",omitempty"`
	AttributeValue interface{}
}

func NewAttributeFromTag(tag ttlv.Tag, idx int, val interface{}) Attribute {
	return Attribute{
		AttributeName:  tag.CanonicalName(),
		AttributeIndex: idx,
		AttributeValue: val,
	}
}

// Credential 2.1.2 Table 3
//
// A Credential is a structure (see Table 3) used for client identification purposes and is not managed by the
// key management system (e.g., user id/password pairs, Kerberos tokens, etc.). It MAY be used for authentication
// purposes as indicated in [KMIP-Prof].
//
// TODO: add an unmarshal impl to Credential to handle decoding the right kind
// of credential based on the credential type value
type Credential struct {
	CredentialType  kmip14.CredentialType
	CredentialValue interface{}
}

// UsernameAndPasswordCredentialValue 2.1.2 Table 4
//
// If the Credential Type in the Credential is Username and Password, then Credential Value is a
// structure as shown in Table 4. The Username field identifies the client, and the Password field
// is a secret that authenticates the client.
type UsernameAndPasswordCredentialValue struct {
	Username string
	Password string `ttlv:",omitempty"`
}

// DeviceCredentialValue 2.1.2 Table 5
//
// If the Credential Type in the Credential is Device, then Credential Value is a structure as shown in
// Table 5. One or a combination of the Device Serial Number, Network Identifier, Machine Identifier,
// and Media Identifier SHALL be unique. Server implementations MAY enforce policies on uniqueness for
// individual fields.  A shared secret or password MAY also be used to authenticate the client.
// The client SHALL provide at least one field.
type DeviceCredentialValue struct {
	DeviceSerialNumber string `ttlv:",omitempty"`
	Password           string `ttlv:",omitempty"`
	DeviceIdentifier   string `ttlv:",omitempty"`
	NetworkIdentifier  string `ttlv:",omitempty"`
	MachineIdentifier  string `ttlv:",omitempty"`
	MediaIdentifier    string `ttlv:",omitempty"`
}

// AttestationCredentialValue 2.1.2 Table 6
//
// If the Credential Type in the Credential is Attestation, then Credential Value is a structure
// as shown in Table 6. The Nonce Value is obtained from the key management server in a Nonce Object.
// The Attestation Credential Object can contain a measurement from the client or an assertion from a
// third party if the server is not capable or willing to verify the attestation data from the client.
// Neither type of attestation data (Attestation Measurement or Attestation Assertion) is necessary to
// allow the server to accept either. However, the client SHALL provide attestation data in either the
// Attestation Measurement or Attestation Assertion fields.
type AttestationCredentialValue struct {
	Nonce                  Nonce
	AttestationType        kmip14.AttestationType
	AttestationMeasurement []byte `ttlv:",omitempty"`
	AttestationAssertion   []byte `ttlv:",omitempty"`
}

// KeyBlock 2.1.3 Table 7
//
// A Key Block object is a structure (see Table 7) used to encapsulate all of the information that is
// closely associated with a cryptographic key. It contains a Key Value of one of the following Key Format Types:
//
//   - Raw – This is a key that contains only cryptographic key material, encoded as a string of bytes.
//   - Opaque – This is an encoded key for which the encoding is unknown to the key management system.
//     It is encoded as a string of bytes.
//   - PKCS1 – This is an encoded private key, expressed as a DER-encoded ASN.1 PKCS#1 object.
//   - PKCS8 – This is an encoded private key, expressed as a DER-encoded ASN.1 PKCS#8 object, supporting both
//     the RSAPrivateKey syntax and EncryptedPrivateKey.
//   - X.509 – This is an encoded object, expressed as a DER-encoded ASN.1 X.509 object.
//   - ECPrivateKey – This is an ASN.1 encoded elliptic curve private key.
//   - Several Transparent Key types – These are algorithm-specific structures containing defined values
//     for the various key types, as defined in Section 2.1.7.
//   - Extensions – These are vendor-specific extensions to allow for proprietary or legacy key formats.
//
// The Key Block MAY contain the Key Compression Type, which indicates the format of the elliptic curve public
// key. By default, the public key is uncompressed.
//
// The Key Block also has the Cryptographic Algorithm and the Cryptographic Length of the key contained
// in the Key Value field. Some example values are:
//
//   - RSA keys are typically 1024, 2048 or 3072 bits in length.
//   - 3DES keys are typically from 112 to 192 bits (depending upon key length and the presence of parity bits).
//   - AES keys are 128, 192 or 256 bits in length.
//
// The Key Block SHALL contain a Key Wrapping Data structure if the key in the Key Value field is
// wrapped (i.e., encrypted, or MACed/signed, or both).
type KeyBlock struct {
	KeyFormatType          kmip14.KeyFormatType
	KeyCompressionType     kmip14.KeyCompressionType     `ttlv:",omitempty"`
	KeyValue               *KeyValue                     `ttlv:",omitempty"`
	CryptographicAlgorithm kmip14.CryptographicAlgorithm `ttlv:",omitempty"`
	CryptographicLength    int                           `ttlv:",omitempty"`
	KeyWrappingData        *KeyWrappingData
}

// KeyValue 2.1.4 Table 8
//
// The Key Value is used only inside a Key Block and is either a Byte String or a structure (see Table 8):
//
//   - The Key Value structure contains the key material, either as a byte string or as a Transparent Key
//     structure (see Section 2.1.7), and OPTIONAL attribute information that is associated and encapsulated
//     with the key material. This attribute information differs from the attributes associated with Managed
//     Objects, and is obtained via the Get Attributes operation, only by the fact that it is encapsulated with
//     (and possibly wrapped with) the key material itself.
//   - The Key Value Byte String is either the wrapped TTLV-encoded (see Section 9.1) Key Value structure, or
//     the wrapped un-encoded value of the Byte String Key Material field.
//
// TODO: Unmarshaler impl which unmarshals correct KeyMaterial type.
type KeyValue struct {
	// KeyMaterial should be []byte, one of the Transparent*Key structs, or a custom struct if KeyFormatType is
	// an extension.
	KeyMaterial interface{}
	Attribute   []Attribute
}

// KeyWrappingData 2.1.5 Table 9
//
// The Key Block MAY also supply OPTIONAL information about a cryptographic key wrapping mechanism used
// to wrap the Key Value. This consists of a Key Wrapping Data structure (see Table 9). It is only used
// inside a Key Block.
//
// This structure contains fields for:
//
//   - A Wrapping Method, which indicates the method used to wrap the Key Value.
//   - Encryption Key Information, which contains the Unique Identifier (see 3.1) value of the encryption key
//     and associated cryptographic parameters.
//   - MAC/Signature Key Information, which contains the Unique Identifier value of the MAC/signature key
//     and associated cryptographic parameters.
//   - A MAC/Signature, which contains a MAC or signature of the Key Value.
//   - An IV/Counter/Nonce, if REQUIRED by the wrapping method.
//   - An Encoding Option, specifying the encoding of the Key Material within the Key Value structure of the
//     Key Block that has been wrapped. If No Encoding is specified, then the Key Value structure SHALL NOT contain
//     any attributes.
//
// If wrapping is used, then the whole Key Value structure is wrapped unless otherwise specified by the
// Wrapping Method. The algorithms used for wrapping are given by the Cryptographic Algorithm attributes of
// the encryption key and/or MAC/signature key; the block-cipher mode, padding method, and hashing algorithm used
// for wrapping are given by the Cryptographic Parameters in the Encryption Key Information and/or MAC/Signature
// Key Information, or, if not present, from the Cryptographic Parameters attribute of the respective key(s).
// Either the Encryption Key Information or the MAC/Signature Key Information (or both) in the Key Wrapping Data
// structure SHALL be specified.
//
// The following wrapping methods are currently defined:
//
//   - Encrypt only (i.e., encryption using a symmetric key or public key, or authenticated encryption algorithms that use a single key).
//   - MAC/sign only (i.e., either MACing the Key Value with a symmetric key, or signing the Key Value with a private key).
//   - Encrypt then MAC/sign.
//   - MAC/sign then encrypt.
//   - TR-31.
//   - Extensions.
//
// The following encoding options are currently defined:
//
//   - No Encoding (i.e., the wrapped un-encoded value of the Byte String Key Material field in the Key Value structure).
//   - TTLV Encoding (i.e., the wrapped TTLV-encoded Key Value structure).
type KeyWrappingData struct {
	WrappingMethod             kmip14.WrappingMethod
	EncryptionKeyInformation   *EncryptionKeyInformation
	MACSignatureKeyInformation *MACSignatureKeyInformation
	MACSignature               []byte
	IVCounterNonce             []byte
	EncodingOption             kmip14.EncodingOption `ttlv:",omitempty" default:"TTLVEncoding"`
}

// EncryptionKeyInformation 2.1.5 Table 10
type EncryptionKeyInformation struct {
	UniqueIdentifier        string
	CryptographicParameters *CryptographicParameters
}

// MACSignatureKeyInformation 2.1.5 Table 11
type MACSignatureKeyInformation struct {
	UniqueIdentifier        string
	CryptographicParameters *CryptographicParameters
}

// TransparentSymmetricKey 2.1.7.1 Table 14
//
// If the Key Format Type in the Key Block is Transparent Symmetric Key, then Key Material is a
// structure as shown in Table 14.
type TransparentSymmetricKey struct {
	Key []byte `validate:"required"`
}

// TransparentDSAPrivateKey 2.1.7.2 Table 15
//
// If the Key Format Type in the Key Block is Transparent DSA Private Key, then Key Material is a structure as
// shown in Table 15.
type TransparentDSAPrivateKey struct {
	// TODO: should these be pointers?  big package deals entirely with pointers, but these are not optional values.
	P *big.Int `validate:"required"`
	Q *big.Int `validate:"required"`
	G *big.Int `validate:"required"`
	X *big.Int `validate:"required"`
}

// TransparentDSAPublicKey 2.1.7.3 Table 16
//
// If the Key Format Type in the Key Block is Transparent DSA Public Key, then Key Material is a structure as
// shown in Table 16.
type TransparentDSAPublicKey struct {
	P *big.Int `validate:"required"`
	Q *big.Int `validate:"required"`
	G *big.Int `validate:"required"`
	Y *big.Int `validate:"required"`
}

// TransparentRSAPrivateKey 2.1.7.4 Table 17
//
// If the Key Format Type in the Key Block is Transparent RSA Private Key, then Key Material is a structure
// as shown in Table 17.
//
// One of the following SHALL be present (refer to [PKCS#1]):
//
//   - Private Exponent,
//   - P and Q (the first two prime factors of Modulus), or
//   - Prime Exponent P and Prime Exponent Q.
type TransparentRSAPrivateKey struct {
	Modulus                         *big.Int `validate:"required"`
	PrivateExponent, PublicExponent *big.Int
	P, Q                            *big.Int
	PrimeExponentP, PrimeExponentQ  *big.Int
	CRTCoefficient                  *big.Int
}

// TransparentRSAPublicKey 2.1.7.5 Table 18
//
// If the Key Format Type in the Key Block is Transparent RSA Public Key, then Key Material is a structure
// as shown in Table 18.
type TransparentRSAPublicKey struct {
	Modulus        *big.Int `validate:"required"`
	PublicExponent *big.Int `validate:"required"`
}

// TransparentDHPrivateKey 2.1.7.6 Table 19
//
// If the Key Format Type in the Key Block is Transparent DH Private Key, then Key Material is a structure as shown
// in Table 19.
type TransparentDHPrivateKey struct {
	P *big.Int `validate:"required"`
	Q *big.Int
	G *big.Int `validate:"required"`
	J *big.Int
	X *big.Int `validate:"required"`
}

// TransparentDHPublicKey 2.1.7.7 Table 20
//
// If the Key Format Type in the Key Block is Transparent DH Public Key, then Key Material is a structure as
// shown in Table 20.
//
// P, G, and Y are required.
type TransparentDHPublicKey struct {
	P *big.Int `validate:"required"`
	Q *big.Int
	G *big.Int `validate:"required"`
	J *big.Int
	Y *big.Int `validate:"required"`
}

// TransparentECDSAPrivateKey 2.1.7.8 Table 21
//
// The Transparent ECDSA Private Key structure is deprecated as of version 1.3 of this
// specification and MAY be removed from subsequent versions of the specification. The
// Transparent EC Private Key structure SHOULD be used as a replacement.
//
// If the Key Format Type in the Key Block is Transparent ECDSA Private Key, then Key Material is a
// structure as shown in Table 21.
type TransparentECDSAPrivateKey struct {
	RecommendedCurve kmip14.RecommendedCurve
	D                *big.Int `validate:"required"`
}

// TransparentECDSAPublicKey 2.1.7.9 Table 22
//
// The Transparent ECDSA Public Key structure is deprecated as of version 1.3 of this specification and
// MAY be removed from subsequent versions of the specification. The Transparent EC Public Key structure
// SHOULD be used as a replacement.
//
// If the Key Format Type in the Key Block is Transparent ECDSA Public Key, then Key Material is a
// structure as shown in Table 22.
type TransparentECDSAPublicKey struct {
	RecommendedCurve kmip14.RecommendedCurve
	QString          []byte `validate:"required"`
}

// TransparentECDHPrivateKey 2.1.7.10 Table 23
//
// The Transparent ECDH Private Key structure is deprecated as of version 1.3 of this specification and
// MAY be removed from subsequent versions of the specification. The Transparent EC Private Key structure
// SHOULD be used as a replacement.
//
// If the Key Format Type in the Key Block is Transparent ECDH Private Key, then Key Material is a structure
// as shown in Table 23.
type TransparentECDHPrivateKey TransparentECPrivateKey

// TransparentECDHPublicKey 2.1.7.11 Table 24
//
// The Transparent ECDH Public Key structure is deprecated as of version 1.3 of this specification and MAY
// be removed from subsequent versions of the specification. The Transparent EC Public Key structure SHOULD
// be used as a replacement.
//
// If the Key Format Type in the Key Block is Transparent ECDH Public Key, then Key Material is a structure as
// shown in Table 24.
type TransparentECDHPublicKey TransparentECPublicKey

// TransparentECMQVPrivateKey 2.1.7.12 Table 25
//
// The Transparent ECMQV Private Key structure is deprecated as of version 1.3 of this specification and MAY
// be removed from subsequent versions of the specification. The Transparent EC Private Key structure SHOULD
// be used as a replacement.
//
// If the Key Format Type in the Key Block is Transparent ECMQV Private Key, then Key Material is a structure
// as shown in Table 25.
type TransparentECMQVPrivateKey TransparentECPrivateKey

// TransparentECMQVPublicKey 2.1.7.13 Table 26
//
// The Transparent ECMQV Public Key structure is deprecated as of version 1.3 of this specification and MAY be
// removed from subsequent versions of the specification. The Transparent EC Public Key structure SHOULD be used as
// a replacement.
//
// If the Key Format Type in the Key Block is Transparent ECMQV Public Key, then Key Material is a structure as shown
// in Table 26.
type TransparentECMQVPublicKey TransparentECPublicKey

// TransparentECPrivateKey 2.1.7.14 Table 27
//
// If the Key Format Type in the Key Block is Transparent EC Private Key, then Key Material is a structure as shown
// in Table 27.
type TransparentECPrivateKey struct {
	RecommendedCurve kmip14.RecommendedCurve
	D                *big.Int `validate:"required"`
}

// TransparentECPublicKey 2.1.7.15 Table 28
//
// If the Key Format Type in the Key Block is Transparent EC Public Key, then Key Material is a structure as
// shown in Table 28.
type TransparentECPublicKey struct {
	RecommendedCurve kmip14.RecommendedCurve
	QString          []byte `validate:"required"`
}

// TemplateAttribute 2.1.8 Table 29
//
// The Template Managed Object is deprecated as of version 1.3 of this specification and MAY be removed from
// subsequent versions of the specification. Individual Attributes SHOULD be used in operations which currently
// support use of a Name within a Template-Attribute to reference a Template.
//
// These structures are used in various operations to provide the desired attribute values and/or template
// names in the request and to return the actual attribute values in the response.
//
// The Template-Attribute, Common Template-Attribute, Private Key Template-Attribute, and Public Key
// Template-Attribute structures are defined identically as follows:
//
//	type TemplateAttribute struct {
//		Attribute []Attribute
//	}
type TemplateAttribute struct {
	Name      []Name
	Attribute []Attribute
}

// Get returns a reference to the first Attribute in the list matching the name.
// Returns nil if not found.
func (t *TemplateAttribute) Get(s string) *Attribute {
	if t == nil {
		return nil
	}

	for i := range t.Attribute {
		if t.Attribute[i].AttributeName == s {
			return &t.Attribute[i]
		}
	}

	return nil
}

// GetIdx returns a reference to the Attribute in the list matching the name and index.
// Returns nil if not found.
func (t *TemplateAttribute) GetIdx(s string, idx int) *Attribute {
	if t == nil {
		return nil
	}

	for i := range t.Attribute {
		if t.Attribute[i].AttributeName == s && t.Attribute[i].AttributeIndex == idx {
			return &t.Attribute[i]
		}
	}

	return nil
}

// GetTag returns a reference to the first Attribute in the list matching the tag.
// Returns nil if not found.
func (t *TemplateAttribute) GetTag(tag ttlv.Tag) *Attribute {
	return t.Get(tag.String())
}

// GetTagIdx returns a reference to the first Attribute in the list matching the tag and index.
// Returns nil if not found.
func (t *TemplateAttribute) GetTagIdx(tag ttlv.Tag, idx int) *Attribute {
	return t.GetIdx(tag.String(), idx)
}

func (t *TemplateAttribute) GetAll(s string) []Attribute {
	if t == nil {
		return nil
	}

	var ret []Attribute

	for i := range t.Attribute {
		if t.Attribute[i].AttributeName == s {
			ret = append(ret, t.Attribute[i])
		}
	}

	return ret
}

func (t *TemplateAttribute) Append(tag ttlv.Tag, value interface{}) {
	t.Attribute = append(t.Attribute, NewAttributeFromTag(tag, 0, value))
}

func (t *TemplateAttribute) GetAllTag(tag ttlv.Tag) []Attribute {
	return t.GetAll(tag.String())
}
