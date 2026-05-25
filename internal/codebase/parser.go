package codebase

import (
	"fmt"
	"strings"
)

// RefactorSpec holds the completely sanitized, fully-qualified execution blueprints
type RefactorSpec struct {
	SourcePkgPath string
	SourceDecl    string
	DestPkgPath   string
	DestDecl      string // Will be identical to SourceDecl if no rename is requested
	NewName       string // Explicit new name for the object, empty string if no rename
}

// ParseRefactorSpec handles the layout translation from raw CLI input arguments.
// It resolves module naming automatically and falls back to matching declaration identities.
func ParseRefactorSpec(srcInput, dstInput, modulePath string) (*RefactorSpec, error) {
	// 1. Isolate paths and declaration names for both targets
	srcPkg, srcDecl := splitPkgAndDecl(srcInput)
	dstPkg, dstDecl := splitPkgAndDecl(dstInput)

	if srcDecl == "" {
		return nil, fmt.Errorf("source input %q must explicitly specify a declaration name (e.g., package.Name)", srcInput)
	}

	// Rule 3: If no destination declaration name is provided, assume it matches the source
	var newName string
	if dstDecl == "" {
		dstDecl = srcDecl
		newName = "" // No rename transformation occurring
	} else if dstDecl != srcDecl {
		newName = dstDecl // Explicit rename requested
	}

	// Rule 2: Automatically prepend the module path to local targets if missing
	srcPkg = ensureFullyQualified(srcPkg, modulePath)
	dstPkg = ensureFullyQualified(dstPkg, modulePath)

	return &RefactorSpec{
		SourcePkgPath: srcPkg,
		SourceDecl:    srcDecl,
		DestPkgPath:   dstPkg,
		DestDecl:      dstDecl,
		NewName:       newName,
	}, nil
}

// splitPkgAndDecl safely separates a package path from its target declaration token.
// It guards against package paths containing domain-level dots (e.g., github.com/foo/bar.Method)
func splitPkgAndDecl(input string) (string, string) {
	input = strings.TrimSuffix(input, ".")

	// Find the final forward slash to isolate the base package directory name
	lastSlash := strings.LastIndex(input, "/")

	base := input
	prefix := ""
	if lastSlash != -1 {
		prefix = input[:lastSlash+1]
		base = input[lastSlash+1:]
	}

	// Scan exclusively the base filename block for a declaration dot separator
	lastDot := strings.LastIndex(base, ".")
	if lastDot == -1 {
		return input, "" // No declaration block present
	}

	pkgPath := prefix + base[:lastDot]
	declName := base[lastDot+1:]
	return pkgPath, declName
}

// ensureFullyQualified binds un-prefixed local folders to the active module context.
func ensureFullyQualified(pkgPath, modulePath string) string {
	if modulePath == "" {
		return pkgPath
	}

	pkgPath = strings.Trim(pkgPath, "/")
	modulePath = strings.Trim(modulePath, "/")

	// If the path already has the module path or looks like an absolute external domain, keep it
	if strings.HasPrefix(pkgPath, modulePath) || strings.Contains(strings.Split(pkgPath, "/")[0], ".") {
		return pkgPath
	}

	// Automatically prepend the module prefix name to local sub-paths
	return modulePath + "/" + pkgPath
}
