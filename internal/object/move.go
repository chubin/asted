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

// MoveObject severs the node (and all its associated receiver methods if it's a type)
// along with its comments from the source file and grafts it into the destination.
func MoveObject(pkgs []*packages.Package, foundObj *FoundObject, dstPkgPath string, newName string) (*MoveResult, error) {
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
	if foundObj.Pkg.PkgPath == dstPkgPath {
		dstFile = foundObj.File
	}

	// 2. OMNIVOROUS COMMENT LOOKUP: Extract comments and identify the old identifier name
	var docComment *ast.CommentGroup
	var oldName string
	var parentTok token.Token = token.VAR // Default fallback

	switch n := foundObj.Node.(type) {
	case *ast.FuncDecl:
		docComment = n.Doc
		oldName = n.Name.Name
	case *ast.TypeSpec:
		docComment = n.Doc
		oldName = n.Name.Name
	case *ast.ValueSpec:
		docComment = n.Doc
		if len(n.Names) > 0 {
			oldName = n.Names[0].Name
		}
	case *ast.GenDecl:
		docComment = n.Doc
		parentTok = n.Tok // Capture token directly if the node is the GenDecl wrapper
	}

	if docComment == nil {
		if spec, ok := foundObj.Node.(ast.Spec); ok {
			for _, decl := range foundObj.File.Decls {
				if gDecl, ok := decl.(*ast.GenDecl); ok && isSpecParent(gDecl, spec) {
					parentTok = gDecl.Tok // CRITICAL FIX: Capture token.CONST or token.VAR safely!
					if gDecl.Doc != nil {
						docComment = gDecl.Doc
					}
					break
				}
			}
		}
	}

	// 2.5. COMPANION METHOD SCANNING: If the node is a TypeSpec, aggregate its methods
	var companionMethods []*ast.FuncDecl
	companionMethodsMap := make(map[ast.Decl]bool)

	var targetTypeName string
	if tSpec, ok := foundObj.Node.(*ast.TypeSpec); ok {
		targetTypeName = tSpec.Name.Name
	} else if gDecl, ok := foundObj.Node.(*ast.GenDecl); ok && gDecl.Tok == token.TYPE {
		if len(gDecl.Specs) > 0 {
			if tSpec, ok := gDecl.Specs[0].(*ast.TypeSpec); ok {
				targetTypeName = tSpec.Name.Name
			}
		}
	}

	// If we successfully resolved a target struct type name, sweep for its companion methods
	if targetTypeName != "" {
		for _, decl := range foundObj.File.Decls {
			if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl.Recv != nil {
				if getReceiverTypeName(funcDecl.Recv) == targetTypeName {
					companionMethods = append(companionMethods, funcDecl)
					companionMethodsMap[funcDecl] = true
				}
			}
		}
	}

	// 3. TEXT ISOLATION (PART A): Render the moved item cleanly within its native source environment
	var srcBuf bytes.Buffer
	if docComment != nil {
		for _, c := range docComment.List {
			srcBuf.WriteString(c.Text + "\n")
		}
	}

	// Create a safe, isolated clone of the declaration and apply the rename interceptor
	var declToPrint ast.Decl
	if spec, ok := foundObj.Node.(ast.Spec); ok {
		var cleanedSpec ast.Spec
		switch s := spec.(type) {
		case *ast.ValueSpec:
			clone := *s
			clone.Doc = nil
			clone.Comment = nil
			if newName != "" {
				newNames := make([]*ast.Ident, len(s.Names))
				for i, ident := range s.Names {
					idClone := *ident
					if ident.Name == oldName || len(s.Names) == 1 {
						idClone.Name = newName
					}
					newNames[i] = &idClone
				}
				clone.Names = newNames
			}
			cleanedSpec = &clone

		case *ast.TypeSpec:
			clone := *s
			clone.Doc = nil
			clone.Comment = nil
			if newName != "" {
				idClone := *s.Name
				idClone.Name = newName
				clone.Name = &idClone
			}
			cleanedSpec = &clone

		default:
			cleanedSpec = spec
		}

		declToPrint = &ast.GenDecl{
			Tok:   parentTok,
			Specs: []ast.Spec{cleanedSpec},
		}
	} else if decl, ok := foundObj.Node.(ast.Decl); ok {
		if fDecl, ok := decl.(*ast.FuncDecl); ok {
			clone := *fDecl
			clone.Doc = nil
			if newName != "" {
				idClone := *fDecl.Name
				idClone.Name = newName
				clone.Name = &idClone
			}
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
	srcBuf.WriteString("\n\n")

	// Append all gathered companion methods to the text isolation sequence stream
	for _, method := range companionMethods {
		if method.Doc != nil {
			for _, c := range method.Doc.List {
				srcBuf.WriteString(c.Text + "\n")
			}
		}
		methodClone := *method
		methodClone.Doc = nil // Prevent double comments from printing
		if err := format.Node(&srcBuf, foundObj.Pkg.Fset, &methodClone); err != nil {
			return nil, fmt.Errorf("failed to stringify companion method %s: %w", method.Name.Name, err)
		}
		srcBuf.WriteString("\n\n")
	}

	// 4. SEVER: Remove the declaration, all companion methods, and comments from the source file
	var updatedDecls []ast.Decl
	for _, decl := range foundObj.File.Decls {
		if decl == foundObj.Node || companionMethodsMap[decl] {
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

	// Track all comment groups that must be dropped from the file cache
	commentsToRemove := make(map[*ast.CommentGroup]bool)
	if docComment != nil {
		commentsToRemove[docComment] = true
	}
	for _, m := range companionMethods {
		if m.Doc != nil {
			commentsToRemove[m.Doc] = true
		}
	}

	var updatedComments []*ast.CommentGroup
	for _, cg := range foundObj.File.Comments {
		if !commentsToRemove[cg] {
			updatedComments = append(updatedComments, cg)
		}
	}
	foundObj.File.Comments = updatedComments

	// 5. TEXT ISOLATION (PART B): Render the untouched destination file into a clean string
	var dstBuf bytes.Buffer
	if err := format.Node(&dstBuf, dstPkg.Fset, dstFile); err != nil {
		return nil, fmt.Errorf("failed to stringify destination file: %w", err)
	}

	// 6. MERGE: Combine the pristine code bases together safely at the pure text layer
	combinedText := dstBuf.String() + "\n\n" + srcBuf.String()

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

// getReceiverTypeName extracts the raw type identifier string from a method receiver.
// It seamlessly peels back pointer stars, generic constraints, and index lists.
func getReceiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}

	t := recv.List[0].Type

	// Peel off pointer wrappers: *Type -> Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}

	// Peel off single generic type index identifiers: Type[T] -> Type
	if idx, ok := t.(*ast.IndexExpr); ok {
		t = idx.X
	}

	// Peel off multiple generic type index identifier lists: Type[T, K] -> Type
	if idxList, ok := t.(*ast.IndexListExpr); ok {
		t = idxList.X
	}

	if ident, ok := t.(*ast.Ident); ok {
		return ident.Name
	}

	return ""
}

// Internal standard layout parsing helpers
func isSpecParent(gDecl *ast.GenDecl, target ast.Node) bool {
	for _, spec := range gDecl.Specs {
		if spec == target {
			return true
		}
	}
	return false
}

func removeSpecFromGenDecl(gDecl *ast.GenDecl, target ast.Node) {
	var remaining []ast.Spec
	for _, spec := range gDecl.Specs {
		if spec != target {
			remaining = append(remaining, spec)
		}
	}
	gDecl.Specs = remaining
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
