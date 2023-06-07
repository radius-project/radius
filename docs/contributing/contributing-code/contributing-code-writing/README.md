# Contributing to Radius code

This guide includes background and tips for working on the Radius Go codebase.

## Learning Go

Go is a great language for newcomers! Due to its simple style and uncomplicated design, we find that new contributors can get *going* without a long learning process.

For learning Go, we recommend the following resources:

- [Tour of Go](https://go.dev/tour/welcome/1)
- [Effective Go](https://go.dev/doc/effective_go)
- [Offical tutorials](https://go.dev/doc/)

We're happy to accept pull-requests and give code review feedback aimed at newbies. If you have programmed in other languages before, we are confident you can pick up Go and start contributing easily.

## Getting productive

You'll want to run the following command often:

```sh
make build test lint
```

This will build, run unit tests, and run linters to point out any problems. It's a good idea to run this if you're about to make a `git commit`.

If you're looking for something, use the [code organization guide](./../contributing-code-organization/) to find what you're looking for.

## Coding style & linting

We enforce coding style through using [gofmt](https://pkg.go.dev/cmd/gofmt).

We stick to the usual philosophy of Go projects regarding styling, meaning that we prefer to avoid bikeshedding and debates about styling:

>  gofmt isn't anybody's preferred style, but it's adequate for everybody.

If you're using a modern editor with Go support, chances are it is already integrated with `gofmt` and this will mostly be automatic. If there's any question about how to style a piece of code, following the style of the surrounding code is a safe bet. 

---

We also *mostly* agree with [Google's Go Style Guide](https://google.github.io/styleguide/go/), but don't follow it strictly or enforce everything written there. If you're new to working on a Go project, this is a great read that will get you thinking critically about the small decisions you will make when writing Go code. You can find an appendix further down with some cases where we disagree with and don't follow their guidance.

### Documentation

One thing we do require is [godoc comments](https://tip.golang.org/doc/comment) on **exported** packages, types, variables, constants, and functions. We like this because it has two good effects:

- Encourages you to minimize the exported surface-area, thus simplifying the design.
- Requires you to document clearly the purpose code you expect other parts of the codebase to call.

Right now we don't have automated enforcement of this rule, so expect it to come up in code review if you forget.

### Linting

We run [golint-ci](https://github.com/golangci/golangci-lint) as part of the pull-request process for static analysis. We don't have many customizations and mostly rely on the defaults.

### CodeQL security analysis

We run [CodeQL](https://codeql.github.com/) as part of the pull-request process for security analysis. If the CodeQL analysis finds a security issue it will be reported as part of the PR checks. CodeQL is not currently required to pass for a PR to be merged, as it may be triggered by other alerts within the repo.

## Appendix: style guidance we don't follow

We mostly ignore the Google style guide's [guidance for testing](https://google.github.io/styleguide/go/decisions#assertion-libraries). In particular, we really like [assertion libraries](https://github.com/stretchr/testify) and use them in our tests.
