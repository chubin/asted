package codebase

import (
	"fmt"
	"strings"
)

// RefactorSpec holds the completely sanitized, fully-qualified execution blueprints
type RefactorSpec struct {
	SourcePkgPath  string
	SourceTypeName string
	SourceMethod   string
	DestPkgPath    string
	DestTypeName   string
	DestMethod     string
	NewName        string
	IsMethod       bool
	SourceDecl     string // Fallback field for standard declarations
	DestDecl       string // Fallback field for standard declarations
}

// ParseRefactorSpec handles the layout translation from raw CLI input arguments.
func ParseRefactorSpec(srcInput, dstInput, modulePath string) (*RefactorSpec, error) {
	srcPkg, srcType, srcMethod := splitPkgTypeMethod(srcInput)
	dstPkg, dstType, dstMethod := splitPkgTypeMethod(dstInput)

	srcPkg = ensureFullyQualified(srcPkg, modulePath)
	dstPkg = ensureFullyQualified(dstPkg, modulePath)

	// Detect if we are processing a method-level operation
	if srcType != "" && srcMethod != "" {
		if dstMethod == "" {
			dstMethod = srcMethod
		}
		return &RefactorSpec{
			SourcePkgPath:  srcPkg,
			SourceTypeName: srcType,
			SourceMethod:   srcMethod,
			DestPkgPath:    dstPkg,
			DestTypeName:   dstType,
			DestMethod:     dstMethod,
			NewName:        dstMethod,
			IsMethod:       true,
		}, nil
	}

	// Fallback to standard declaration parsing rules
	if srcType == "" {
		return nil, fmt.Errorf("source input %q must explicitly specify a declaration name", srcInput)
	}

	dstDecl := dstType
	if dstDecl == "" {
		dstDecl = srcType
	}

	var newName string
	if dstDecl != srcType {
		newName = dstDecl
	}

	return &RefactorSpec{
		SourcePkgPath: srcPkg,
		SourceDecl:    srcType,
		DestPkgPath:   dstPkg,
		DestDecl:      dstDecl,
		NewName:       newName,
		IsMethod:      false,
	}, nil
}

// splitPkgTypeMethod safely separates package paths from type boundaries and method tokens.
// It explicitly guards against domain-level dots in path strings (e.g., github.com/user/repo)
func splitPkgTypeMethod(input string) (string, string, string) {
	input = strings.TrimSuffix(input, ".")
	lastSlash := strings.LastIndex(input, "/")

	base := input
	prefix := ""
	if lastSlash != -1 {
		prefix = input[:lastSlash+1]
		base = input[lastSlash+1:]
	}

	dots := strings.Count(base, ".")
	if dots >= 2 {
		// Method Format identified: pkg.TypeName.MethodName
		firstDot := strings.Index(base, ".")
		lastDot := strings.LastIndex(base, ".")

		pkgName := base[:firstDot]
		typeName := base[firstDot+1 : lastDot]
		methodName := base[lastDot+1:]

		return prefix + pkgName, typeName, methodName
	} else if dots == 1 {
		// Standard Format identified: pkg.DeclarationName
		lastDot := strings.LastIndex(base, ".")
		return prefix + base[:lastDot], base[lastDot+1:], ""
	}

	return input, "", ""
}

func ensureFullyQualified(pkgPath, modulePath string) string {
	if modulePath == "" {
		return pkgPath
	}
	pkgPath = strings.Trim(pkgPath, "/")
	modulePath = strings.Trim(modulePath, "/")
	if strings.HasPrefix(pkgPath, modulePath) || strings.Contains(strings.Split(pkgPath, "/")[0], ".") {
		return pkgPath
	}
	return modulePath + "/" + pkgPath
}
