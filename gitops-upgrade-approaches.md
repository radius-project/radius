# Radius Upgrades - GitOps Support Approaches

## Option 1: Kubernetes Controller

Watch Flux HelmRelease and ArgoCD Application objects to manage upgrades.

### Pros
- Responds immediately to GitOps events
- Supports multiple GitOps tools
- Separate from core Radius services

### Cons
- Must maintain compatibility with Flux and ArgoCD APIs
- Additional deployment to manage
- More complex debugging
- Extra resource overhead

## Option 2: New Dedicated CLI

Create a new CLI specifically for preflight checks and upgrade coordination, run as init container.

### Pros
- Built for the specific use case
- Clean separation from user tooling
- Standard init container pattern
- Independent testing

### Cons
- New tool to build and maintain
- May duplicate existing code
- Another binary to distribute
- Need to design new interfaces

## Option 3: Extend rad CLI

Add preflight and upgrade modes to the existing rad CLI, run as init container.

### Pros
- Reuses existing CLI code and infrastructure
- Single tool for users and operations
- Existing build pipeline
- Known codebase

### Cons
- Mixes user and operational concerns
- May confuse end users
- Testing both modes together is complex
- Risk of breaking user functionality
