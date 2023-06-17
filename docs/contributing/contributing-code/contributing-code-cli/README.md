# Build the Radius CLI

For a lot of your development tasks you will need to build `rad` from source instead of using a binary.

This is the best way to test changes, and to make sure you have the latest bits (other people's changes).

## Making code changes to the CLI

If you're working on the CLI then you will need to test out your changes. This section describes ways you might want to use a custom build of the CLI.

## Debugging in VS Code

We provide a debug target `Launch rad CLI` as part of our repo's VS Code configuration. Select `Launch rad CLI` from the drop-down press the Debug button to launch.

If you need to pass command line arguments into the CLI for testing then you can do this by editing `./vscode/launch.json`. Find the `Launch rad CLI` target and edit the `args` property.

> ⚠️ VS Code debugging does not support interactive user-input. If you need to debug a command that prompts for confirmation, you can bypass it by passing `--yes` at the command line. This tip does not apply to `rad init` which is always interactive.

## Installing a local build

You can use the Makefile to install a local build of Radius. By default this will install to `/usr/local/bin/rad`. You may also need to specify `sudo` depending on the destination and its permissions.

```sh
sudo make install
```

If you need to install to a different location, you can override via the `RAD_LOCATION` environment variable. Make sure the path you choose ends includes `rad` as the filename and is on your `PATH` so it can be executed easily.

```sh
RAD_LOCATION=/my/custom/location/rad sudo make install
```

## Creating a wrapper script

If you frequently need to work with a local build of `rad` you can create a script that will wrap `go run`.

> ⚠️ This tip only works when your current working directory is inside the repository. If your shell is outside the repository, then Go won't know how to resolve modules and the build will fail.

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
Radius CLI

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

### Using your $PATH variable to resolve the debug version of `rad` before the release version (MacOS/Linux)

You can update your `$PATH` environment variable to resolve to a custom build of `rad` by updating your shell profile. For ZSH users this is your `~/.zshrc` file:

```bash
# add debug rad bits to $PATH resolution
export PATH="$(pwd)/dist/linux_amd64/release:$PATH"
```

Make sure to set the appropriate path based on your OS and CPU type.

## Writing code for the CLI

### Error handling in the CLI

In the `rad` CLI we try to classify errors into *expected* and *unexpected* errors. 

- Expected: a known state we expect to encounter, that is not a bug in Radius. eg: application is not found.
- Unexpected: an unknown state that could be a bug in Radius. eg: a partial HTTP response was sent by the server.

*Expected* errors should be returned to the user as a normal error message. For these cases use the `clierrors.Message` and `clierrors.MessageWithCause` functions to create and return an error. When creating error messages for *expected* errors, write in complete sentences and end with a period. eg: `"The application could not be found."`

We want *unexpected* errors to be shown to the user with full troubleshooting information. For these cases, return the error as-is. If it makes sense for the scenario it's useful to wrap the error with additional context as shown in the example below.

**Example:**

```go
// Good: classify expected errors and return them as basic user-facing messages.
result, err := findApplication(id)
if errors.Is(err, NotFoundError{}) {
   return clierrors.Message("The application %q could not be found.", applicationName)
} else if err != nil {
  // optional: wrapping the error to add context.
  return fmt.Errorf("error retrieving application: %w", err)
}
```