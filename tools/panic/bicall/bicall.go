// Package bicall locates calls of built-in functions in Go source.
package bicall

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
)

// See https://golang.org/ref/spec#Built-in_functions
var isBuiltin = map[string]bool{
	"append":  true,
	"cap":     true,
	"close":   true,
	"complex": true,
	"copy":    true,
	"delete":  true,
	"imag":    true,
	"len":     true,
	"make":    true,
	"new":     true,
	"panic":   true,
	"print":   true,
	"println": true,
	"real":    true,
	"recover": true,
}

// Call represents a call to a built-in function in a source program.
type Call struct {
	Name string         // the name of the built-in function
	Call *ast.CallExpr  // the call expression in the AST
	Site token.Position // the location of the call
	Path []ast.Node     // the AST path to the call
}

// Parse parses the contents of r as a Go source file, and calls f for each
// call expression targeting a built-in function that occurs in the resulting
// AST. Location information about each call is attributed to the specified
// filename.
//
// If f reports an error, traversal stops and that error is reported to the
// caller of Parse.
func Parse(r io.Reader, filename string, f func(Call) error) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, r, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parsing %q: %w", filename, err)
	}

	var path []ast.Node
	v := &visitor{
		visit: func(node ast.Node) error {
			if node == nil {
				path = path[:len(path)-1]
				return nil
			}
			path = append(path, node)
			if call, ok := node.(*ast.CallExpr); ok {
				id, ok := call.Fun.(*ast.Ident)
				if !ok || !isBuiltin[id.Name] {
					return nil
				}

				if err := f(Call{
					Name: id.Name,
					Call: call,
					Site: fset.Position(call.Pos()),
					Path: path,
				}); err != nil {
					return err
				}
			}
			return nil
		},
	}
	ast.Walk(v, file)
	return v.err
}

type visitor struct {
	err   error
	visit func(ast.Node) error
}

func (v *visitor) Visit(node ast.Node) ast.Visitor {
	if v.err == nil {
		v.err = v.visit(node)
	}
	if v.err != nil {
		return nil
	}
	return v
}
