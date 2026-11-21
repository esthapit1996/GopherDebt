package db

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// internalNilLists are `var x []T` declarations that never reach an API response, so nil is fine
// there. Anything else must start as an empty list (`x := []T{}`): encoding/json turns a nil slice
// into null, and a page that reads .length on it crashes (this happened with GoCrumbs notes).
var internalNilLists = map[string]bool{
	"db/users.go:settled":                 true, // account deletion
	"db/users.go:group_ids":               true, // account deletion
	"db/security.go:drops":                true, // RLS policy setup
	"db/security.go:revokes":              true, // RLS policy setup
	"db/settlements.go:creditors":         true, // debt simplification
	"db/settlements.go:debtors":           true, // debt simplification
	"handlers/expenses.go:splits":         true, // builds the split input of a request
	"handlers/groups.go:changes":          true, // text for the activity log
	"handlers/wish_lookup.go:h1s":         true, // page headings scraped for a product name
	"handlers/wish_lookup.go:price_hints": true, // price text scraped for the AI prompt
}

// TestListsAreNeverNil fails when a new nil list declaration appears in db or handlers, so an
// empty list can't reach the API as null again.
func TestListsAreNeverNil(t *testing.T) {
	for _, dir := range []string{".", "../handlers"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
			if err != nil {
				t.Fatal(err)
			}
			pkg := filepath.Base(filepath.Dir(path))
			if dir == "." {
				pkg = "db"
			}
			ast.Inspect(file, func(n ast.Node) bool {
				decl, ok := n.(*ast.GenDecl)
				if !ok || decl.Tok != token.VAR {
					return true
				}
				for _, spec := range decl.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok || len(vs.Values) > 0 {
						continue
					}
					if arr, ok := vs.Type.(*ast.ArrayType); !ok || arr.Len != nil {
						continue
					}
					for _, name := range vs.Names {
						key := pkg + "/" + filepath.Base(path) + ":" + name.Name
						if !internalNilLists[key] {
							t.Errorf("%s: `var %s []...` is nil when empty and becomes null in the API; use `%s := []T{}` (or add it to internalNilLists if it never reaches a response)", key, name.Name, name.Name)
						}
					}
				}
				return true
			})
		}
	}
}
