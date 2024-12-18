package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/xhd2015/xgo/support/fileutil"
	"github.com/xhd2015/xgo/support/pattern"

	"github.com/xhd2015/go-vendir/pkg/rewrite"
	"github.com/xhd2015/xgo/support/cmd"
	"github.com/xhd2015/xgo/support/filecopy"
	"github.com/xhd2015/xgo/support/goinfo"
)

const help = `
vendir helps to create third party vendor dependency without
introducing changes to go.mod.

Usage: vendir <cmd> [OPTIONS] <ARGUMENTS...>

Commands:
  create <dir> <target_vendor_dir>
    create vendoring directory.

  rewrite-file <file> <target_vendor_dir>
    check rewrite content of a file, this command does not modify anything.

  rewrite-path <path> <target_vendor_dir>
    check rewrite import path, this command does not modify anything.

  help
    show help message.

Arguments:
  <dir> must be a dir containing a go.mod and a vendor dir.

Options:
  -v, --verbose         show verbose information
      --update          run 'go mod tidy' and 'go mod vendor' in <dir> prior to create vendor
  -f, --force           force remove <target_vendor_dir> and make a new one
      --incldue FILE    include only a set of files,FILE can be directory or pattern
	  --exclude FILE    extende a set of files

Example:
  $ vendir create ./some-pkg/internal/src ./some-pkg/internal/third_party_vendir

See https://github.com/xhd2015/go-vendir for documentation.
`

// usage:
//
//	go run ./script/vendir create ./script/vendir/testdata/src ./script/vendir/testdata/third_party_vendir
func main() {
	err := handle(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func handle(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("requires cmd")
	}
	cmd := args[0]
	args = args[1:]
	switch cmd {
	case "create":
		return createVendor(args)
	case "rewrite-file":
		return rewriteFile(args)
	case "rewrite-path":
		return rewritePath(args)
	case "help", "--help", "-h":
		fmt.Println(strings.TrimPrefix(help, "\n"))
		return nil
	default:
		return fmt.Errorf("unrecognized cmd: %s, try 'vendir help'", cmd)
	}
}

func createVendor(args []string) error {
	var remainArgs []string
	var verbose bool
	var update bool
	var force bool
	var exclude []string
	var include []string
	n := len(args)
	for i := 0; i < n; i++ {
		if args[i] == "--update" {
			update = true
			continue
		}
		if args[i] == "-f" || args[i] == "--force" {
			force = true
			continue
		}
		if args[i] == "--include" {
			if i+1 >= n {
				return fmt.Errorf("%s requires arg", args[i])
			}
			include = append(include, args[i+1])
			i++
			continue
		}
		if args[i] == "--exclude" {
			if i+1 >= n {
				return fmt.Errorf("%s requires arg", args[i])
			}
			exclude = append(exclude, args[i+1])
			i++
			continue
		}
		if args[i] == "-v" || args[i] == "--verbose" {
			verbose = true
			continue
		}
		if args[i] == "--" {
			remainArgs = append(remainArgs, args[i+1:]...)
			break
		}
		if strings.HasPrefix(args[i], "-") {
			return fmt.Errorf("unrecognized flag: %v", args[i])
		}
		remainArgs = append(remainArgs, args[i])
	}
	if len(remainArgs) < 2 {
		return fmt.Errorf("usage: vendir create <dir> <target_vendor_dir>")
	}

	dir := remainArgs[0]
	targetVendorDir := remainArgs[1]

	// check dir
	goMod := filepath.Join(dir, "go.mod")
	vendorDir := filepath.Join(dir, "vendor")
	_, err := os.Stat(goMod)
	if err != nil {
		return err
	}
	_, err = os.Stat(vendorDir)
	if err != nil {
		return err
	}

	if update {
		err := cmd.Dir(dir).Run("go", "mod", "tidy")
		if err != nil {
			return err
		}
		err = cmd.Dir(dir).Run("go", "mod", "vendor")
		if err != nil {
			return err
		}
	}

	if force {
		err := os.RemoveAll(targetVendorDir)
		if err != nil {
			return err
		}
	} else {
		_, targetStatErr := os.Stat(targetVendorDir)
		if !os.IsNotExist(targetStatErr) {
			return fmt.Errorf("%s already exists, remove it before create", targetVendorDir)
		}
	}

	err = os.MkdirAll(targetVendorDir, 0755)
	if err != nil {
		return err
	}

	modPath, pkgPath, err := goinfo.ResolveModulePkgPath(targetVendorDir)
	if err != nil {
		return err
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "modPath: %s, pkgPath(prefix): %s\n", modPath, pkgPath)
	}
	modFsPath := modPathToFsPath(modPath)
	rw, err := rewrite.New(modPath, pkgPath)
	if err != nil {
		return err
	}

	if update {
		// remove original unecessary dep
		err = os.RemoveAll(filepath.Join(vendorDir, modFsPath))
		if err != nil {
			return err
		}
	}

	err = filecopy.Copy(vendorDir, targetVendorDir)
	if err != nil {
		return err
	}

	// unnecessary mod(provided by current module itself)
	err = os.RemoveAll(filepath.Join(targetVendorDir, modFsPath))
	if err != nil {
		return err
	}

	filter := NewFileFilter(include, exclude)
	// traverse all go files, and rewrite
	return fileutil.WalkRelative(targetVendorDir, func(path string, relPath string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if !filter.MatchFile(relPath) {
			return nil
		}
		newCode, err := rw.RewriteFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		return os.WriteFile(path, []byte(newCode), 0755)
	})
}

func modPathToFsPath(modPath string) string {
	if filepath.Separator == '/' {
		return modPath
	}
	return strings.ReplaceAll(modPath, "/", string(filepath.Separator))
}

func rewriteFile(args []string) error {
	var remainArgs []string
	n := len(args)
	for i := 0; i < n; i++ {
		if args[i] == "--help" {
			fmt.Println(strings.TrimSpace(help))
			return nil
		}
		if args[i] == "--" {
			remainArgs = append(remainArgs, args[i+1:]...)
			break
		}
		if strings.HasPrefix(args[i], "-") {
			return fmt.Errorf("unrecognized flag: %v", args[i])
		}
		remainArgs = append(remainArgs, args[i])
	}
	if len(remainArgs) < 2 {
		return fmt.Errorf("usage: vendir <file> <target_vendor_dir>")
	}

	file := remainArgs[0]
	targetDir := remainArgs[1]

	stat, err := os.Stat(file)
	if err != nil {
		return err
	}

	if stat.IsDir() {
		return fmt.Errorf("%s is not a file", file)
	}

	modPath, pkgPath, err := goinfo.ResolveModulePkgPath(targetDir)
	if err != nil {
		return err
	}
	rw, err := rewrite.New(modPath, pkgPath)
	if err != nil {
		return err
	}
	// rewrite file
	code, err := rw.RewriteFile(file)
	if err != nil {
		return err
	}
	fmt.Println(code)

	return nil
}

func rewritePath(args []string) error {
	remainArgs := args
	if len(remainArgs) < 2 {
		return fmt.Errorf("usage: vendir rewrite-path <path> <target_vendor_dir>")
	}

	path := remainArgs[0]
	targetDir := remainArgs[1]

	modPath, pkgPath, err := goinfo.ResolveModulePkgPath(targetDir)
	if err != nil {
		return err
	}
	rw, err := rewrite.New(modPath, pkgPath)
	if err != nil {
		return err
	}

	fmt.Println(rw.RewritePath(path))
	return nil
}

// package filter
// copied from github.com/xhd2015/lines-annotation/path/filter

// FileFilter
type FileFilter struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`

	includePatterns pattern.Patterns
	excludePatterns pattern.Patterns
}

func NewFileFilter(include []string, exclude []string) *FileFilter {
	return &FileFilter{
		Include:         include,
		Exclude:         exclude,
		includePatterns: CompilePatterns(include),
		excludePatterns: CompilePatterns(exclude),
	}
}

func CompilePatterns(patterns []string) pattern.Patterns {
	list := make([]*pattern.Pattern, 0, len(patterns))
	for _, p := range patterns {
		ptn := pattern.CompilePattern(p)
		list = append(list, ptn)
	}
	return list
}

// MatchFile checks whether patterns of this filter
// match given *file*.
// NOTE: the target `file` must be a file, not a
// directory
func (c *FileFilter) MatchFile(file string) bool {
	hasInclude := len(c.Include) > 0
	if hasInclude {
		if !c.includePatterns.MatchAnyPrefix(file) {
			return false
		}
	}
	hasExclude := len(c.Exclude) > 0
	if hasExclude {
		if c.excludePatterns.MatchAnyPrefix(file) {
			return false
		}
	}
	return true
}
