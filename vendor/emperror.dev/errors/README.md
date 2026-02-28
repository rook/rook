# Emperror: Errors [![Mentioned in Awesome Go](https://awesome.re/mentioned-badge-flat.svg)](https://github.com/avelino/awesome-go#error-handling)

[![GitHub Workflow Status](https://img.shields.io/github/workflow/status/emperror/errors/CI?style=flat-square)](https://github.com/emperror/errors/actions?query=workflow%3ACI)
[![Codecov](https://img.shields.io/codecov/c/github/emperror/errors?style=flat-square)](https://codecov.io/gh/emperror/errors)
[![Go Report Card](https://goreportcard.com/badge/emperror.dev/errors?style=flat-square)](https://goreportcard.com/report/emperror.dev/errors)
![Go Version](https://img.shields.io/badge/go%20version-%3E=1.12-61CFDD.svg?style=flat-square)
[![PkgGoDev](https://pkg.go.dev/badge/mod/emperror.dev/errors)](https://pkg.go.dev/mod/emperror.dev/errors)
[![FOSSA Status](https://app.fossa.com/api/projects/custom%2B8125%2Femperror.dev%2Ferrors.svg?type=shield)](https://app.fossa.com/projects/custom%2B8125%2Femperror.dev%2Ferrors?ref=badge_shield)


**Drop-in replacement for the standard library `errors` package and [github.com/pkg/errors](https://github.com/pkg/errors).**

This is a single, lightweight library merging the features of standard library `errors` package
and [github.com/pkg/errors](https://github.com/pkg/errors). It also backports a few features
(like Go 1.13 error handling related features).

Standard library features:
- `New` creates an error with stack trace
- `Unwrap` supports both Go 1.13 wrapper (`interface { Unwrap() error }`) and **pkg/errors** causer (`interface { Cause() error }`) interface
- Backported `Is` and `As` functions

[github.com/pkg/errors](https://github.com/pkg/errors) features:
- `New`, `Errorf`, `WithMessage`, `WithMessagef`, `WithStack`, `Wrap`, `Wrapf` functions behave the same way as in the original library
- `Cause` supports both Go 1.13 wrapper (`interface { Unwrap() error }`) and **pkg/errors** causer (`interface { Cause() error }`) interface

Additional features:
- `NewPlain` creates a new error without any attached context, like stack trace
- `Sentinel` is a shorthand type for creating [constant error](https://dave.cheney.net/2016/04/07/constant-errors)
- `WithStackDepth` allows attaching stack trace with a custom caller depth
- `WithStackDepthIf`, `WithStackIf`, `WrapIf`, `WrapIff` only annotate errors with a stack trace if there isn't one already in the error chain
- Multi error aggregating multiple errors into a single value
- `NewWithDetails`, `WithDetails` and `Wrap*WithDetails` functions to add key-value pairs to an error
- Match errors using the `match` package


## Installation

```bash
go get emperror.dev/errors
```


## Usage

```go
package main

import "emperror.dev/errors"

// ErrSomethingWentWrong is a sentinel error which can be useful within a single API layer.
const ErrSomethingWentWrong = errors.Sentinel("something went wrong")

// ErrMyError is an error that can be returned from a public API.
type ErrMyError struct {
	Msg string
}

func (e ErrMyError) Error() string {
	return e.Msg
}

func foo() error {
	// Attach stack trace to the sentinel error.
	return errors.WithStack(ErrSomethingWentWrong)
}

func bar() error {
	return errors.Wrap(ErrMyError{"something went wrong"}, "error")
}

func main() {
	if err := foo(); err != nil {
		if errors.Cause(err) == ErrSomethingWentWrong { // or errors.Is(ErrSomethingWentWrong)
			// handle error
		}
	}

	if err := bar(); err != nil {
		if errors.As(err, &ErrMyError{}) {
			// handle error
		}
	}
}
```

Match errors:

```go
package main

import (
    "emperror.dev/errors"
    "emperror.dev/errors/match"
)

// ErrSomethingWentWrong is a sentinel error which can be useful within a single API layer.
const ErrSomethingWentWrong = errors.Sentinel("something went wrong")

type clientError interface{
    ClientError() bool
}

func foo() error {
	// Attach stack trace to the sentinel error.
	return errors.WithStack(ErrSomethingWentWrong)
}

func main() {
    var ce clientError
    matcher := match.Any{match.As(&ce), match.Is(ErrSomethingWentWrong)}

	if err := foo(); err != nil {
		if matcher.MatchError(err) {
			// you can use matchers to write complex conditions for handling (or not) an error
            // used in emperror
		}
	}
}
```


## Development

Contributions are welcome! :)

1. Clone the repository
1. Make changes on a new branch
1. Run the test suite:
    ```bash
    ./pleasew build
    ./pleasew test
    ./pleasew gotest
    ./pleasew lint
    ```
1. Commit, push and open a PR


## License

The MIT License (MIT). Please see [License File](LICENSE) for more information.

Certain parts of this library are inspired by (or entirely copied from) various third party libraries.
Their licenses can be found in the [Third Party License File](LICENSE_THIRD_PARTY).

[![FOSSA Status](https://app.fossa.com/api/projects/custom%2B8125%2Femperror.dev%2Ferrors.svg?type=large)](https://app.fossa.com/projects/custom%2B8125%2Femperror.dev%2Ferrors?ref=badge_large)
