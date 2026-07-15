# Your first commit: Building

This step of the walkthrough gets you a working build. For the full build reference — every `make` target, container builds, and code generation — see the authoritative [Building the code](../../contributing-code-building/README.md) guide.

## Fork the repository

If you have not already done so, [make a fork of the repository](../../contributing-code-forks/index.md) and clone it to your local machine.

## Build the code

From the root of the repository, build the main outputs with:

```sh
make build
```

The first build downloads and compiles dependencies, so it may take a few minutes; later builds are faster. When it finishes, the `rad` CLI binary is written under `./dist/<GOOS>_<GOARCH>/release/rad` (the exact path is printed in the build output, and depends on your OS and architecture).

Run `make` (or `make help`) at any time to see every target and its description.

## Test it out

Run the binary that was just built — copy the path printed in the build output (it depends on your OS and architecture):

```sh
./dist/<GOOS>_<GOARCH>/release/rad
```

You should see the `rad` CLI help text listing its top-level commands. If you got this far, your build works.

## Next step

- [Work on the CLI](../first-commit-03-working-on-cli/index.md)
