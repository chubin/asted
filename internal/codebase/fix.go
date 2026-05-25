package codebase

import (
	"go/ast"

	"github.com/welibekov/asted/internal/object"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

// FixCallersAndImports updates all references to the moved object and updates package imports.
// It returns a map of all files that were modified during this process.
func FixCallersAndImports(
	pkgs []*packages.Package,
	foundObj *object.FoundObject,
	dstPkgPath string,
	dstPkgName string,
) (map[*ast.File]*packages.Package, error) {

	modifiedFiles := make(map[*ast.File]*packages.Package)
	targetObj := foundObj.Object

	for _, pkg := range pkgs {
		// FIX 1: Shifted to an index loop so we can overwrite package syntax pointers
		for idx, file := range pkg.Syntax {
			fileWasModified := false

			updatedFile := astutil.Apply(file, func(c *astutil.Cursor) bool {
				node := c.Node()
				if node == nil {
					return true
				}

				switch n := node.(type) {
				// Scenario A: Caller is in an external package (e.g., pkgA.MyFunction)
				case *ast.SelectorExpr:
					if obj := pkg.TypesInfo.Uses[n.Sel]; obj == targetObj {
						if pkg.PkgPath == dstPkgPath {
							c.Replace(n.Sel)
						} else {
							n.X = ast.NewIdent(dstPkgName)
						}
						fileWasModified = true
					}

				// Scenario B: Caller is in the original package (e.g., direct call to MyFunction)
				case *ast.Ident:
					if obj := pkg.TypesInfo.Uses[n]; obj == targetObj {

						// =================================================================
						// >>> CRITICAL FIX: CHILD-SELECTOR GUARD <<<
						if _, isParentSelector := c.Parent().(*ast.SelectorExpr); isParentSelector {
							return true
						}
						// =================================================================

						// FIX 2: Upgraded from a simple FuncDecl check to a Comprehensive Definition Guard.
						// This ensures functions, types, variables, and constants are all protected.
						isDefinition := false
						switch parent := c.Parent().(type) {
						case *ast.FuncDecl:
							if parent.Name == n {
								isDefinition = true
							}
						case *ast.TypeSpec:
							if parent.Name == n {
								isDefinition = true
							}
						case *ast.ValueSpec:
							for _, nameIdent := range parent.Names {
								if nameIdent == n {
									isDefinition = true
									break
								}
							}
						}

						// If it is an actual usage/caller (not a definition), apply the prefix
						if !isDefinition {
							if pkg.PkgPath != dstPkgPath {
								newSelector := &ast.SelectorExpr{
									X:   ast.NewIdent(dstPkgName),
									Sel: n,
								}
								c.Replace(newSelector)
								fileWasModified = true
							}
						}
					}
				}
				return true
			}, nil)

			if fileWasModified {
				astFile := updatedFile.(*ast.File)

				// FIX 3: Keep the workspace syntax tree perfectly synchronized in memory
				pkg.Syntax[idx] = astFile

				if pkg.PkgPath != dstPkgPath {
					astutil.AddImport(pkg.Fset, astFile, dstPkgPath)
				}

				astutil.DeleteImport(pkg.Fset, astFile, foundObj.Pkg.PkgPath)
				modifiedFiles[astFile] = pkg
			}
		}
	}

	return modifiedFiles, nil
}
