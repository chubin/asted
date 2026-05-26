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
