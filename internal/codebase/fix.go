package codebase

import (
	"go/ast"
	"path/filepath"

	"github.com/welibekov/asted/internal/object"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

// FixCallersAndImports inspects the workspace for usages of the moved object.
// It guards against self-importing when renaming objects within the same package.
func FixCallersAndImports(
	pkgs []*packages.Package,
	foundObj *object.FoundObject,
	dstPkgPath string,
	newName string,
) (map[*ast.File]*packages.Package, error) {
	modifiedFiles := make(map[*ast.File]*packages.Package)

	dstPkgName := filepath.Base(dstPkgPath)
	finalName := newName
	if finalName == "" {
		finalName = foundObj.Object.Name()
	}

	targetObj := foundObj.Object

	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}

		// Check if this package is the destination package itself
		isSamePackage := (pkg.PkgPath == dstPkgPath)

		for _, file := range pkg.Syntax {
			fileWasModified := false

			updatedFile := astutil.Apply(file, func(c *astutil.Cursor) bool {
				node := c.Node()
				if node == nil {
					return true
				}

				// Look for identifiers tied to our target object definition
				if ident, ok := node.(*ast.Ident); ok {
					if obj, exists := pkg.TypesInfo.Uses[ident]; exists && obj == targetObj {

						if isSamePackage {
							// SCENARIO A: In-place rename within the same package.
							// Simply mutate the identifier token name; do NOT qualify it.
							ident.Name = finalName
							fileWasModified = true
						} else {
							// SCENARIO B: External package reference adjustment.
							parent := c.Parent()
							if selExpr, isSel := parent.(*ast.SelectorExpr); isSel && selExpr.Sel == ident {
								// Fix existing external selectors (e.g., oldpkg.Ptr -> compute.NewPtr)
								selExpr.Sel.Name = finalName
								if id, ok := selExpr.X.(*ast.Ident); ok {
									id.Name = dstPkgName
								}
								fileWasModified = true
							} else if !isSel {
								// Upgrade external bare identifier (like dot-imports) to full selector
								replacement := &ast.SelectorExpr{
									X:   ast.NewIdent(dstPkgName),
									Sel: ast.NewIdent(finalName),
								}
								c.Replace(replacement)
								fileWasModified = true
							}
						}
					}
				}

				return true
			}, nil)

			if fileWasModified {
				astFile := updatedFile.(*ast.File)

				// CRITICAL GUARD: Only inject the import statement if the file
				// is OUTSIDE the target destination package.
				if !isSamePackage {
					astutil.AddImport(pkg.Fset, astFile, dstPkgPath)
				}

				modifiedFiles[astFile] = pkg
			}
		}
	}

	return modifiedFiles, nil
}
