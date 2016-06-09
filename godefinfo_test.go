package main

import (
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

var writeGoFile = flag.Bool("test.write-go-file", false, "write the test .go file to disk for easier debugging (run with -test.v to see filename)")

func init() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	build.Default.GOPATH = filepath.Join(dir, "testdata")
	minimalEnv = []string{"GOPATH=" + build.Default.GOPATH, "GOROOT=" + runtime.GOROOT()}
}

var minimalEnv []string

func TestSingleFile(t *testing.T) {
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
	http //http: net/http
	http.DefaultClient // net/http DefaultClient
	http.DefaultClient.Transport // net/http Client Transport
	http.DefaultClient.Transport.RoundTrip // net/http RoundTripper RoundTrip
	http.DefaultClient.Do // net/http Client Do

	x, err := http.Get("http://example.com")
	x.Body // net/http Response Body

	w := http.ResponseWriter(nil)
	w.Header().Set // net/http Header Set
}

func F() {} //F: p F

type T struct { //T: p T
	F0 int //F0: p T F0
	S
	*P
	K //K: p T K
}

func (T) M0() {}
func (*T) M1() {}
func (_ T) M2() {}
func (_ *T) M3() {}
func (t T) M4() {}
func (t *T) M5() {} //M5: p T M5

type S struct { F1 int }

func (S) M6() {}

type P struct {
	I
	F2 int
}

func (P) M7() {}

type I interface { //I: p I
	L //L: p L
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
	M12() //M12: p L M12
}

var M = 1 //M: p M

const N = 2 //N: p N
`

	const filename = "/tmp/godef_testdata.go"
	if *writeGoFile {
		if err := ioutil.WriteFile(filename, []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
		t.Log("wrote test file to", filename)
	}

	testFile(t, filename, src)
}

func TestGOPATH(t *testing.T) {
	cmd := exec.Command("go", "install", "mypkg/subpkg")
	cmd.Env = minimalEnv
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build mypkg/subpkg failed: %s (output follows)\n\n%s", err, out)
	}

	filenames := []string{
		"mypkg/a.go",
		"mypkg/b.go",
		"mypkg/subpkg/c.go",
	}
	for _, filename := range filenames {
		filename, err := filepath.Abs(filepath.Join("testdata/src", filename))
		if err != nil {
			t.Fatal(err)
		}
		src, err := ioutil.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		testFile(t, filename, string(src))
	}
}

func testFile(t *testing.T, filename, src string) {
	pat := regexp.MustCompile(`\s*(?P<ref>.+)\s*//(?:(?P<tok>\w+):)? (?P<pkg>[\w/.-]+)(?: (?P<name1>\w+)(?: (?P<name2>\w+))?)?`)
	matches := pat.FindAllStringSubmatchIndex(src, -1)
	if numTests := strings.Count(src, " //"); len(matches) != numTests {
		t.Fatalf("%s: source has %d tests (lines with ' // '), but %d matches found (regexp probably needs to be updated to include new styles of test specifications)", filename, numTests, len(matches))
	}
	for _, m := range matches {
		ref := src[m[2]:m[3]]

		// Narrow the ref if the tok is provided.
		var tokIdxInRef, tokLen int
		if m[4] == -1 {
			// Take right-most component of dotted selector.
			tokIdxInRef = strings.LastIndex(ref, ".")
			if tokIdxInRef == -1 {
				tokIdxInRef = 0
			}
			tokLen = len(ref) - tokIdxInRef
		} else {
			tok := src[m[4]:m[5]]
			tokLen = len(tok)
			tokIdxInRef = strings.Index(ref, tok)
			if tokIdxInRef == -1 {
				t.Errorf("%s: could not find token %q in ref %q", filename, tok, ref)
				continue
			}
		}
		ref = ref[tokIdxInRef : tokIdxInRef+tokLen]
		m[2] += tokIdxInRef

		m[2]++
		wantPkg := src[m[6]:m[7]]
		var wantName1, wantName2 string
		if m[8] != -1 {
			wantName1 = src[m[8]:m[9]]
		}
		if m[10] != -1 {
			wantName2 = src[m[10]:m[11]]
		}

		label := fmt.Sprintf("%s: ref %q at offset %d", filename, ref, m[2])

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
	cmd := exec.Command("godefinfo", "-i", "-o", strconv.Itoa(offset), "-f", filename, "-strict", "-importsrc")
	cmd.Env = minimalEnv
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
