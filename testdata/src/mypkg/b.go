package mypkg

/* uses GOPATH=/path/to/godefinfo/testdata */
import "mypkg/subpkg"

func init() {
	subpkg.C0()
}

func B0() {}

func b1() {}

var b2 string

const b3 = 123

type b4 struct{}

func (b4) b5() {}
