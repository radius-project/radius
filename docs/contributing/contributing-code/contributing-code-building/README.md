# Building the code

Radius uses a Makefile to build the repository and automate most common repository tasks.

You can run `make` (no additional arguments) to see the list of targets and their descriptions.

## Building the repository

You can build the repository with `make build`. This will build all of the packages and executables. The first time you run `make build` it may take a few minutes because it will download and build dependencies. Subsequent builds will be faster because they can use cached output.

The following command will build, run unit tests, and run linters. This command is handy for verifying that your local changes are working correctly.

```sh
make build test lint
```

- See further information about tests [here](../contributing-code-tests/).
- See further information about linking [here](../contributing-code-writing/).

## Building containers

You can build containers for the Radius services using `make docker-build`, and push them with `make docker-push`.

By default we will assume your Docker registry is your OS username, and assume you want to build the `latest` tag. You can override this with environment variables.

- `DOCKER_REGISTRY` - set destination registry
- `DOCKER_TAG_VERSION` - set image tag

These commands assume you are already logged-in to the registry you are using. If you get errors related to authentication, double-check that you are logged-in.

Here's an example command that will push and push images to a specified registry:

```sh
DOCKER_REGISTRY=myregistry.ghcr.io make docker-push docker-build
```

If you work with Radius frequently, you may want to define a shell variable as part of your profile to set your registry.

## Generating code

If you are updating API schemas, or updating Go APIs that have mocks, you will need to update the generated code as part of your commit. It is our policy that we **check in** generated code. This minimizes the number of people that have to install the generators and wait for them to run. We validate as part of our PR process that the generated files are up to date. 

If you need to do this, first see the [prerequisites](../contributing-code-prerequisites/) for code generation.

Once you have installed the prerequisites, run the following command and then **commit** the changes as part of your commit.

```sh
make generate
```

This may take a few minutes as there are several steps. 

If you encounter problems please [open an issue](https://github.com/project-radius/radius/issues/new/choose) so we can help. We're trying to make these instructions as streamlined as possible for contributors, your help in identifying problems with the tools and instructions is very much appreciated!


## Troubleshooting and getting help

You might encounter error messages while running various `make` commands due to missing dependencies. Review the [prerequisites](./../contributing-code-prerequisites/) page for installation instructions.

If you get stuck working with the repository, please ask for help in our [forum](https://discordapp.com/channels/1113519723347456110/1115302284356767814). We're always interested in ways to improve the tooling, so please feel free to report problems and suggest improvements.

If you need to report an issue with the Makefile, we may ask you for a dump of the variables. You can see the state of all of the variables our Makefile defines with `make dump`. The output will be quite large so you might want to redirect this to a file.