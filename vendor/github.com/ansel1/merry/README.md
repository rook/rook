merry [![Build](https://github.com/ansel1/merry/workflows/Build/badge.svg)](https://github.com/ansel1/merry/actions?query=branch%3Amaster+workflow%3ABuild+) [![GoDoc](https://godoc.org/github.com/ansel1/merry?status.png)](https://godoc.org/github.com/ansel1/merry) [![Go Report Card](https://goreportcard.com/badge/github.com/ansel1/merry)](https://goreportcard.com/report/github.com/ansel1/merry)
=====

Add context to errors, including automatic stack capture, cause chains, HTTP status code, user
messages, and arbitrary values.
            
The package is largely based on http://github.com/go-errors/errors, with additional
inspiration from https://github.com/go-errgo/errgo and https://github.com/amattn/deeperror.

V2
-- 

[github.com/ansel1/merry/v2](https://github.com/ansel1/merry/tree/master/v2) now replaces v1.  v1 will continue to be supported.  v1 has been re-implemented
in terms of v2, and the two packages can be used together and interchangeably.

There are some small enhancements and changes to v1 with the introduction of v2:

- err.Error() now *always* just prints out the basic error message.  It no longer prints out details,
  user message, or cause.  VerboseDefault() and SetVerboseDefault() no longer have any effect.  To 
  print more detailed error information, you must use fmt:
  
        // print err message and cause chain
        fmt.Printf("%v", err)    // %s works too
  
        // print details, same as Details(err)
        fmt.Printf("%v+", err) 
  
- MaxStackDepth is no longer supported.  Setting it has no effect.  It has been replaced with
  GetMaxStackDepth() and SetMaxStackDepth(), which delegate to corresponding v2 functions.
- New, Errorf, Wrap, and WrapSkipping now accept v2.Wrapper arguments, allowing a mixture of 
  v1's fluent API style and v2's option-func API style.
- Compatibility with other error wrapping libraries is improved.  All functions which extract
  a value from an error will now search the entire chain of errors, even if errors created by
  other libraries are inserted in the middle of the chain, so long as those errors implement
  Unwrap().

Installation
------------

    go get github.com/ansel1/merry
    
Features
--------

Merry errors work a lot like google's golang.org/x/net/context package.
Merry errors wrap normal errors with a context of key/value pairs.
Like contexts, merry errors are immutable: adding a key/value to an error
always creates a new error which wraps the original.  

`merry` comes with built-in support for adding information to errors:

* stacktraces
* overriding the error message
* HTTP status codes
* End user error messages
 
You can also add your own additional information.

The stack capturing feature can be turned off for better performance, though it's pretty fast.  Benchmarks
on an 2017 MacBook Pro, with go 1.10:

    BenchmarkNew_withStackCapture-8      	 2000000	       749 ns/op
    BenchmarkNew_withoutStackCapture-8   	20000000	        64.1 ns/op

Details
-------

* Support for go 2's errors.Is and errors.As functions
* New errors have a stacktrace captured where they are created
* Add a stacktrace to existing errors (captured where they are wrapped)

    ```go
    err := lib.Read()
    return merry.Wrap(err)  // no-op if err is already merry
    ```
        
* Add a stacktrace to a sentinel error

    ```go
    var ParseError = merry.New("parse error")
    
    func Parse() error {
    	// ...
        return ParseError.Here() // captures a stacktrace here
    }
    ```
  
* The golang idiom for testing errors against sentinel values or type checking them
  doesn't work with merry errors, since they are wrapped.  Use Is() for sentinel value
  checks, or the new go 2 errors.As() function for testing error types. 
  
    ```go
    err := Parse()
  
    // sentinel value check
    if merry.Is(err, ParseError) {
       // ...
    }
  
    // type check
    if serr, ok := merry.Unwrap(err).(*SyntaxError); ok {
      // ...
    }
  
    // these only work in go1.13
    
    // sentinel value check
    if errors.Is(err, ParseError) {}
  
    // type check
    var serr *SyntaxError
    if errors.As(err, &serr) {}
    ```
        
* Add to the message on an error.

    ```go
    err := merry.Prepend(ParseError, "reading config").Append("bad input")
    fmt.Println(err.Error()) // reading config: parse error: bad input
    ```
        
* Hierarchies of errors

    ```go
    var ParseError = merry.New("Parse error")
    var InvalidCharSet = merry.WithMessage(ParseError, "Invalid char set")
    var InvalidSyntax = merry.WithMessage(ParseError, "Invalid syntax")
    
    func Parse(s string) error {
        // use chainable methods to add context
        return InvalidCharSet.Here().WithMessagef("Invalid char set: %s", "UTF-8")
        // or functions
        // return merry.WithMessagef(merry.Here(InvalidCharSet), "Invalid char set: %s", "UTF-8")
    }
    
    func Check() {
        err := Parse("fields")
        merry.Is(err, ParseError) // yup
        merry.Is(err, InvalidCharSet) // yup
        merry.Is(err, InvalidSyntax) // nope
    }
    ```
        
* Add an HTTP status code

    ```go
    merry.HTTPCode(errors.New("regular error")) // 500
    merry.HTTPCode(merry.New("merry error").WithHTTPCode(404)) // 404
    ```

* Set an alternate error message for end users
 
    ```go
    e := merry.New("crash").WithUserMessage("nothing to see here")
    merry.UserMessage(e)  // returns "nothing to see here"
    ```
        
* Functions for printing error details
 
    ```go
    err := merry.New("boom")
    m := merry.Stacktrace(err) // just the stacktrace
    m = merry.Details(err) // error message and stacktrace
    fmt.Sprintf("%+v", err) == merry.Details(err) // errors implement fmt.Formatter
    ```
   
* Add your own context info

    ```go
    err := merry.New("boom").WithValue("explosive", "black powder")
    ```
    
Basic Usage
-----------

The package contains functions for creating new errors with stacks, or adding a stack to `error` 
instances.  Functions with add context (e.g. `WithValue()`) work on any `error`, and will 
automatically convert them to merry errors (with a stack) if necessary.

Capturing the stack can be globally disabled with `SetStackCaptureEnabled(false)`

Functions which get context values from errors also accept `error`, and will return default
values if the error is not merry, or doesn't have that key attached.

All the functions which create or attach context return concrete instances of `*Error`.  `*Error`
implements methods to add context to the error (they mirror the functions and do
the same thing).  They allow for a chainable syntax for adding context.

Example:

```go
package main

import (
    "github.com/ansel1/merry"
    "errors"
)

var InvalidInputs = errors.New("Input is invalid")

func main() {
    // create a new error, with a stacktrace attached
    err := merry.New("bad stuff happened")
    
    // create a new error with format string, like fmt.Errorf
    err = merry.Errorf("bad input: %v", os.Args)
    
    // capture a fresh stacktrace from this callsite
    err = merry.Here(InvalidInputs)
    
    // Make err merry if it wasn't already.  The stacktrace will be captured here if the
    // error didn't already have one.  Also useful to cast to *Error 
    err = merry.Wrap(err, 0)

    // override the original error's message
    err.WithMessagef("Input is invalid: %v", os.Args)
    
    // Use Is to compare errors against values, which is a common golang idiom
    merry.Is(err, InvalidInputs) // will be true
    
    // associated an http code
    err.WithHTTPCode(400)
    
    perr := parser.Parse("blah")
    err = Wrap(perr, 0)
    // Get the original error back
    merry.Unwrap(err) == perr  // will be true
    
    // Print the error to a string, with the stacktrace, if it has one
    s := merry.Details(err)
    
    // Just print the stacktrace (empty string if err is not a RichError)
    s := merry.Stacktrace(err)

    // Get the location of the error (the first line in the stacktrace)
    file, line := merry.Location(err)
    
    // Get an HTTP status code for an error.  Defaults to 500 for non-nil errors, and 200 if err is nil.
    code := merry.HTTPCode(err)
    
}
```
    
See inline docs for more details.

Plugs
-----

- Check out my HTTP client library: [github.com/gemalto/requester](https://github.com/gemalto/requester)
- Check out my log library: [github.com/gemalto/flume](https://github.com/gemalto/flume)

License
-------

This package is licensed under the MIT license, see LICENSE.MIT for details.
