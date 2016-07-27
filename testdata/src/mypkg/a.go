package mypkg

/* uses GOPATH=/path/to/godefinfo/testdata */
import (
	"mypkg/subpkg"
	"strings"
)

func init() {
	A0               // mypkg A0
	a1               // mypkg a1
	a2               // mypkg a2
	a3               // mypkg a3
	(a4{}).a5        // mypkg a4 a5
	B0               // mypkg B0
	b1               // mypkg b1
	b2               // mypkg b2
	b3               // mypkg b3
	(b4{}).b5        // mypkg b4 b5
	subpkg.C0        // mypkg/subpkg C0
	(subpkg.C1{}).C2 // mypkg/subpkg C1 C2
	strings.Contains // strings Contains

	subpkg.C3{C4: subpkg.C1} //C4: mypkg/subpkg C3 C4
}

func A0() {}

func a1() {}

var a2 string

const a3 = 123

type a4 struct{}

func (a4) a5() {}
