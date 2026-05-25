package codebase

import (
	"fmt"

	"golang.org/x/tools/go/packages"
)

// LoadPackages loads the full AST and Type information for all packages inside the specified codebase directory path.
func LoadPackages(baseDir string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		// Force the Go toolchain context to change directory (cd) to the target codebase path
		Dir: baseDir,
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedCompiledGoFiles |
			packages.NeedModule,
	}

	// Loading "./..." inside the baseDir context ensures every sub-package in that project is analyzed
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace at path %s: %w", baseDir, err)
	}

	// Check if there are any critical compile/parser errors in the loaded packages
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("package %s has compile errors: %v", pkg.PkgPath, pkg.Errors[0])
		}
	}

	return pkgs, nil
}
