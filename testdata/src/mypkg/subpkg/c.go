package subpkg

func C0() {}

type C1 struct{}

func (C1) C2() {}

type C3 struct {
	C4 C1
}
