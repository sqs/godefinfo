package main

import (
	"encoding/json"
	"fmt"
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

func outputData(data ...interface{}) {
	output := fmt.Sprintln(data...)
	if !*useJSON {
		fmt.Print(output)
		return
	}
	printStructured(output)
}

func printStructured(output string) {
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
	info.IsGoRepoPath = isGoRepoPath(info.Package)
	bytes, err := json.MarshalIndent(info, "", "\t")
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(bytes)
}
