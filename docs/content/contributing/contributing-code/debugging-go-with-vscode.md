---
type: docs
title: "Debugging Radius with Visual Studio Code"
linkTitle: "Debug with VSCode"
description: "How to debug Project Radius with Visual Studio Code"
weight: 70
---

VSCode has good support for debugging VSCode, it's just a little unintuitive to set up.

VSCode's configuration for the debugger lives in `.vscode/launch.json` in the repo. We don't check this file in, and so everyone can maintain their own set of profiles/aliases.

## Debugging tests

Debugging unit tests is easy. Codelens on the test function will offer you a run or debug option, and you just click. No setup needed.

## Basic debugging

The most basic configuration for Go looks like this:

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Launch file",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${file}"
        },
    ]
}
```

This example will allow you to launch the debugger with whatever file open in the editor as the entry point.

To use this open a file like `cmd/cli/main.go` and press the green triangle button.

## Building aliases

While that option is flexible, it's not super convenient because it depends on the focus of the editor.

You can create your own aliases as well, and select them from the drop down.

```json
{
    "version": "0.2.0",
    "configurations": [
        ...

        {
            "name": "Launch RP",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}/cmd/rp/main.go",
            "env": [ ... environment variables here ...]
            "args": [ ... args here ...]
        },
    ]
}
```

## Debugging interactive commands

Interactive commands like `rad env init azure -i` don't work with Go debugging. This is a limitation of the underlying debugging infrastucture, it just doesn't support user input.

For this case you will need to create an alias and pass in the values you want. For this reason it's important that we create scriptable version of everything we do interactively. We can't easily debug the interactive version!

Here's an example:

```json
{
    "version": "0.2.0",
    "configurations": [
        ...

        {
            "name": "Launch rad env init azure",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}/cmd/cli/main.go",
            "args": [
                "env", "init", "azure", 
                "--subscription-id", "...", 
                "--resource-group", "...", 
                "--deployment-template", "${workspaceFolder}/deploy/rp-full.json"
            ]
        }
    ]
}
```
