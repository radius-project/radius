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

To use this open a file like `cmd/rad/main.go` and press the green triangle button.

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

At the time of writing of this document, interactive commands like `rad env init azure -i` don't work with vanilla Go debugging in vs-code. However there is (a somewhat ugly) but useful way of still being able to debug.

For this case you will need to create an alias and a task. However, for convenience and usability it's very important that we create scriptable version of everything we do interactively since we can't easily debug the interactive version!

Here's an example alias (`.vscode/launch.json`):

```json
{
    "version": "0.2.0",
    "configurations": [
        ...

        {
            "name": "Debug: rad init azure (interactive mode)",
            "preLaunchTask": "start dlv-dap debugger",
            "type": "go",
            "request": "launch",
            "mode": "debug",
             "port": 2345,
            "host": "127.0.0.1",
            "program": "${workspaceFolder}/cmd/rad/main.go",
            "args": [
                "env", "init", "azure", "-i"
            ]
        }
    ]
}
```
With the corresponding task definition (`.vscode/tasks.json`)
PS: Make sure you have `$GOPATH` set in your shell environment.

```json
{
    "version": "2.0.0",
    "tasks": [
        ...

        {
            "label": "start dlv-dap debugger",
            "type": "shell",
            "command": "${env:GOPATH}/bin/dlv-dap dap --headless --listen=:2345 --log --api-version=2",
            "problemMatcher": ["$go"],
            "group": {
                "kind": "build",
                "isDefault": true
            },  
            "isBackground": true,     
        }
    ]
}
```