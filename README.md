## godefinfo - find symbol information in Go source

godefinfo, given a location in a source file, prints information about
the identifier's definition: package import path, parent type name
(for methods and fields), and name.

It is based on rogpeppe's [godef](https://github.com/rogpeppe/godef),
but it uses the Go standard library's `go/types` instead of a custom
variant, and it removes support for acme and the `-a -A -t` flags.

### Sample output

For the character offset denoted with the caret:

```
package foo

import "net/http"

func bar() {
	resp, _ := http.Get("http://example.com")
	resp.Body
	     ^
}
```

godefinfo will output:

```
net/http Response Body
```

### Installation

```
go get -u github.com/sqs/godefinfo
```

### Usage

See `godefinfo -h` for more information.

```
# prints information about the identifier at offset 1234
godefinfo -o 1234 -f /path/to/go/file.go
```

## Using in your editor

If you prefer to see godefinfo-style output over godef output

godefinfo is very easy to install (it intentionally has zero
dependencies outside the Go standard library).

```
GOPATH=/tmp/MYTEMPDIR GOBIN=/tmp/MAYBE-A-DIR-IN-YOUR-EDITOR-PLUGIN-DATA-DIR go get -u github.com/sqs/godefinfo
rm -rf /tmp/MYTEMPDIR # no need to keep the source code around
```

You can also install without needing `git` or to clone the whole repository:

```
curl -sSL https://raw.githubusercontent.com/sqs/godefinfo/master/godefinfo.go > /tmp/godefinfo.go && go build -o /tmp/godefinfo /tmp/godefinfo.go
rm -rf /tmp/godefinfo.go
```

Then the godefinfo program will be available at `/tmp/MAYBE-A-DIR-IN-YOUR-EDITOR-PLUGIN-DATA-DIR/godefinfo` or wherever you installed it.

## Requirements

* Go 1.6+ (otherwise it will fail with compilation error on `types.ImporterFrom`)
