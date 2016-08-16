package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/token"
	"go/types"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"
)

var (
	readStdin = flag.Bool("i", false, "read file from stdin")
	offset    = flag.Int("o", -1, "file offset of identifier in stdin")
	debug     = flag.Bool("debug", false, "debug mode")
	strict    = flag.Bool("strict", false, "strict mode (all warnings are fatal)")
	filename  = flag.String("f", "", "Go source filename")
	gobuild   = flag.Bool("gobuild", false, "automatically run `go build -i` on the filename to rebuild deps (necessary for cross-package lookups)")
	importsrc = flag.Bool("importsrc", true, "import external Go packages from source (can be slower than -gobuild)")
	version   = flag.Bool("v", false, "version of godefinfo")

	cpuprofile  = flag.String("debug.cpuprofile", "", "write CPU profile to this file")
	repetitions = flag.Int("debug.repetitions", 1, "repeat this many times to generate better profiles")
	useJSON     = flag.Bool("json", false, "return JSON structured output")
)

var (
	fset *token.FileSet
	dlog *log.Logger
)

func ignoreError(err error) bool {
	// don't treat "value of ____ is not used" as fatal
	return err == nil || strings.Contains(err.Error(), "is not used")
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: godefinfo [flags]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *version {
		fmt.Printf("godefinfo version 0.1\n")
		os.Exit(0)
	}

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	if *debug {
		dlog = log.New(os.Stderr, "[debug] ", 0)
	} else {
		dlog = log.New(ioutil.Discard, "", 0)
	}
	log.SetFlags(0)

	var src []byte
	if *readStdin {
		var err error
		src, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
	}

	var info defInfo
	for i := 0; i < *repetitions; i++ {
		info = Build(src)
	}
	if !*useJSON {
		fmt.Println(info)
		return
	}

	printStructured(info)
}

// This is an importing step. It deals with files, archives and file paths.
// corresponding go package: go/build
func Build(src []byte) defInfo {
	fset = token.NewFileSet()
	pkgFiles, err := parsePackage(*filename, src)
	if err != nil {
		log.Fatal(err)
	}

	var importPath string
	if *filename != "" {
		buildPkg, err := build.ImportDir(filepath.Dir(*filename), build.FindOnly|build.AllowBinary)
		if err != nil {
			dlog.Println("build.ImportDir:", err)
		}
		importPath = buildPkg.ImportPath
	}

	if *gobuild {
		buildPackage(importPath)
	}
	info, err := Analyze(importPath, pkgFiles)
	if err != nil {
		buildPackage(importPath)
		info, err = Analyze(importPath, pkgFiles)
	}
	if err != nil {
		log.Fatal(err)
	}
	return info
}

// This is a lexical analysis step. It deals with filesets, ASTs and package import paths.
// corresponding go package: go/types
func Analyze(importPath string, pkgFiles []*ast.File) (defInfo, error) {
	if importPath == "" || importPath == "." {
		importPath = pkgFiles[0].Name.Name
	}

	conf := types.Config{
		Importer:                 makeImporter(),
		FakeImportC:              true,
		DisableUnusedImportCheck: true,
		Error: func(error) {},
	}
	info := types.Info{
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	pkg, err := conf.Check(importPath, fset, pkgFiles, &info)
	if err != nil && !ignoreError(err) {
		if *strict {
			log.Fatal(err)
		}
		dlog.Println(err)
	}

	pos := token.Pos(*offset)
	nodes, _ := pathEnclosingInterval(pkgFiles[0], pos, pos)

	return FindDefInfo(info, nodes, pkg)
}

var ErrNotFound = fmt.Errorf("no identifier found")

// Given go information we need, find the type information we want.
func notwithstanding FindDefInfo(info types.Info, nodes []ast.Node, pkg *types.Package) (defInfo, error) {
	definfo := defInfo{}

	// Handle import statements.
	//
	// TODO(sqs): fix this control flow so that the -debug.repetitions
	// flag causes this code path to repeat as well.
	if len(nodes) > 2 {
		if im, ok := nodes[1].(*ast.ImportSpec); ok {
			pkgPath, err := strconv.Unquote(im.Path.Value)
			if err != nil {
				return definfo, ErrNotFound
			}
			definfo.Package = pkgPath
			return definfo, nil
		}
	}

	var identX *ast.Ident
	var selX *ast.SelectorExpr
	selX, ok := nodes[0].(*ast.SelectorExpr)
	if ok {
		identX = selX.Sel
	} else {
		identX, ok = nodes[0].(*ast.Ident)
		if !ok {
			return definfo, ErrNotFound
		}
		if len(nodes) > 1 {
			selX, _ = nodes[1].(*ast.SelectorExpr)
		}
	}
	if obj := info.Defs[identX]; obj != nil {
		definfo.Package = obj.Pkg().Path()
		switch t := obj.Type().(type) {
		case *types.Signature:
			if t.Recv() == nil {
				definfo.Name = obj.Name()
				return definfo, nil
			} else {
				// Method or interface method.
				definfo.Container = dereferenceType(t.Recv().Type()).(*types.Named).Obj().Name()
				definfo.Name = identX.Name
				return definfo, nil
			}
		}

		if obj.Parent() == pkg.Scope() {
			// Top-level package def.
			return objectInfo(obj), nil
		}

		// Struct field.
		if _, ok := nodes[1].(*ast.Field); ok {
			if typ, ok := nodes[4].(*ast.TypeSpec); ok {
				definfo.Container = typ.Name.Name
				definfo.Name = obj.Name()
				return definfo, nil
			}
		}

		if pkg, name, ok := typeName(dereferenceType(obj.Type())); ok {
			definfo.Package = pkg
			definfo.Name = name
			return definfo, nil
		}

		return definfo, ErrNotFound
	}

	obj := info.Uses[identX]
	if obj == nil {
		return definfo, ErrNotFound
	}

	if obj, ok := obj.(*types.Var); ok && obj.IsField() {
		// Struct literal
		if lit, ok := nodes[2].(*ast.CompositeLit); ok {
			definfo.Package = obj.Pkg().Path()
			definfo.Name = obj.Id()
			if parent, ok := lit.Type.(*ast.SelectorExpr); ok {
				definfo.Container = fmt.Sprint(parent.Sel)
			} else if parent, ok := lit.Type.(*ast.Ident); ok {
				definfo.Container = fmt.Sprint(parent)
			}
			return definfo, nil
		}
	}

	if pkgName, ok := obj.(*types.PkgName); ok {
		definfo.Package = pkgName.Imported().Path()
		return definfo, nil
	} else if selX == nil {
		if pkg.Scope().Lookup(identX.Name) == obj {
			return objectInfo(obj), nil
		} else if types.Universe.Lookup(identX.Name) == obj {
			definfo.Name = obj.Name()
			definfo.Package = "builtin"
			return definfo, nil
		} else {
			t := dereferenceType(obj.Type())
			if pkg, name, ok := typeName(t); ok {
				definfo.Package = pkg
				definfo.Name = name
				return definfo, nil
			}
			return objectInfo(obj), nil
		}
	} else if sel, ok := info.Selections[selX]; ok {
		recv, ok := dereferenceType(deepRecvType(sel)).(*types.Named)
		if !ok || recv == nil || recv.Obj() == nil || recv.Obj().Pkg() == nil || recv.Obj().Pkg().Scope().Lookup(recv.Obj().Name()) != recv.Obj() {
			return definfo, ErrNotFound
		}

		field, _, _ := types.LookupFieldOrMethod(sel.Recv(), true, pkg, identX.Name)
		if field == nil {
			// field invoked, but object is selected
			t := dereferenceType(obj.Type())
			if pkg, name, ok := typeName(t); ok {
				definfo.Package = pkg
				definfo.Name = name
				return definfo, nil
			}
			return definfo, ErrNotFound
		}

		definfo.Package = fmt.Sprint(recv.Obj().Pkg().Path())
		definfo.Container = recv.Obj().Name()
		definfo.Name = identX.Name
		return definfo, nil
	} else {
		// Qualified reference (to another package's top-level
		// definition).
		if obj := info.Uses[selX.Sel]; obj != nil {
			return objectInfo(obj), nil
		} else {
			return definfo, ErrNotFound
		}
	}
	return definfo, nil
}

func buildPackage(importPath string) {
	t1 := time.Now()
	if importPath != "" {
		// Generates the .a files that the importer.Default() must
		// have to import other packages.
		if err := exec.Command("go", "build", "-i", importPath).Run(); err != nil {
			dlog.Println("go build:", err)
		}
		dlog.Println("go build took", time.Since(t1))
	}
}
