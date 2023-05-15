# Build the Radius CLI

For a lot of your development tasks you will need to build `rad` from source instead of using a binary.

This is the best way to test changes, and to make sure you have the latest bits (other people's changes).

## Creating an wrapper script (MacOS/Linux)

Create the following script and place it on your path with a name like `dev-rad`.

```sh
#!/bin/sh
set -eu
go run ~/github.com/project-radius/radius/cmd/rad/main.go $@
```

Replace `~/github.com/project-radius/radius` with the path to your repository root.

Run `chmod +x dev-rad` to mark it executable

Now use it as-if it were `rad`

```txt
➜ dev-rad env
Project Radius CLI

Usage:
  rad [command]

Available Commands:
  deploy      Deploy a Radius application
  env         Manage environments
  expose      Expose local port
  help        Help about any command

Flags:
      --config string   config file (default is $HOME/.rad/config.yaml)
  -h, --help            help for rad

Use "rad [command] --help" for more information about a command.
```

## Using your $PATH variable to resolve the debug version of `rad` before the release version (MacOS/Linux)

Update your $PATH variable to resolve the debug bits folder in your /zsrc/.bashrc file

```bash
# add debug rad bits to $PATH resolution
export PATH="$(pwd)/dist/linux_amd64/release:$PATH"
```

## Enabling VSCode debugging using codelenses

VSCode will start a child process when you execute a `'run test'/'debug test'` codelens (see image for example), this process may not resolve `rad` to the debug bits folder. To allow VSCode to correctly resolve the debug bits,  in your `settings.json` file specify:

```json

 "go.testEnvVars": {
        "RAD_PATH": "${workspaceFolder}/dist/linux_amd64/release"
    },

```

![](https://user-images.githubusercontent.com/9611108/174677971-673e220b-7447-4330-b25b-9a6e0d01b351.png)
