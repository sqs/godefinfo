package main

import (
	"encoding/json"
	"fmt"
	"go/types"
	"log"
	"os"
	"strings"
)

type defInfo struct {
	Name    string
	Package string

	// Container is the object that a method or field is invoked upon.
	Container string

	// IsGoRepoPath describes whether a package can be found in GOROOT,
	// eg fmt, net/http.
	IsGoRepoPath bool
}

func outputData(data ...interface{}) string {
	output := fmt.Sprintln(data...)
	return output
}

func (i defInfo) String() string {
	if i.Container == "" {
		return fmt.Sprintf("%s %s", i.Package, i.Name)
	}
	return fmt.Sprintf("%s %s %s", i.Package, i.Container, i.Name)
}

func stringToDefInfo(output string) defInfo {
	datas := strings.Split(strings.Trim(output, "\n"), " ")
	info := defInfo{}
	if len(datas) > 0 {
		info.Package = datas[0]
	}
	if len(datas) > 2 {
		info.Name = datas[2]
		info.Container = datas[1]
	} else if len(datas) > 1 {
		info.Name = datas[1]
	}
	return info
}

func printStructured(info defInfo) {
	info.IsGoRepoPath = isGoRepoPath(info.Package)
	bytes, err := json.MarshalIndent(info, "", "\t")
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(bytes)
}

func objectInfo(obj types.Object) defInfo {
	return defInfo{
		Name:    obj.Name(),
		Package: obj.Pkg().Path(),
	}
}
