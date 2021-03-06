// Copyright 2015 Francisco Souza. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// pray is a tool for finding unused public items (functions, methods, types,
// variables and constants) in packages, searching in a given context (the
// context being a list of packages).
//
// Example of use:
//
//     % pray -src github.com/tsuru/config github.com/tsuru/tsuru/app
//     /Users/f/src/github.com/tsuru/config/checker.go:10:6: Check is unused
//     /Users/f/src/github.com/tsuru/config/config.go:113:6: WriteConfigFile is unused
//     /Users/f/src/github.com/tsuru/config/config.go:229:6: GetUint is unused
//     /Users/f/src/github.com/tsuru/config/config.go:75:6: ReadAndWatchConfigFile is unused
//     /Users/f/src/github.com/tsuru/config/config.go:296:6: GetList is unused
//     /Users/f/src/github.com/tsuru/config/config.go:208:6: GetFloat is unused
//
// The flag -src specifies the source packages, and then users can provide the
// set of packages for searching. Users can also use the Go notation for
// expanding subpackages:
//
//     % pray -src github.com/tsuru/config github.com/tsuru/tsuru/...
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"golang.org/x/tools/oracle"
)

var srcPackage string

func init() {
	flag.StringVar(&srcPackage, "src", "", "Source package, to load types from")
	flag.Usage = func() {
		fmt.Println("Searchs for public types declared in the source package that are unused in the given import paths.")
		fmt.Printf("\nUsage of %s:\n\n", os.Args[0])
		fmt.Printf("%s [flags] <import_path1> [import_path2] ... [import_path3]\n\n", os.Args[0])
		fmt.Println("Flags:")
		flag.PrintDefaults()
	}
	flag.Parse()
}

type OraclePos struct {
	Identifier string
	Pos        string
	Filename   string
	Line       int
	Column     int
}

func getOraclePos(position token.Position) OraclePos {
	return OraclePos{
		Pos:      fmt.Sprintf("%s:#%d", position.Filename, position.Offset),
		Filename: position.Filename,
		Line:     position.Line,
		Column:   position.Column,
	}
}

func positionToStr(position token.Position) string {
	return fmt.Sprintf("%s:#%d", position.Filename, position.Offset)
}

func getFuncs(fileSet *token.FileSet, pkg *doc.Package) []OraclePos {
	result := make([]OraclePos, len(pkg.Funcs))
	for i, function := range pkg.Funcs {
		pos := function.Decl.Name.Pos()
		opos := getOraclePos(fileSet.Position(pos))
		opos.Identifier = function.Name
		result[i] = opos
	}
	return result
}

func getTypes(fileSet *token.FileSet, pkg *doc.Package) []OraclePos {
	result := make([]OraclePos, 0, len(pkg.Types))
	for _, tp := range pkg.Types {
		for _, spec := range tp.Decl.Specs {
			pos := spec.(*ast.TypeSpec).Name.Pos()
			opos := getOraclePos(fileSet.Position(pos))
			opos.Identifier = tp.Name
			result = append(result, opos)
		}
		for _, method := range tp.Methods {
			pos := method.Decl.Name.Pos()
			opos := getOraclePos(fileSet.Position(pos))
			opos.Identifier = method.Name
			result = append(result, opos)
		}
	}
	return result
}

func getConsts(fileSet *token.FileSet, pkg *doc.Package) []OraclePos {
	result := make([]OraclePos, 0, len(pkg.Consts))
	for _, constant := range pkg.Consts {
		for _, spec := range constant.Decl.Specs {
			for _, name := range spec.(*ast.ValueSpec).Names {
				position := fileSet.Position(name.Pos())
				opos := getOraclePos(position)
				opos.Identifier = name.Name
				result = append(result, opos)
			}
		}
	}
	return result
}

func getVars(fileSet *token.FileSet, pkg *doc.Package) []OraclePos {
	result := make([]OraclePos, 0, len(pkg.Vars))
	for _, variable := range pkg.Vars {
		for _, spec := range variable.Decl.Specs {
			for _, name := range spec.(*ast.ValueSpec).Names {
				position := fileSet.Position(name.Pos())
				opos := getOraclePos(position)
				opos.Identifier = name.Name
				result = append(result, opos)
			}
		}
	}
	return result
}

func loadDir(dir string, includeTests bool) ([]OraclePos, error) {
	filter := func(fi os.FileInfo) bool {
		return includeTests || !strings.Contains(fi.Name(), "_test")
	}
	fileSet := token.NewFileSet()
	pkgs, err := parser.ParseDir(fileSet, dir, filter, 0)
	if err != nil {
		return nil, err
	}
	var result []OraclePos
	for name, pkg := range pkgs {
		docPkg := doc.New(pkg, name, 0)
		result = append(result, getConsts(fileSet, docPkg)...)
		result = append(result, getVars(fileSet, docPkg)...)
		result = append(result, getTypes(fileSet, docPkg)...)
		result = append(result, getFuncs(fileSet, docPkg)...)
	}
	return result, nil
}

func runPackage(srcPackage string, dstPackages []string, output io.Writer) error {
	var status int
	dirName := strings.TrimRight(build.Default.GOPATH, "/") + "/src/" + srcPackage
	items, err := loadDir(dirName, false)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(items))
	finish := make(chan int)
	for _, item := range items {
		wg.Add(1)
		go func(item OraclePos) {
			defer wg.Done()
			result, err := oracle.Query(dstPackages, "referrers", item.Pos, nil, &build.Default, true)
			if err != nil {
				msg := err.Error()
				if msg == "no identifier here" {
					msg = fmt.Sprintf("%s:%d:%d - %s", item.Filename, item.Line, item.Column, msg)
				}
				errs <- fmt.Errorf(msg)
			}
			if len(result.Serial().Referrers.Refs) < 1 {
				errs <- fmt.Errorf("%s:%d:%d: %s is unused", item.Filename, item.Line, item.Column, item.Identifier)
			}
		}(item)
	}
	go func() {
		wg.Wait()
		close(finish)
	}()
	for {
		select {
		case err := <-errs:
			fmt.Fprintln(output, err)
			status = 1
		case <-finish:
			os.Exit(status)
		}
	}
}

func main() {
	flag.Parse()
	dstPackages := expandPackages(flag.Args())
	runPackage(srcPackage, dstPackages, os.Stderr)
}

func expandPackages(args []string) []string {
	var buf bytes.Buffer
	result := make([]string, 0, len(args))
	goArgs := append([]string{"list", "-e"}, args...)
	cmd := exec.Command("go", goArgs...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = &buf
	err := cmd.Run()
	if err != nil {
		return args
	}
	scanner := bufio.NewScanner(&buf)
	for scanner.Scan() {
		result = append(result, scanner.Text())
	}
	return result
}
