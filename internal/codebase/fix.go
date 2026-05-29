package codebase

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"strings"

	"github.com/welibekov/asted/internal/object"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

// FixCallersAndImports inspects the workspace for usages of the moved object.
// It uses contextual state tracking to guarantee method receivers are never incorrectly package-qualified,
// automatically resolves package namespace collisions using smart overrides, and cleans up dead imports.
func FixCallersAndImports(
	pkgs []*packages.Package,
	foundObj *object.FoundObject,
	dstPkgPath string,
	newName string,
) (map[*ast.File]*packages.Package, error) {
	modifiedFiles := make(map[*ast.File]*packages.Package)

	srcPkgPath := foundObj.Pkg.PkgPath
	dstDefaultName := filepath.Base(dstPkgPath) // e.g., "types"
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

			// Step 1: Pre-flight check to ensure this file actually references our target object
			usesTargetObj := false
			for _, obj := range pkg.TypesInfo.Uses {
				if obj == targetObj {
					usesTargetObj = true
					break
				}
			}
			if !usesTargetObj {
				continue
			}

			// Step 2: Determine if an import namespace conflict will occur in this file
			var localPkgAlias string
			if !isSamePackage {
				collision, existingImportPath := inspectImportCollision(file, dstPkgPath, dstDefaultName)
				if collision && existingImportPath != dstPkgPath {
					// Namespace is occupied by a different package (e.g., auth/types vs compute/types).
					// Calculate a clean, deterministic prefix like "computetypes".
					localPkgAlias = generateSmartAlias(dstPkgPath)
				} else {
					// No namespace collision detected. Use standard base qualifier.
					localPkgAlias = dstDefaultName
				}
			}

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
								// Already qualified expression (e.g., oldpkg.MyStruct -> computetypes.YourStruct)
								selExpr.Sel.Name = finalName
								if id, ok := selExpr.X.(*ast.Ident); ok {
									id.Name = localPkgAlias
								}
								fileWasModified = true
							} else if !isSel {
								// Unqualified naked identifier -> Convert to SelectorExpr
								newX := ast.NewIdent(localPkgAlias)
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

			// 4. POST-WALK IMPORT ADJUSTMENTS WITH COLLISION SAFETY & GARBAGE COLLECTION
			if fileWasModified {
				astFile := updatedFile.(*ast.File)
				if !isSamePackage {
					// Inject the import with an alias override if a collision occurred
					if localPkgAlias != dstDefaultName {
						astutil.AddNamedImport(pkg.Fset, astFile, localPkgAlias, dstPkgPath)
					} else {
						astutil.AddImport(pkg.Fset, astFile, dstPkgPath)
					}

					// Garbage collect the old source package import if no remaining identifiers need it
					if !isOldImportStillNeeded(astFile, pkg.TypesInfo, targetObj, srcPkgPath) {
						astutil.DeleteImport(pkg.Fset, astFile, srcPkgPath)
					}
				}
				modifiedFiles[astFile] = pkg
			}
		}
	}

	return modifiedFiles, nil
}

// inspectImportCollision scans current file imports to identify package naming conflicts.
func inspectImportCollision(file *ast.File, newPkgPath, baseName string) (bool, string) {
	for _, imp := range file.Imports {
		pathValue := strings.Trim(imp.Path.Value, `"`)

		// If an existing explicit named alias matches our base name target
		if imp.Name != nil {
			if imp.Name.Name == baseName {
				return true, pathValue
			}
			continue
		}

		// If an un-aliased basic import target matches our base name path element
		if filepath.Base(pathValue) == baseName {
			return true, pathValue
		}
	}
	return false, ""
}

// generateSmartAlias builds a descriptive naming alias combining domain steps.
// (e.g., "internal/compute/types" -> "computetypes", "auth" -> "authpkg")
func generateSmartAlias(pkgPath string) string {
	// Clean up any trailing/leading slashes and split
	cleaned := strings.Trim(pkgPath, "/")
	if cleaned == "" {
		return "aliasedpkg"
	}

	parts := strings.Split(cleaned, "/")
	if len(parts) >= 2 {
		// e.g., internal/compute/types -> "compute" + "types" = "computetypes"
		return parts[len(parts)-2] + parts[len(parts)-1]
	}

	// Fallback for single-element paths (e.g., "auth" -> "authpkg")
	return parts[0] + "pkg"
}

// isOldImportStillNeeded verifies if any remaining expressions depend on the abandoned package statement.
func isOldImportStillNeeded(file *ast.File, info *types.Info, targetObj types.Object, srcPkgPath string) bool {
	stillNeeded := false

	ast.Inspect(file, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}

		obj, exists := info.Uses[ident]
		if !exists || obj == targetObj {
			return true
		}

		// If another element points to an active object belonging to our old package, preserve the import boundary
		if obj.Pkg() != nil && obj.Pkg().Path() == srcPkgPath {
			stillNeeded = true
			return false // Stop traversal early
		}
		return true
	})

	return stillNeeded
}
