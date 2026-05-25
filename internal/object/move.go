package object

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"

	"golang.org/x/tools/go/packages"
)

// MoveResult holds references to the modified files so they can be saved or analyzed later.
type MoveResult struct {
	SourceFile      *ast.File         // The modified original file (item removed)
	DestinationFile *ast.File         // The modified target file (item added)
	DestinationPkg  *packages.Package // Context of the destination package
}

// MoveObject severs the node and its comments from the source file and grafts it into the destination.
func MoveObject(pkgs []*packages.Package, foundObj *FoundObject, dstPkgPath string) (*MoveResult, error) {
	// 1. Locate the destination package in the loaded workspace
	var dstPkg *packages.Package
	for _, pkg := range pkgs {
		if pkg.PkgPath == dstPkgPath {
			dstPkg = pkg
			break
		}
	}
	if dstPkg == nil {
		return nil, fmt.Errorf("destination package %s not found in workspace", dstPkgPath)
	}

	dstFile := findOrCreateDstFile(dstPkg)

	// 2. Identify and isolate the documentation comments attached to the node
	var docComment *ast.CommentGroup
	if fDecl, ok := foundObj.Node.(*ast.FuncDecl); ok {
		docComment = fDecl.Doc
	} else if gDecl, ok := foundObj.Node.(*ast.GenDecl); ok {
		docComment = gDecl.Doc
	}

	// 3. TEXT ISOLATION (PART A): Render the moved item cleanly within its native source environment
	var srcBuf bytes.Buffer
	if docComment != nil {
		for _, c := range docComment.List {
			srcBuf.WriteString(c.Text + "\n")
		}
	}

	// Create a temporary copy of the declaration to clear inner doc pointers for printing
	var declToPrint ast.Decl
	if spec, ok := foundObj.Node.(ast.Spec); ok {
		declToPrint = &ast.GenDecl{
			Tok:   getSpecToken(spec),
			Specs: []ast.Spec{spec},
		}
	} else if decl, ok := foundObj.Node.(ast.Decl); ok {
		// Deep-copy look-alikes to prevent mutating the tree before stringification
		if fDecl, ok := decl.(*ast.FuncDecl); ok {
			clone := *fDecl
			clone.Doc = nil
			declToPrint = &clone
		} else if gDecl, ok := decl.(*ast.GenDecl); ok {
			clone := *gDecl
			clone.Doc = nil
			declToPrint = &clone
		} else {
			declToPrint = decl
		}
	}

	if err := format.Node(&srcBuf, foundObj.Pkg.Fset, declToPrint); err != nil {
		return nil, fmt.Errorf("failed to stringify source node: %w", err)
	}

	// 4. SEVER: Remove the declaration and comments completely from the source file
	var updatedDecls []ast.Decl
	for _, decl := range foundObj.File.Decls {
		if decl == foundObj.Node {
			continue
		}
		if gDecl, ok := decl.(*ast.GenDecl); ok && isSpecParent(gDecl, foundObj.Node) {
			removeSpecFromGenDecl(gDecl, foundObj.Node)
			if len(gDecl.Specs) == 0 {
				continue
			}
		}
		updatedDecls = append(updatedDecls, decl)
	}
	foundObj.File.Decls = updatedDecls

	if docComment != nil {
		var updatedComments []*ast.CommentGroup
		for _, cg := range foundObj.File.Comments {
			if cg != docComment {
				updatedComments = append(updatedComments, cg)
			}
		}
		foundObj.File.Comments = updatedComments
	}

	// 5. TEXT ISOLATION (PART B): Render the untouched destination file into a clean string
	var dstBuf bytes.Buffer
	if err := format.Node(&dstBuf, dstPkg.Fset, dstFile); err != nil {
		return nil, fmt.Errorf("failed to stringify destination file: %w", err)
	}

	// 6. MERGE: Combine the pristine code bases together safely at the pure text layer
	combinedText := dstBuf.String() + "\n\n" + srcBuf.String()

	// Capture target file path for parsing metadata
	dstFilePath := dstPkg.Fset.Position(dstFile.Pos()).Filename
	if dstFilePath == "" {
		dstFilePath = "grafted_file.go"
	}

	// 7. RE-PARSE: Parse the text stream into a fresh, perfectly indexed destination syntax tree
	newDstFile, err := parser.ParseFile(dstPkg.Fset, dstFilePath, combinedText, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to re-parse combined code stream: %w", err)
	}

	// 8. SYNCHRONIZE: Overwrite the package workspace syntax pointer with our new master tree
	for i, f := range dstPkg.Syntax {
		if f == dstFile {
			dstPkg.Syntax[i] = newDstFile
			break
		}
	}

	return &MoveResult{
		SourceFile:      foundObj.File,
		DestinationFile: newDstFile,
		DestinationPkg:  dstPkg,
	}, nil
}

// Helper: Finds "packagename.go" or initializes an empty AST file if the package is empty
func findOrCreateDstFile(dstPkg *packages.Package) *ast.File {
	for _, file := range dstPkg.Syntax {
		if filepath.Base(dstPkg.Fset.Position(file.Pos()).Filename) == dstPkg.Name+".go" {
			return file
		}
	}
	if len(dstPkg.Syntax) > 0 {
		return dstPkg.Syntax[0]
	}

	// Fallback: Create a synthetic file layout if no files exist
	newFile := &ast.File{
		Name: ast.NewIdent(dstPkg.Name),
	}
	dstPkg.Syntax = append(dstPkg.Syntax, newFile)
	return newFile
}

// Helper: Checks if a GenDecl block (like var (...)) contains our target Spec node
func isSpecParent(gDecl *ast.GenDecl, target ast.Node) bool {
	spec, ok := target.(ast.Spec)
	if !ok {
		return false
	}
	for _, s := range gDecl.Specs {
		if s == spec {
			return true
		}
	}
	return false
}

// Helper: Modifies a multi-declaration block to extract a single spec
func removeSpecFromGenDecl(gDecl *ast.GenDecl, target ast.Node) {
	var updatedSpecs []ast.Spec
	for _, s := range gDecl.Specs {
		if s != target {
			updatedSpecs = append(updatedSpecs, s)
		}
	}
	gDecl.Specs = updatedSpecs
}

// Helper: Determines the token type (VAR, CONST, TYPE) for a standalone Spec wrapper
func getSpecToken(spec ast.Spec) token.Token {
	switch spec.(type) {
	case *ast.TypeSpec:
		return token.TYPE
	case *ast.ValueSpec:
		return token.VAR
	default:
		return token.VAR
	}
}
