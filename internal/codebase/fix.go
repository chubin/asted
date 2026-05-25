package codebase

import (
	"go/ast"
	"path/filepath"

	"github.com/welibekov/asted/internal/object"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

// FixCallersAndImports inspects the workspace for usages of the moved object.
// It tracks node ancestors to guarantee method receivers are never incorrectly package-qualified.
func FixCallersAndImports(pkgs []*packages.Package, foundObj *object.FoundObject, dstPkgPath string, newName string) (map[*ast.File]*packages.Package, error) {
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

		isSamePackage := (pkg.PkgPath == dstPkgPath)

		for _, file := range pkg.Syntax {
			fileWasModified := false

			// Maintain an ordered stack of active parent nodes during traversal
			var ancestors []ast.Node

			updatedFile := astutil.Apply(file, func(c *astutil.Cursor) bool {
				node := c.Node()
				if node == nil {
					return true
				}
				ancestors = append(ancestors, node)

				if ident, ok := node.(*ast.Ident); ok {
					if obj, exists := pkg.TypesInfo.Uses[ident]; exists && obj == targetObj {

						// CRITICAL GUARD: Check if this identifier lives inside a method receiver definition
						isReceiverType := false
						for i := len(ancestors) - 1; i >= 0; i-- {
							if fl, ok := ancestors[i].(*ast.FieldList); ok {
								if i > 0 {
									// If the parent of this field list is a FuncDecl and matches its Recv field
									if fd, ok := ancestors[i-1].(*ast.FuncDecl); ok && fd.Recv == fl {
										isReceiverType = true
										break
									}
								}
							}
						}

						if isSamePackage {
							// Scenario A: Internal package updates
							ident.Name = finalName
							fileWasModified = true
						} else {
							// Scenario B: External package reference adjustments
							if isReceiverType {
								// Method receivers can never be package-qualified.
								// Skip modifications here so MoveObject can cleanly bundle them.
								return true
							}

							parent := c.Parent()
							if selExpr, isSel := parent.(*ast.SelectorExpr); isSel && selExpr.Sel == ident {
								selExpr.Sel.Name = finalName
								if id, ok := selExpr.X.(*ast.Ident); ok {
									id.Name = dstPkgName
								}
								fileWasModified = true
							} else if !isSel {
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
			}, func(c *astutil.Cursor) bool {
				// Clean up the ancestor tracking stack as we walk back up the syntax tree
				if c.Node() != nil {
					ancestors = ancestors[:len(ancestors)-1]
				}
				return true
			})

			if fileWasModified {
				astFile := updatedFile.(*ast.File)
				if !isSamePackage {
					astutil.AddImport(pkg.Fset, astFile, dstPkgPath)
				}
				modifiedFiles[astFile] = pkg
			}
		}
	}

	return modifiedFiles, nil
}
