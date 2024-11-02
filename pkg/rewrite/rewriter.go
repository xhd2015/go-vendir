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
	file, err := parser.ParseFile(fset, "", code, parser.ImportsOnly)
	if err != nil {
		return "", err
	}

	edit := goedit.New(fset, code)
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
