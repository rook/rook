// Package ttlv encodes and decodes the 3 wire formats defined in the KMIP specification:
//
// 1. TTLV (the default, binary wire format)
// 2. JSON
// 3. XML
//
// The core representation of KMIP values is the ttlv.TTLV type, which is
// a []byte encoded in the TTLV binary format.  The ttlv.TTLV type knows how to marshal/
// unmarshal to and from the JSON and XML encoding formats.
//
// This package also knows how to marshal and unmarshal ttlv.TTLV values to golang structs,
// in a way similar to the json or xml packages.
//
// See Marshal() and Unmarshal() for the rules about how golang values map to KMIP TTLVs.
// Encoder and Decoder can be used to process streams of KMIP values.
//
// This package holds a registry of type, tag, and enum value names, which are used to transcode
// strings into these values. KMIP 1.4 names will be automatically loaded into the
// DefaultRegistry.  See the kmip20 package to add definitions for 2.0 names.
//
// Print() and PrettyPrintHex() can be used to debug TTLV values.
package ttlv
