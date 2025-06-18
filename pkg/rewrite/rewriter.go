package rewrite

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"

	"github.com/xhd2015/xgo/support/edit/goedit"
)

type Rewriter struct {
	stdPkgMapping map[string]bool
	modPath       string
	pkgPath       string
}

func New(modPath string, pkgPath string) (*Rewriter, error) {
	if modPath == "" {
		return nil, fmt.Errorf("requires modPath")
	}
	goroot, err := getGoroot()
	if err != nil {
		return nil, err
	}
	stdPkgs, err := listStdPkgs(goroot)
	if err != nil {
		return nil, err
	}
	stdPkgMapping := make(map[string]bool, len(stdPkgs))
	for _, pkg := range stdPkgs {
		stdPkgMapping[pkg] = true
	}

	return &Rewriter{
		stdPkgMapping: stdPkgMapping,
		modPath:       modPath,
		pkgPath:       pkgPath,
	}, nil
}

func (c *Rewriter) RewritePath(path string) string {
	if path == "" {
		return ""
	}
	if path == "C" {
		return "C"
	}
	if !replaceablePath(path) {
		return path
	}
	if c.stdPkgMapping[path] {
		return path
	}
	if strings.HasPrefix(path, c.modPath) {
		// prefixed module, to resolve cyclic-dependency
		if len(path) == len(c.modPath) || path[len(c.modPath)] == '/' {
			return path
		}
	}
	return c.pkgPath + "/" + path
}

func (c *Rewriter) RewriteFile(file string) (string, error) {
	code, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return c.RewriteCode(string(code))
}

func (c *Rewriter) RewriteCode(code string) (string, error) {
	fset := token.NewFileSet()

	// parser.ImportsOnly|parser.ParseComments
	file, err := parser.ParseFile(fset, "", code, parser.ParseComments)
	if err != nil {
		return "", err
	}

	const GO_GENERATE = "//go:" + "generate "
	edit := goedit.New(fset, code)
	// all go generate will be removed
	for _, cmt := range file.Comments {
		for _, c := range cmt.List {
			if strings.HasPrefix(c.Text, GO_GENERATE) {
				edit.Replace(c.Pos(), c.Pos()+token.Pos(len(GO_GENERATE)), "// removed go:"+"generate ")
			}
		}
	}

	for _, imp := range file.Imports {
		pkg, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return "", err
		}
		newPkg := c.RewritePath(pkg)
		if newPkg != pkg {
			edit.Replace(imp.Path.Pos(), imp.Path.End(), strconv.Quote(newPkg))
		}
	}
	return edit.String(), nil
}
