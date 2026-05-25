package codebase

import (
	"go/ast"
	"path/filepath"

	"github.com/welibekov/asted/internal/object"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

// FixCallersAndImports inspects the entire workspace for usages of the moved object.
// It updates their package qualifiers, applies the optional newName, and injects missing imports.
func FixCallersAndImports(
	pkgs []*packages.Package,
	foundObj *object.FoundObject,
	dstPkgPath string,
	newName string,
) (map[*ast.File]*packages.Package, error) {
	modifiedFiles := make(map[*ast.File]*packages.Package)

	// Determine the target name (fallback to original name if newName is empty)
	finalName := newName
	if finalName == "" {
		finalName = foundObj.Object.Name()
	}

	// Derive the clean short package name of the destination (e.g., "compute" from "internal/compute")
	dstPkgName := filepath.Base(dstPkgPath)

	// We use the type-checker's unique object pointer to find exact references across the workspace
	targetObj := foundObj.Object

	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}

		for _, file := range pkg.Syntax {
			fileWasModified := false

			// astutil.Apply allows us to cleanly mutate or replace AST nodes during a depth-first traversal
			updatedFile := astutil.Apply(file, func(c *astutil.Cursor) bool {
				node := c.Node()
				if node == nil {
					return true
				}

				// SCENARIO 1: Handle existing qualified references (e.g., oldpkg.OldName)
				if selExpr, ok := node.(*ast.SelectorExpr); ok {
					// Check if the selection identifier matches our target object
					if obj, exists := pkg.TypesInfo.Uses[selExpr.Sel]; exists && obj == targetObj {

						// 1. Rewrite the selector name to the new name
						selExpr.Sel.Name = finalName

						// 2. Rewrite the package qualifier prefix to the new package name
						if id, ok := selExpr.X.(*ast.Ident); ok {
							id.Name = dstPkgName
						}

						fileWasModified = true
						return false // Stop traversing down this specific sub-tree
					}
				}

				// SCENARIO 2: Handle bare internal references (e.g., OldName used inside the source package)
				if ident, ok := node.(*ast.Ident); ok {
					// Ensure we are matching the target object definition or usage
					if obj, exists := pkg.TypesInfo.Uses[ident]; exists && obj == targetObj {

						// Ensure this identifier isn't already the child of a SelectorExpr we handled above
						parent := c.Parent()
						if _, isSel := parent.(*ast.SelectorExpr); !isSel {

							// Transform the bare identifier into a full qualified selector expression
							replacement := &ast.SelectorExpr{
								X:   ast.NewIdent(dstPkgName),
								Sel: ast.NewIdent(finalName),
							}

							c.Replace(replacement)
							fileWasModified = true
						}
					}
				}

				return true
			}, nil)

			// If any references were updated in this file, manage package imports
			if fileWasModified {
				astFile := updatedFile.(*ast.File)

				// Add the new package import path to the top of the file
				astutil.AddImport(pkg.Fset, astFile, dstPkgPath)

				// If we are modifying files inside the original source package,
				// goimports will automatically clean up the old unused import statement later.

				modifiedFiles[astFile] = pkg
			}
		}
	}

	return modifiedFiles, nil
}
