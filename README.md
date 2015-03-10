pray
====

[![Build Status](https://travis-ci.org/fsouza/pray.png)](https://travis-ci.org/fsouza/pray)
[![GoDoc](https://godoc.org/github.com/fsouza/pray?status.png)](https://godoc.org/github.com/fsouza/pray)

pray is a tool for finding unused public variables, constants, types, functions
and methods. Given a package, it will explore all public items in that package
and check for the usage of these items in a set of specified packages. Here is
a sample usage:

```
% pray -src github.com/tsuru/config github.com/tsuru/tsuru/app
/Users/f/src/github.com/tsuru/config/checker.go:10:6: Check is unused
/Users/f/src/github.com/tsuru/config/config.go:113:6: WriteConfigFile is unused
/Users/f/src/github.com/tsuru/config/config.go:229:6: GetUint is unused
/Users/f/src/github.com/tsuru/config/config.go:75:6: ReadAndWatchConfigFile is unused
/Users/f/src/github.com/tsuru/config/config.go:296:6: GetList is unused
/Users/f/src/github.com/tsuru/config/config.go:208:6: GetFloat is unused
```

The flag ``-src`` specifies the source packages, and then users can provide the
set of packages for searching. Users can also use the Go notation for expanding subpackages:

```
% pray -src github.com/tsuru/config github.com/tsuru/tsuru/...
```

Current status
--------------

Currently, it works, but may eat your entire system memory. It also gives false
negatives when analyzing public methods of a type that implement an interface,
and the method is called only through the interface, so it still reports that
MyError.Error is unused in the code below:

```
type MyError struct {
	message string
}

func (err *MyError) Error() string {
	return err.message
}

func getError() string {
	var err error = &MyError{}
	return err.Error()
}
```

Contributing
------------

Feel free to send any pull requests or open issues. Please ensure that your
code follow [gofmt](https://golang.org/cmd/gofmt/) and
[goimports](https://godoc.org/golang.org/x/tools/cmd/goimports) rules.
