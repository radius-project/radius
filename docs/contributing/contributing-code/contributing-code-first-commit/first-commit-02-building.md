# Your first commit: Building

## Building the code

If you have not already done so, clone the repository and navigate there in your command shell.

You can build the main outputs using `make`:

```sh
make build
```

You should see output similar to the following:

```txt
➜ make build
=> Building CLI from 'cmd/rad/main.go'
=> Built CLI in './dist/darwin_amd64/release/rad'
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 \
	go build \
	-gcflags "" \
	-ldflags "-s -w -X main.version=edge" \
	-o ./dist/darwin_amd64/release/rad \
	./cmd/rad/main.go;
```

Our makefile also has a built-in help command. Run `make` or `make help` to see the list of targets.

## Test it out

You should be able to run the binary that was just produced for the CLI. Copy the path from the previous output and run it at the command line.

```sh
./dist/darwin_amd64/release/rad
```

You should see the basic help text of the CLI. At the time of this writing it looks like:

```txt
Project Radius CLI

Usage:
  rad [command]

Available Commands:
  application Manage applications
  bicep       Manage bicep compiler
  component   Manage components
  deploy      Deploy a Radius application
  deployment  Manage deployments
  env         Manage environments
  expose      Expose local port
  help        Help about any command

Flags:
      --config string   config file (default is $HOME/.rad/config.yaml)
  -h, --help            help for rad

Use "rad [command] --help" for more information about a command.
```

If you got this far then you're able to build, and you should move to the next step.

## Next step:
- [Work on the CLI](first-commit-03-working-on-cli/index.md)
