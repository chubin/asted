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

## Where asted shines

***asted*** works best when you need to make big, repetitive changes across many
files — without turning the pull request into a nightmare to review.

**Table 1. Structural and Organizational Refactorings**  
*(the strongest area for **asted**)*

| Task                       | Example                                          | Why it fits **asted** well                                                                         |
|----------------------------|--------------------------------------------------|------------------------------------------------------------------------------------------------|
| Moving packages or modules | Move `internal/auth` → `pkg/auth`                | One **asted** script updates the folder structure and fixes all imports across the codebase        |
| Mass renaming              | Rename package `user` → `account` everywhere     | The script defines the renaming rule once and automatically updates all references and imports |
| Folder reorganization      | Move all domain models into a separate directory | A clear, repeatable rule for transforming file paths and package declarations                  |
| Module extraction          | Extract client logic into its own package        | The script precisely describes what code should be moved and where it should go                |

This category is where ***asted*** delivers the highest leverage: one small, reviewable script can safely reorganize large parts of the project.

Here are several other typical usage patterns where it really helps:

### 1. Moving and renaming things at scale

You want to reorganize packages, move files, or rename a module everywhere.
Instead of manually updating hundreds of imports and references, you describe the change once. **asted** turns it into a clear, repeatable script.

### 2. Changing APIs and updating all call sites

You add, remove, or rename a parameter. Or you change how a function should be called.
**asted** can generate a script that updates every place that uses it — consistently and safely.

### 3. Applying the same pattern everywhere

You decide to add logging, error handling, validation, or a new wrapper to many functions.
You don’t do it by hand. You create one small script that applies the pattern the same way in every file.


**In short:**

***asted*** shines when the change is mechanical and repetitive.
You stop editing code by hand at scale and start writing small, reliable instructions instead.

## Screenshots

### Moving a Struct and its Methods
![asted screenshot 01](assets/screenshots/screenshot_01.png)
![asted screenshot 02](assets/screenshots/screenshot_02.png)
![asted screenshot 03](assets/screenshots/screenshot_03.png)
![asted screenshot 03](assets/screenshots/screenshot_04.png)

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

