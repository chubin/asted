## asted

**asted** is a lightning-fast, compilation-safe CLI tool for code structural migration, package-boundary refactoring, and global declaration rewriting in Go.

## Introduction

Think of it as your old friend `sed`, but matching and transforming abstract syntax tree (AST) structures instead of flat file blocks. You can re-route, rename, and relocate functions, types, variables, or interfaces without ever fracturing your module graphs or breaking cross-package imports.

![asted demo](assets/asted.svg)

## Quick Start

You can unleash `asted` across your codebase in seconds. Let's look at a structural migration in action:

```bash
# General usage form:
asted mv [source-package.Declaration] [destination-package.[NewName]]

# Example: Move Ptr from util to compute, updating all workspace reference callers
asted mv internal/auth/util.Ptr internal/compute
```

## Installation

`asted` can be installed natively using standard Go toolchains:

```bash
go install github.com/welibekov/asted@latest
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

## Feature Highlight

At its core, asted is built to bypass blind text find-and-replace, leveraging Go's official Abstract Syntax Tree (AST) to execute structural code changes safely and instantly.

* Use the Paths You Already Know: Move any function, type, or constant by referencing the exact import paths you write every day (e.g., pkg/path.ObjectName). No complex configuration required.

* Smart Struct Moving (Method Bundling): In Go, structs and their methods are completely separate. If you move a struct, standard tools leave its methods behind. asted automatically finds, detaches, and migrates all associated methods—handling value, pointer, and complex generic receivers (like Struct[T, K]) flawlessly.

* Automatic Interface Tracking: Renaming an interface method usually breaks compilation across your project. asted analyzes your workspace type graph to find every struct that implicitly implements that interface, updating the interface contracts, concrete implementations, and call sites simultaneously.

