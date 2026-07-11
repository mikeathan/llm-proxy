// check-complexity computes McCabe cyclomatic complexity for every function
// in the project and fails if any exceeds the threshold.  Uses only stdlib —
// no external dependencies.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

const defaultThreshold = 12

func main() {
	threshold := defaultThreshold

	root := "."
	dirs := []string{
		"backend/internal",
	}

	failed := false
	for _, dir := range dirs {
		abs := filepath.Join(root, dir)
		filepath.Walk(abs, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			f, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return nil
			}
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				c := complexity(fn.Body)
				if c > threshold {
					pos := fset.Position(fn.Pos())
					fmt.Printf("%s:%d: cyclomatic complexity %d exceeds limit %d\n",
						path, pos.Line, c, threshold)
					failed = true
				}
			}
			return nil
		})
	}

	if failed {
		os.Exit(1)
	}
}

// complexity returns the McCabe cyclomatic complexity for a block.
func complexity(block *ast.BlockStmt) int {
	c := 1
	ast.Inspect(block, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt:
			c++
		case *ast.ForStmt:
			c++
		case *ast.RangeStmt:
			c++
		case *ast.CaseClause:
			c++
		case *ast.CommClause:
			c++
		case *ast.BinaryExpr:
			be := n.(*ast.BinaryExpr)
			if be.Op == token.LAND || be.Op == token.LOR {
				c++
			}
		}
		return true
	})
	return c
}
