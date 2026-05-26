# asted

> **asted** is a lightning-fast, compilation-safe CLI tool for code structural migration, package-boundary refactoring, and global declaration rewriting in Go.

Think of it as your old friend `mv`, but matching and transforming abstract syntax tree (AST) structures instead of flat file blocks. You can re-route, rename, and relocate functions, types, variables, or interfaces without ever fracturing your module graphs or breaking cross-package imports.

---

## Quick Start

You can unleash `asted` across your codebase in seconds. Let's look at a structural migration in action:

```bash
# General usage form:
asted mv [source-package.Declaration] [destination-package.[NewName]]

# Example: Move Ptr from util to compute, updating all workspace reference callers
asted mv internal/auth/util.Ptr internal/compute
```

## Feature Highlight

asted's core is an engine engineered to bypass traditional string-replacement and unstable memory syntax-tree manipulation barriers.

**Isomorphic Declaration Moving**: Move code by referencing the paths you already write every day (e.g., `pkg/path.ObjectName`).

**Companion Method Bundling**: Go structurally decouples structs and methods. asted automatically scans, severs, and migrates all companion methods matching value, pointer, or complex generic receivers (like `Struct[T, K]`) alongside the parent type declaration.

**Implicit Satisfaction Analysis**: Powered by `go/types`, the engine sweeps the workspace to resolve hidden interface bounds, safely applying method renames to abstract contracts and structural structs at the same time.

**Paradox & Loop Protection**: Detects local in-package renames to safely omit package qualifiers and intercept destructive circular dependency loops.

**Token Duality Retention**: Navigates the AST `ValueSpec` duality natively. Constants remain `const` and variables remain `var` throughout the extraction sequence.

**Performant & Cache-Safe**: Utilizes process-level directory shifting (`os.Chdir`) to safely run alongside Go's native compilation caching engines without tree corruption.

## Installation

`asted` can be installed natively using standard Go toolchains:

```bash
go install [github.com/welibekov/asted@latest](https://github.com/welibekov/asted@latest)
```

## Command Line Arguments

```bash
Usage:
  asted mv [source-declaration] [destination-target] [flags]

Examples:
  asted mv internal/auth/util.Ptr internal/compute
  asted mv internal/auth/util.Ptr internal/compute.NewPtr
  asted mv internal/types.User.GetName internal/types.User.GetFullName --dir ./app

Flags:
  -d, --dir string   Path to the target project workspace directory (default ".")
  -h, --help         help for mv
```

