## godefinfo - find symbol information in Go source

godefinfo, given a location in a source file, prints information about
the identifier's definition: package import path, parent type name
(for methods and fields), and name.

It is based on rogpeppe's [godef](https://github.com/rogpeppe/godef),
but it uses the Go standard library's `go/types` instead of a custom
variant, and it removes support for acme and the `-a -A -t` flags.
