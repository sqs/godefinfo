package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/ioutil"
	"log"
	"os"

	"golang.org/x/tools/go/ast/astutil"
)

var (
	readStdin = flag.Bool("i", false, "read file from stdin")
	offset    = flag.Int("o", -1, "file offset of identifier in stdin")
	debug     = flag.Bool("debug", false, "debug mode")
	filename  = flag.String("f", "", "Go source filename")
)

var (
	fset = token.NewFileSet()
	dlog *log.Logger
)

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

	if *debug {
		dlog = log.New(os.Stderr, "[debug] ", 0)
	} else {
		dlog = log.New(ioutil.Discard, "", 0)
	}
	log.SetFlags(0)

	var src []byte
	if *readStdin {
		src, _ = ioutil.ReadAll(os.Stdin)
	} else {
		b, err := ioutil.ReadFile(*filename)
		if err != nil {
			log.Fatal(err)
		}
		src = b
	}
	f, err := parser.ParseFile(fset, *filename, src, 0)
	if f == nil {
		log.Fatal(err)
	}

	conf := types.Config{
		Importer:                 importer.Default(),
		DisableUnusedImportCheck: true,
		Error: func(error) {},
	}
	info := types.Info{
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	pkg, err := conf.Check(f.Name.Name, fset, []*ast.File{f}, &info)
	if err != nil {
		dlog.Println(err)
	}

	pos := token.Pos(*offset) + 1 // 1-indexed (because 0 Pos is invalid)
	nodes, _ := astutil.PathEnclosingInterval(f, pos, pos)

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

	obj := info.Uses[identX]
	if pkgName, ok := obj.(*types.PkgName); ok {
		fmt.Println(pkgName.Imported().Path())
		return
	}

	if selX == nil {
		if pkg.Scope().Lookup(identX.Name) == obj {
			fmt.Println(objectString(obj))
		} else if types.Universe.Lookup(identX.Name) == obj {
			fmt.Println("builtin", obj.Name())
		} else {
			log.Fatal("not a package-level definition")
		}
	} else {
		sel, ok := info.Selections[selX]
		if !ok {
			// Qualified reference (to another package's top-level
			// definition).
			if obj := info.Uses[selX.Sel]; obj != nil {
				fmt.Println(objectString(obj))
				return
			}

			log.Fatal("no selector type")
		}

		recv, ok := dereferenceType(deepRecvType(sel)).(*types.Named)
		if !ok || recv == nil || recv.Obj() == nil || recv.Obj().Pkg() == nil || recv.Obj().Pkg().Scope().Lookup(recv.Obj().Name()) != recv.Obj() {
			log.Fatal("receiver is not a top-level named type")
		}

		obj, _, _ := types.LookupFieldOrMethod(sel.Recv(), true, pkg, identX.Name)
		if obj == nil {
			log.Fatal("method or field not found")
		}

		fmt.Println(objectString(recv.Obj()), identX.Name)
		return
	}
}

// deepRecvType gets the embedded struct's name that the method or
// field is actually defined on, not just the original/outer recv
// type.
func deepRecvType(sel *types.Selection) types.Type {
	var offset int
	offset = 1
	if sel.Kind() == types.MethodVal || sel.Kind() == types.MethodExpr {
		offset = 0
	}

	typ := sel.Recv()
	idx := sel.Index()
	for k, i := range idx[:len(idx)-offset] {
		final := k == len(idx)-offset-1
		typ = getMethod(typ, i, final, sel.Kind() != types.FieldVal).Type()
	}
	return typ
}

func dereferenceType(typ types.Type) types.Type {
	if typ, ok := typ.(*types.Pointer); ok {
		return typ.Elem()
	}
	return typ
}

func getMethod(typ types.Type, idx int, final bool, method bool) (obj types.Object) {
	switch obj := typ.(type) {
	case *types.Pointer:
		return getMethod(obj.Elem(), idx, final, method)

	case *types.Named:
		if final && method {
			switch obj2 := dereferenceType(obj.Underlying()).(type) {
			case *types.Struct:
				return obj.Method(idx).Type().(*types.Signature).Recv()
			case *types.Interface:
				recvObj := obj2.Method(idx).Type().(*types.Signature).Recv()
				if recvObj.Type() == obj.Underlying() {
					return obj.Obj()
				}
				return recvObj
			}
		}
		return getMethod(obj.Underlying(), idx, final, method)

	case *types.Struct:
		return obj.Field(idx)

	case *types.Interface:
		// Our index is among all methods, but we want to get the
		// interface that defines the method at our index.
		return obj.Method(idx).Type().(*types.Signature).Recv()
	}
	return nil
}

func objectString(obj types.Object) string {
	if obj.Pkg() != nil {
		return fmt.Sprintf("%s %s", obj.Pkg().Path(), obj.Name())
	}
	return obj.Name()
}
