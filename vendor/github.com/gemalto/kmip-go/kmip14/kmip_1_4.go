//go:generate go run ../cmd/kmipgen/main.go -o kmip_1_4_generated.go -i kmip_1_4.json -p kmip14

// Package kmip14 contains tag and enumeration value definitions from the 1.4 specification.
// These definitions will be registered automatically into the DefaultRegistry.
//
// Each tag is stored in a package constant, named Tag<normalized KMIP name>.
// Bitmask and Enumeration values are each represented by a type, named
// after the normalized name of the values set from the spec, e.g.
package kmip14

import (
	"github.com/gemalto/kmip-go/ttlv"
)

//nolint:gochecknoinits
func init() {
	Register(&ttlv.DefaultRegistry)
}

// Registers the 1.4 enumeration values with the registry.
func Register(registry *ttlv.Registry) {
	RegisterGeneratedDefinitions(registry)
}
