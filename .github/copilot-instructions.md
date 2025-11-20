---
description: Comprehensive coding guidelines and instructions for GitHub Copilot
---

# GitHub Copilot Instructions

This file serves as the entry point for GitHub Copilot instructions in the Radius project.

These instructions define **HOW** Copilot should process user queries and **WHEN** to read specific guidance files.

## Overview

Copilot should follow the best practices and conventions defined in the specialized instruction files located in `.github/instructions/`. These files contain detailed guidelines for specific technologies, tools, and workflows used in this project.

## Instructions

The following instruction files are available:

- **[GitHub Workflows](instructions/github-workflows.instructions.md)** - CI/CD best practices for GitHub Workflows
- **[Shell Scripts](instructions/shell.instructions.md)** - Guidelines for Bash/Shell script development
- **[Go (Golang)](instructions/golang.instructions.md)** - Guidelines for Go (Golang) development

## How to Use

When working on files that match the patterns defined in instruction files (e.g., `*.sh`, `.github/workflows/*.yml`), Copilot will automatically apply the relevant guidelines from the corresponding instruction file.

For general development queries, Copilot will use standard best practices and conventions appropriate for the technology or task at hand.
