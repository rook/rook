kmip-go [![GoDoc](https://godoc.org/github.com/gemalto/kmip-go?status.png)](https://godoc.org/github.com/gemalto/kmip-go) [![Go Report Card](https://goreportcard.com/badge/github.com/gemalto/kmip-go)](https://goreportcard.com/report/gemalto/kmip-go) [![Build](https://github.com/gemalto/kmip-go/workflows/Build/badge.svg)](https://github.com/gemalto/kmip-go/actions?query=branch%3Amaster+workflow%3ABuild+)
=======

kmip-go is a go implemenation of KMIP protocol primitives.  It supports marshaling data in TTLV, XML, or JSON
encodings to and from go values and structs.  It can be used to implement KMIP clients or servers.

Installation
------------

    go get github.com/gemalto/kmip-go
    
Or, to just install the `ppkmip` pretty printing tool:

    go install github.com/gemalto/kmip-go/cmd/ppkmip
    
Packages
--------

The `ttlv` package implements the core encoder and decoder logic.

The `kmip14` package contains constants for all the tags, types, enumerations and bitmasks defined in the KMIP 1.4
specification.   It also contains mappings from these values to the normalized names used in the JSON and XML
encodings, and the canonical names used in Attribute structures.  
The `kmip14` definitions are all automatically registered with `ttlv.DefaultRegistry`.

The `kmip20` package adds additional enumeration values from the 2.0 specification.  It is meant to be registered
on top of the 1.4 definitions.

The root package defines golang structures for some of the significant Structure definitions in the 1.4 
specification, like Attributes, Request, Response, etc.  It is incomplete, but can be used as an example
for defining other structures.  It also contains an example of a client and server.

`cmd/kmipgen` is a code generation tool which generates the tag and enum constants from a JSON specification
input.  It can also be used independently in your own code to generate additional tags and constants.  `make install`
to build and install the tool.  See `kmip14/kmip_1_4.go` for an example of using the tool.

`cmd/kmipgen` is a tool for pretty printing kmip values.  It can accept KMIP input from stdin or files, encoded
in TTLV, XML, or JSON, and output in a variety of formats.  `make install` to intall the tool, and 
`ppkmip --help` to see usage.

Contributing
------------

To build, be sure to have a recent go SDK, and make.  Run `make tools` to install other dependencies.

There is also a dockerized build, which only requires make and docker-compose: `make docker`.  You can also
do `make fish` or `make bash` to shell into the docker build container.

Merge requests are welcome!  Before submitting, please run `make` and make sure all tests pass and there are
no linter findings.