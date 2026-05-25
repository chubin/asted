package codebase

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

// FixMethodRenames tracks the implicit interface satisfaction graph across your codebase
// and applies the new name to all interface signatures, concrete implementations, and call sites.
func FixMethodRenames(pkgs []*packages.Package, spec *RefactorSpec) (map[*ast.File]*packages.Package, error) {
	methodsToRename := make(map[*types.Func]bool)
	var initialMethod *types.Func

	// Step 1: Find the target method definition inside the compilation scopes
	for _, pkg := range pkgs {
		if pkg.PkgPath == spec.SourcePkgPath {
			obj := pkg.Types.Scope().Lookup(spec.SourceTypeName)
			if tName, ok := obj.(*types.TypeName); ok {
				named, ok := tName.Type().(*types.Named)
				if !ok {
					continue
				}

				if itf, ok := named.Underlying().(*types.Interface); ok {
					for i := 0; i < itf.NumMethods(); i++ {
						m := itf.Method(i)
						if m.Name() == spec.SourceMethod {
							initialMethod = m
							break
						}
					}
				} else {
					for i := 0; i < named.NumMethods(); i++ {
						m := named.Method(i)
						if m.Name() == spec.SourceMethod {
							initialMethod = m
							break
						}
					}
				}
			}
		}
	}

	if initialMethod == nil {
		return nil, fmt.Errorf("could not find method %s on type %s in package %s", spec.SourceMethod, spec.SourceTypeName, spec.SourcePkgPath)
	}
	methodsToRename[initialMethod] = true

	// Step 2: Traverse the codebase to collect all structurally matching interfaces
	type InterfaceEntry struct {
		Itf    *types.Interface
		Method *types.Func
	}
	var targetInterfaces []InterfaceEntry

	for _, pkg := range pkgs {
		if pkg.Types == nil {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if tName, ok := obj.(*types.TypeName); ok {
				if named, ok := tName.Type().(*types.Named); ok {
					if itf, ok := named.Underlying().(*types.Interface); ok {
						for i := 0; i < itf.NumMethods(); i++ {
							m := itf.Method(i)
							if m.Name() == spec.SourceMethod && types.Identical(m.Type(), initialMethod.Type()) {
								targetInterfaces = append(targetInterfaces, InterfaceEntry{Itf: itf, Method: m})
								methodsToRename[m] = true
							}
						}
					}
				}
			}
		}
	}

	// Step 3: Identify all concrete types implementing those interfaces and flag their methods
	for _, pkg := range pkgs {
		if pkg.Types == nil {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if tName, ok := obj.(*types.TypeName); ok {
				if named, ok := tName.Type().(*types.Named); ok {
					if _, isItf := named.Underlying().(*types.Interface); !isItf {
						for _, itfEntry := range targetInterfaces {
							// Evaluate structural compatibility for both value and pointer receivers
							if types.Implements(named, itfEntry.Itf) || types.Implements(types.NewPointer(named), itfEntry.Itf) {
								for i := 0; i < named.NumMethods(); i++ {
									cm := named.Method(i)
									if cm.Name() == spec.SourceMethod {
										methodsToRename[cm] = true
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Step 4: Perform the AST transformations across definitions, declarations, and calls
	modifiedFiles := make(map[*ast.File]*packages.Package)

	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}

		for _, file := range pkg.Syntax {
			fileWasModified := false

			astutil.Apply(file, func(c *astutil.Cursor) bool {
				node := c.Node()
				if node == nil {
					return true
				}

				// Match 1: Concrete struct method implementations (func (s *Struct) Method())
				if funcDecl, ok := node.(*ast.FuncDecl); ok {
					if obj := pkg.TypesInfo.Defs[funcDecl.Name]; obj != nil {
						if f, ok := obj.(*types.Func); ok && methodsToRename[f] {
							funcDecl.Name.Name = spec.NewName
							fileWasModified = true
						}
					}
				}

				// Match 2: Interface method signature fields (type Intf interface { Method() })
				if field, ok := node.(*ast.Field); ok {
					for _, ident := range field.Names {
						if obj := pkg.TypesInfo.Defs[ident]; obj != nil {
							if f, ok := obj.(*types.Func); ok && methodsToRename[f] {
								ident.Name = spec.NewName
								fileWasModified = true
							}
						}
					}
				}

				// Match 3: Method invocation call sites (instance.Method())
				if selExpr, ok := node.(*ast.SelectorExpr); ok {
					if selection, exists := pkg.TypesInfo.Selections[selExpr]; exists {
						if f, ok := selection.Obj().(*types.Func); ok && methodsToRename[f] {
							selExpr.Sel.Name = spec.NewName
							fileWasModified = true
						}
					}
				}

				// Match 4: Direct method values or expression tracking references
				if ident, ok := node.(*ast.Ident); ok {
					if obj := pkg.TypesInfo.Uses[ident]; obj != nil {
						if f, ok := obj.(*types.Func); ok && methodsToRename[f] {
							ident.Name = spec.NewName
							fileWasModified = true
						}
					}
				}

				return true
			}, nil)

			if fileWasModified {
				modifiedFiles[file] = pkg
			}
		}
	}

	return modifiedFiles, nil
}
