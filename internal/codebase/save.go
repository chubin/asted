package codebase

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/imports"
)

// SaveModifiedFiles formats and flushes only the mutated files, using an atomic
// os.Chdir shift to ensure imports.Process resolves targeted paths flawlessly.
func SaveModifiedFiles(modifiedFiles map[*ast.File]*packages.Package) error {
	pathToLatestFile := make(map[string]*ast.File)
	fileToPackage := make(map[*ast.File]*packages.Package)
	var sampleAbsPath string

	// Step 1: Deduplicate pointers and map to clean absolute paths
	for file, pkg := range modifiedFiles {
		filePath := pkg.Fset.Position(file.Pos()).Filename

		if filePath == "" || filepath.Base(filePath) == "grafted_file.go" || !filepath.IsAbs(filePath) {
			var pkgDir string

			if len(pkg.GoFiles) > 0 {
				// Strategy A: Derive directory from existing package files
				pkgDir = filepath.Dir(pkg.GoFiles[0])
			} else if pkg.Module != nil {
				// Strategy B: Fallback for empty/new packages using module coordinates
				// e.g., converts "github.com/org/repo/internal/compute" -> "internal/compute"
				relSubPath := strings.TrimPrefix(pkg.PkgPath, pkg.Module.Path)
				relSubPath = strings.TrimPrefix(relSubPath, "/")
				pkgDir = filepath.Join(pkg.Module.Dir, relSubPath)
			}

			if pkgDir != "" {
				baseName := filepath.Base(filePath)
				if baseName == "" || baseName == "." || baseName == "grafted_file.go" {
					baseName = file.Name.Name + ".go"
				}
				filePath = filepath.Join(pkgDir, baseName)
			}
		}

		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return fmt.Errorf("failed to resolve absolute path for %s: %w", filePath, err)
		}

		// CRITICAL SAFETY: Ensure the physical directory path exists on disk before attempting to write.
		// This guarantees that if a new or custom file path was chosen, the directories are initialized on the fly.
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory structure for %s: %w", absPath, err)
		}

		if sampleAbsPath == "" {
			sampleAbsPath = absPath
		}

		fileToPackage[file] = pkg

		isCurrentSyntaxTruth := false
		for _, syntaxFile := range pkg.Syntax {
			if syntaxFile == file {
				isCurrentSyntaxTruth = true
				break
			}
		}

		if _, exists := pathToLatestFile[absPath]; !exists || isCurrentSyntaxTruth {
			pathToLatestFile[absPath] = file
		}
	}

	if sampleAbsPath == "" {
		return nil // Nothing to save
	}

	// Step 2: Dynamically deduce the target project root containing the go.mod file
	rootDir := filepath.Dir(sampleAbsPath)
	for {
		if _, err := os.Stat(filepath.Join(rootDir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(rootDir)
		if parent == rootDir {
			rootDir = filepath.Dir(sampleAbsPath)
			break
		}
		rootDir = parent
	}

	// Step 3: Shift host execution directory into the module home base
	originalCwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}
	if err := os.Chdir(rootDir); err != nil {
		return fmt.Errorf("failed to chdir to project root %s: %w", rootDir, err)
	}
	defer func() {
		_ = os.Chdir(originalCwd) // Always restore home context
	}()

	// PASS 1: Flush accurate AST structures directly to disk using standard formatting.
	for absPath, file := range pathToLatestFile {
		pkg := fileToPackage[file]

		var buf bytes.Buffer
		if err := format.Node(&buf, pkg.Fset, file); err != nil {
			return fmt.Errorf("failed to raw-format node for file %s: %w", absPath, err)
		}

		if err := os.WriteFile(absPath, buf.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write raw file %s to disk: %w", absPath, err)
		}
	}

	// PASS 2: Run imports.Process ONLY on the exact files we changed.
	for absPath := range pathToLatestFile {
		content, err := os.ReadFile(absPath)
		if err != nil {
			return fmt.Errorf("failed to read back file %s for import processing: %w", absPath, err)
		}

		finalBytes, err := imports.Process(absPath, content, nil)
		if err != nil {
			return fmt.Errorf("imports.Process failed on %s: %w", absPath, err)
		}

		if err := os.WriteFile(absPath, finalBytes, 0644); err != nil {
			return fmt.Errorf("failed to write finalized file %s to disk: %w", absPath, err)
		}
	}

	return nil
}
