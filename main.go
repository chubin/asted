package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/welibekov/asted/internal/codebase"
	"github.com/welibekov/asted/internal/object"
)

// Global commands
var (
	projectDir string // Global variable to store the flag's target string

	rootCmd = &cobra.Command{
		Use:   "asted",
		Short: "asted is a lightning-fast Go source code refactoring engine",
		Long:  `A highly specialized abstract syntax tree (AST) manipulation utility designed to move and rename declarations flawlessly across package boundaries.`,
	}

	mvCmd = &cobra.Command{
		Use:   "mv [source-declaration] [destination-package]",
		Short: "Move and optionally rename a declaration across packages",
		Long: `Cuts a specified declaration (function, variable, constant, type) out of its source package,
grafts it into the destination package, and automatically repairs all call sites and import blocks across the entire workspace.

Examples:
  asted mv internal/auth/util.Ptr internal/compute
  asted mv internal/auth/util.Ptr internal/compute.NewPtr`,
		// Enforce that exactly 2 positional arguments must be provided
		Args: cobra.ExactArgs(2),
		// RunE allows us to return standard errors gracefully back to Cobra's layout printer
		RunE: executeMove,
	}
)

func init() {
	// Register the --dir / -d flag persistently across all application subcommands.
	// Setting the default value to "." satisfies assuming the current working directory.
	rootCmd.PersistentFlags().StringVarP(&projectDir, "dir", "d", ".", "Path to the target project workspace directory")

	// Register subcommands under the master application root
	rootCmd.AddCommand(mvCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// executeMove handles the actual orchestration pipeline when 'asted mv' is invoked
func executeMove(cmd *cobra.Command, args []string) error {
	rawSrcInput := args[0] // e.g., "internal/auth/util.Ptr"
	rawDstInput := args[1] // e.g., "internal/compute.NewPtr"

	// 1. ATOMIC ENVIRONMENT SHIFT: Resolve and drop directly into the target directory context
	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path for directory %q: %w", projectDir, err)
	}

	if err := os.Chdir(absProjectDir); err != nil {
		return fmt.Errorf("failed to switch process context to target directory %s: %w", absProjectDir, err)
	}

	cmd.Printf("[+] Scanning workspace analysis trees inside: %s\n", absProjectDir)
	cmd.Printf("[+] Scanning workspace analysis trees...\n")

	// 2. Configure the workspace loader
	pkgs, err := codebase.LoadPackages(absProjectDir)
	if err != nil {
		log.Fatal(err)
	}
	if len(pkgs) == 0 {
		return fmt.Errorf("no packages found in the current working directory hierarchy")
	}

	// 3. Discover the active module path context dynamically
	var modulePath string
	for _, pkg := range pkgs {
		if pkg.Module != nil {
			modulePath = pkg.Module.Path // "github.com/welibekov/asted"
			break
		}
	}

	if modulePath == "" {
		fmt.Errorf("No module path is found")
	}

	// 4. Parse and sanitize the inputs using our smart specification parser
	spec, err := codebase.ParseRefactorSpec(rawSrcInput, rawDstInput, modulePath)
	if err != nil {
		return fmt.Errorf("input spec parsing failure: %w", err)
	}

	// =========================================================================
	// ROUTE INTERCEPTOR FOR INTERFACE & METHOD LEVEL TRANSFORMS
	// =========================================================================
	if spec.IsMethod {
		cmd.Printf("[+] Analyzing contract graph and resolving implementations for method %s...\n", spec.SourceMethod)

		modifiedFiles, err := codebase.FixMethodRenames(pkgs, spec)
		if err != nil {
			return fmt.Errorf("method refactoring execution failure: %w", err)
		}

		cmd.Printf("[+] Flushing optimized modifications securely to disk storage...\n")
		if err := codebase.SaveModifiedFiles(modifiedFiles); err != nil {
			return fmt.Errorf("failed to flush refactored files back to storage: %w", err)
		}

		cmd.Printf("[✓] Successfully refactored method %s into %s across all implementations!\n", spec.SourceMethod, spec.DestMethod)
		return nil
	}

	// =========================================================================
	// STANDARD PIPELINE FOR GLOBAL SYMBOLS (FUNCTIONS, VARIABLES, TYPES)
	// =========================================================================
	// 5. Locate the targeted symbol in the loaded workspace map
	foundObject, err := object.FindObject(pkgs, spec.SourcePkgPath, spec.SourceDecl)
	if err != nil {
		return fmt.Errorf("declaration lookup failure: %w", err)
	}

	cmd.Printf("[+] Rewriting call sites and managing imports across packages...\n")

	// 6. Global reference analysis and correction
	modifiedFiles, err := codebase.FixCallersAndImports(pkgs, foundObject, spec.DestPkgPath, spec.NewName)
	if err != nil {
		return fmt.Errorf("failed to modify cross-package callers: %w", err)
	}

	cmd.Printf("[+] Severing object from source and grafting into destination...\n")

	// 7. Memory AST extraction and re-parsing
	moveResult, err := object.MoveObject(pkgs, foundObject, spec.DestPkgPath, spec.NewName)
	if err != nil {
		return fmt.Errorf("failed to process syntax tree transfer: %w", err)
	}

	// Register the freshly parsed file tree to the save queue map
	modifiedFiles[moveResult.SourceFile] = foundObject.Pkg
	modifiedFiles[moveResult.DestinationFile] = moveResult.DestinationPkg

	cmd.Printf("[+] Flushing optimized modifications securely to disk storage...\n")

	// 8. Atomic directory shift and targeted import preservation
	if err := codebase.SaveModifiedFiles(modifiedFiles); err != nil {
		return fmt.Errorf("failed to flush refactored files back to storage: %w", err)
	}

	cmd.Printf("[✓] Successfully refactored %s into %s!\n", spec.SourceDecl, spec.DestDecl)
	return nil
}
