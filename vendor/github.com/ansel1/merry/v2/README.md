merry [![Build](https://github.com/ansel1/merry/workflows/Build/badge.svg)](https://github.com/ansel1/merry/actions?query=branch%3Amaster+workflow%3ABuild+) [![GoDoc](https://godoc.org/github.com/ansel1/merry/v2?status.png)](https://godoc.org/github.com/ansel1/merry/v2) [![Go Report Card](https://goreportcard.com/badge/github.com/ansel1/merry/v2)](https://goreportcard.com/report/github.com/ansel1/merry/v2)
=====

Add context to errors, including automatic stack capture, cause chains, HTTP status code, user
messages, and arbitrary values.

The package is largely based on http://github.com/go-errors/errors, with additional
inspiration from https://github.com/go-errgo/errgo and https://github.com/amattn/deeperror.

Installation
------------

    go get github.com/ansel1/merry/v2

Features
--------

Wrapped errors work a lot like google's golang.org/x/net/context package:
each wrapper error contains the inner error, a key, and a value.
Like contexts, errors are immutable: adding a key/value to an error
always creates a new error which wraps the original.

This package comes with built-in support for adding information to errors:

* stacktraces
* changing the error message
* HTTP status codes
* End user error messages
* causes

You can also add your own additional information.

The stack capturing feature can be turned off for better performance, though it's pretty fast.  Benchmarks
on an 2017 MacBook Pro, with go 1.10:

    BenchmarkNew_withStackCapture-8      	 2000000	       749 ns/op
    BenchmarkNew_withoutStackCapture-8   	20000000	        64.1 ns/op

Merry errors are fully compatible with errors.Is, As, and Unwrap.

Example
-------

To add a stacktrace, a user message, and an HTTP code to an error:

    err = merry.Wrap(err, WithUserMessagef("Username %s not found.", username), WithHTTPCode(404))

To fetch context information from error:

    userMsg := UserMessage(err)
    statusCode := HTTPCode(err)
    stacktrace := Stacktrace(err)

To print full details of the error:

    log.Printf("%v+", err)  // or log.Println(merry.Details(err))

v1 -> v2
--------

v1 used a fluent API style which proved awkward in some cases.  In general, fluent APIs 
don't work that well in go, because they interfere with how interfaces are typically used to 
compose with other packages.  v2 uses a functional options API style which more easily 
allows other packages to augment this one.

This also fixed bad smell with v1's APIs: they mostly returned a big, ugly `merry.Error` interface, 
instead of plain `error` instances.  v2 has a smaller, simpler API, which exclusively uses plain
errors.

v2 also implements a simpler and more robust integration with errors.Is/As/Unwrap.  v2's functions will
work with error wrapper chains even if those chains contain errors not created with this package, so
long as those errors conform to the Unwrap() convention.

v2 allows more granular control over stacktraces: stack capture can be turned on or off on individual errors,
overriding the global setting.  External stacktraces can be used as well.

v1 has been reimplemented in terms of v2, and the versions are completely compatible, and can be mixed.

Plugs
-----

- Check out my HTTP client library: [github.com/gemalto/requester](https://github.com/gemalto/requester)
- Check out my log library: [github.com/gemalto/flume](https://github.com/gemalto/flume)

License
-------

This package is licensed under the MIT license, see LICENSE.MIT for details.
