package rewrite

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/xhd2015/xgo/support/cmd"
)

func getGoroot() (string, error) {
	goroot, err := cmd.Output("go", "env", "GOROOT")
	if err != nil {
		return "", err
	}
	goroot = strings.TrimSpace(goroot)
	if goroot == "" {
		return "", fmt.Errorf("cannot get 'go env GOROOT'")
	}
	return goroot, nil
}

func listStdPkgs(goroot string) ([]string, error) {
	if goroot == "" {
		return nil, fmt.Errorf("requires GOROOT")
	}
	res, err := cmd.Dir(filepath.Join(goroot, "src")).Output("go", "list", "./...")
	if err != nil {
		return nil, err
	}
	list := strings.Split(res, "\n")
	j := 0
	for i := 0; i < len(list); i++ {
		e := strings.TrimSpace(list[i])
		if e != "" {
			list[j] = e
			j++
		}
	}
	return list[:j], nil
}

// valid and non-relative path
func replaceablePath(path string) bool {
	if path == "" {
		return false
	}
	switch path[0] {
	case '/':
		// invalid import
		return false
	case '.':
		if len(path) == 1 {
			// single dot: .
			return false
		}
		switch path[1] {
		case '/':
			// relative import: ./
			return false
		case '.':
			if len(path) == 2 {
				// two dots: ..
				return false
			}
			if path[2] == '/' {
				// relative import: ../
				return false
			}
		}
	}
	return true
}
