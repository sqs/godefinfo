package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var writeGoFile = flag.Bool("test.write-go-file", false, "write the test .go file to disk for easier debugging (run with -test.v to see filename)")

func TestGodefinfo(t *testing.T) {
	const src = `package p

import "net/http"

func init() {
	F // p F
	T // p T
	(&T{}).F0 // p T F0
	(T{}).F0 // p T F0
	(&T{}).F1 // p S F1
	(T{}).F1 // p S F1
	(&T{}).F2 // p P F2
	(T{}).F2 // p P F2
	(&T{}).M0 // p T M0
	(T{}).M0 // p T M0
	(&T{}).M1 // p T M1
	(&T{}).M2 // p T M2
	(T{}).M2 // p T M2
	(&T{}).M3 // p T M3
	(&T{}).M4 // p T M4
	(T{}).M4 // p T M4
	(&T{}).M5 // p T M5
	(T{}).M6 // p S M6
	(&T{}).M6 // p S M6
	(&T{}).M7 // p P M7
	I(nil).M8 // p I M8
	(&T{}).M8 // p I M8
	(T{}).M8 // p I M8
	(&P{}).M8 // p I M8
	(P{}).M8 // p I M8
	I(nil).M9 // p J M9
	J(nil).M9 // p J M9
	(&T{}).M9 // p J M9
	(T{}).M9 // p J M9
	(&P{}).M9 // p J M9
	(P{}).M9 // p J M9
	K(nil).M10 // p K M10
	(&T{}).M10 // p K M10
	(T{}).M10 // p K M10
	I(nil).M11 // p L M11
	L(nil).M11 // p L M11
	(&T{}).M11 // p L M11
	(T{}).M11 // p L M11
	I(nil).M12 // p L M12
	L(nil).M12 // p L M12
	(&T{}).M12 // p L M12
	(T{}).M12 // p L M12

	error // builtin error
	string // builtin string
	make // builtin make
	http // net/http
	http.DefaultClient // net/http DefaultClient
	http.DefaultClient.Transport // net/http Client Transport
	http.DefaultClient.Transport.RoundTrip // net/http RoundTripper RoundTrip
	http.DefaultClient.Do // net/http Client Do

	x, err := http.Get("http://example.com")
	x.Body // net/http Response Body
}

func F() {}

type T struct {
	F0 int
	S
	*P
	K
}

func (T) M0() {}
func (*T) M1() {}
func (_ T) M2() {}
func (_ *T) M3() {}
func (t T) M4() {}
func (t *T) M5() {}

type S struct { F1 int }

func (S) M6() {}

type P struct {
	I
	F2 int
}

func (P) M7() {}

type I interface {
	L
	M8()
	J
}

type J interface {
	M9()
}

type K interface {
	M10()
}

type L interface {
	M11()
	M12()
}
`

	const filename = "/tmp/godef_testdata.go"
	if *writeGoFile {
		if err := ioutil.WriteFile(filename, []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
		t.Log("wrote test file to", filename)
	}

	pat := regexp.MustCompile(`(?:\t|\.)(?P<ref>\w+) // (?P<pkg>[\w/.-]+)(?: (?P<name1>\w+)(?: (?P<name2>\w+))?)?`)
	matches := pat.FindAllStringSubmatchIndex(src, -1)
	if numTests := strings.Count(src, " // "); len(matches) != numTests {
		t.Fatalf("source has %d tests (lines with ' // '), but %d matches found (regexp probably needs to be updated to include new styles of test specifications)", numTests, len(matches))
	}
	for _, m := range matches {
		ref := src[m[2]:m[3]]
		wantPkg := src[m[4]:m[5]]
		var wantName1, wantName2 string
		if m[6] != -1 {
			wantName1 = src[m[6]:m[7]]
		}
		if m[8] != -1 {
			wantName2 = src[m[8]:m[9]]
		}

		label := fmt.Sprintf("ref %q at offset %d", ref, m[2])

		var out string
		pkg, name1, name2, err := check(filename, src, m[2], &out)
		if err != nil {
			t.Errorf("%s: error: %s", label, err)
			continue
		}

		want := fmt.Sprintf("%s %s %s", wantPkg, wantName1, wantName2)
		got := fmt.Sprintf("%s %s %s", pkg, name1, name2)

		if got != want {
			t.Errorf("%s: got %q, want %q", label, got, want)
			if out != got {
				t.Logf("%s: output: %q", label, out)
			}
		} else {
			// t.Logf("%s: PASS", label)
		}
	}
}

func check(filename, src string, offset int, saveOutput *string) (pkg, name1, name2 string, err error) {
	cmd := exec.Command("godefinfo", "-i", "-o", strconv.Itoa(offset), "-f", filename)
	cmd.Stdin = ioutil.NopCloser(strings.NewReader(src))
	outB, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("%s (output was: %q)", err, outB)
		return
	}

	out := strings.TrimSuffix(string(outB), "\n")
	if saveOutput != nil {
		*saveOutput = out
	}
	parts := strings.SplitN(out, " ", 3)
	if len(parts) == 0 {
		err = fmt.Errorf("bad output: %q", out)
		return
	}

	pkg = parts[0]
	if len(parts) > 1 {
		name1 = parts[1]
	}
	if len(parts) == 3 {
		name2 = parts[2]
	}
	return
}
