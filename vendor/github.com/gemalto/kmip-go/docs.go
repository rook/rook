// Package kmip is a general purpose KMIP library for implementing KMIP services and clients.
//
// The ttlv sub package contains the core logic for parsing the KMIP TTLV encoding formats,
// and marshaling them to and from golang structs.
//
// This package defines structs for many of the structures defined in the KMIP Spec, such as
// the different types of managed objects, request and response bodies, etc.  Not all Structures
// are represented here yet, but the ones that are can be used as examples.
//
// There is also a partial implementation of a server, and an example of a client.  There is
// currently no Client type for KMIP, but it is simple to open a socket overwhich you send
// and receive raw KMIP requests and responses.
package kmip
