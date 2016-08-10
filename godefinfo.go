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

func FindDefInfo(info types.Info, nodes []ast.Node, pkg *types.Package) defInfo {
	definfo := defInfo{}

	// Handle import statements.
	//
	// TODO(sqs): fix this control flow so that the -debug.repetitions
	// flag causes this code path to repeat as well.
	if len(nodes) > 2 {
		if im, ok := nodes[1].(*ast.ImportSpec); ok {
			pkgPath, err := strconv.Unquote(im.Path.Value)
			if err != nil {
				log.Fatal(err)
			}
			definfo.Package = pkgPath
			return definfo
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
			log.Fatal("no identifier found")
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
				return definfo
			} else {
				// Method or interface method.
				// return outputData(obj.Pkg().Path(), dereferenceType(t.Recv().Type()).(*types.Named).Obj().Name(), identX.Name)
				definfo.Container = dereferenceType(t.Recv().Type()).(*types.Named).Obj().Name()
				definfo.Name = identX.Name
				return definfo
			}
		}

		if obj.Parent() == pkg.Scope() {
			// Top-level package def.
			// return outputData(objectString(obj))
			return objectInfo(obj)
		}

		// Struct field.
		if _, ok := nodes[1].(*ast.Field); ok {
			if typ, ok := nodes[4].(*ast.TypeSpec); ok {
				// return outputData(obj.Pkg().Path(), typ.Name.Name, obj.Name())
				definfo.Container = typ.Name.Name
				definfo.Name = obj.Name()
				return definfo
			}
		}

		if pkg, name, ok := typeName(dereferenceType(obj.Type())); ok {
			// return outputData(pkg, name)
			definfo.Package = pkg
			definfo.Name = name
			return definfo
		}

		log.Fatalf("unable to identify def (ident: %v, object: %v)", identX, obj)
	}

	obj := info.Uses[identX]
	if obj == nil {
		log.Fatalf("no type information for identifier %q at %d", identX.Name, *offset)
	}

	if obj, ok := obj.(*types.Var); ok && obj.IsField() {
		// Struct literal
		if lit, ok := nodes[2].(*ast.CompositeLit); ok {
			definfo.Package = obj.Pkg().Path()
			definfo.Name = obj.Id()
			if parent, ok := lit.Type.(*ast.SelectorExpr); ok {
				// return outputData(obj.Pkg().Path(), parent.Sel, obj.Id())
				definfo.Container = fmt.Sprint(parent.Sel)
			} else if parent, ok := lit.Type.(*ast.Ident); ok {
				// return outputData(obj.Pkg().Path(), parent, obj.Id())
				definfo.Container = fmt.Sprint(parent)
			}
			return definfo
		}
	}

	if pkgName, ok := obj.(*types.PkgName); ok {
		definfo.Package = pkgName.Imported().Path()
		return definfo
	} else if selX == nil {
		if pkg.Scope().Lookup(identX.Name) == obj {
			return objectInfo(obj)
		} else if types.Universe.Lookup(identX.Name) == obj {
			// return outputData("builtin", obj.Name())
			definfo.Name = obj.Name()
			definfo.Package = "builtin"
			return definfo
		} else {
			t := dereferenceType(obj.Type())
			if pkg, name, ok := typeName(t); ok {
				// return outputData(pkg, name)
				definfo.Package = pkg
				definfo.Name = name
				return definfo
			}
			log.Fatalf("not a package-level definition (ident: %v, object: %v) and unable to follow type (type: %v)", identX, obj, t)
		}
	} else if sel, ok := info.Selections[selX]; ok {
		recv, ok := dereferenceType(deepRecvType(sel)).(*types.Named)
		if !ok || recv == nil || recv.Obj() == nil || recv.Obj().Pkg() == nil || recv.Obj().Pkg().Scope().Lookup(recv.Obj().Name()) != recv.Obj() {
			log.Fatal("receiver is not a top-level named type")
		}

		field, _, _ := types.LookupFieldOrMethod(sel.Recv(), true, pkg, identX.Name)
		if field == nil {
			// field invoked, but object is selected
			t := dereferenceType(obj.Type())
			if pkg, name, ok := typeName(t); ok {
				// return outputData(pkg, name)
				definfo.Package = pkg
				definfo.Name = name
				return definfo
			}
			log.Fatal("method or field not found")
		}

		// return outputData(objectString(recv.Obj()), identX.Name)
		definfo.Package = fmt.Sprint(recv.Obj().Pkg().Path())
		definfo.Container = recv.Obj().Name()
		definfo.Name = identX.Name
		return definfo
	} else {
		// Qualified reference (to another package's top-level
		// definition).
		if obj := info.Uses[selX.Sel]; obj != nil {
			// return outputData(objectString(obj))
			return objectInfo(obj)
		} else {
			log.Fatal("no selector type")
		}
	}
	return definfo
}
