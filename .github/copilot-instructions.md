---
description: Comprehensive coding guidelines and instructions for GitHub Copilot
---

# GitHub Copilot Instructions

This file serves as the entry point for GitHub Copilot instructions in the Radius project.

These instructions define **HOW** Copilot should process user queries and **WHEN** to read specific guidance files.

## Overview

Copilot should follow the best practices and conventions defined in the specialized instruction files located in `.github/instructions/`. These files contain detailed guidelines for specific technologies, tools, and workflows used in this project.

## Temporary Planning Files

Copilot can create temporary planning files in the `.copilot-tracking/` folder at the root of the repository. This folder is included in `.gitignore` and will not be committed to the repository. Use this folder for:

- Tracking progress on multi-step tasks
- Creating temporary notes or outlines
- Storing intermediate planning documents
- Any other temporary files that help with task completion

Files in this folder can be safely deleted at any time.

## Instructions

The following instruction files are available:

- **[Bicep](instructions/bicep.instructions.md)** - Bicep conventions and guidelines
- **[Code Review](instructions/code-review.instructions.md)** - Guidelines for code review
- **[Docker](instructions/docker.instructions.md)** - Best practices for Docker images and containers
- **[GitHub Workflows](instructions/github-workflows.instructions.md)** - CI/CD best practices for GitHub Workflows
- **[Go (Golang)](instructions/golang.instructions.md)** - Guidelines for Go (Golang) development
- **[Make](instructions/make.instructions.md)** - Best practices for GNU Make Makefiles
- **[Shell Scripts](instructions/shell.instructions.md)** - Guidelines for Bash/Shell script development

## How to Use

When working on files that match the patterns defined in instruction files (e.g., `*.sh`, `.github/workflows/*.yml`), Copilot will automatically apply the relevant guidelines from the corresponding instruction file.

For general development queries, Copilot will use standard best practices and conventions appropriate for the technology or task at hand.
