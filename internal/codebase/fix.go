package codebase

import (
	"go/ast"
	"path/filepath"

	"github.com/welibekov/asted/internal/object"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

// FixCallersAndImports inspects the workspace for usages of the moved object.
// It uses contextual state tracking to guarantee method receivers are never incorrectly package-qualified.
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
			continue // Defensive guard against un-typechecked packages
		}

		isSamePackage := (pkg.PkgPath == dstPkgPath)

		for _, file := range pkg.Syntax {
			fileWasModified := false

			// Contextual State Flag replacing the old manual slice stack
			var inReceiver bool

			updatedFile := astutil.Apply(file, func(c *astutil.Cursor) bool {
				node := c.Node()
				if node == nil {
					return true
				}

				// 1. TRACK ENTRY: Detect if we are stepping into a method receiver list
				if fl, ok := node.(*ast.FieldList); ok {
					if fd, ok := c.Parent().(*ast.FuncDecl); ok && fd.Recv == fl {
						inReceiver = true
					}
				}

				// 2. IDENTIFIER EVALUATION
				if ident, ok := node.(*ast.Ident); ok {
					if obj, exists := pkg.TypesInfo.Uses[ident]; exists && obj == targetObj {

						if isSamePackage {
							// Scenario A: Internal package updates (In-place mutation)
							ident.Name = finalName
							fileWasModified = true
						} else {
							// Scenario B: External package reference adjustments
							if inReceiver {
								// Method receivers can never be package-qualified. Skip.
								return true
							}

							parent := c.Parent()
							if selExpr, isSel := parent.(*ast.SelectorExpr); isSel && selExpr.Sel == ident {
								// Already qualified expression (e.g., oldpkg.MyStruct -> newpkg.YourStruct)
								selExpr.Sel.Name = finalName
								if id, ok := selExpr.X.(*ast.Ident); ok {
									id.Name = dstPkgName
								}
								fileWasModified = true
							} else if !isSel {
								// Unqualified naked identifier -> Convert to SelectorExpr (e.g., MyStruct -> newpkg.YourStruct)
								newX := ast.NewIdent(dstPkgName)
								newSel := ast.NewIdent(finalName)

								// POSITION TRAP FIX: Transfer position tokens to protect code formatting
								newX.NamePos = ident.NamePos
								newSel.NamePos = ident.NamePos

								c.Replace(&ast.SelectorExpr{
									X:   newX,
									Sel: newSel,
								})
								fileWasModified = true
							}
						}
					}
				}

				return true
			}, func(c *astutil.Cursor) bool {
				// 3. TRACK EXIT: Turn off the flag as we walk back up past the receiver list
				if fl, ok := c.Node().(*ast.FieldList); ok {
					if fd, ok := c.Parent().(*ast.FuncDecl); ok && fd.Recv == fl {
						inReceiver = false
					}
				}
				return true
			})

			// 4. POST-WALK IMPORT ADJUSTMENTS
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
