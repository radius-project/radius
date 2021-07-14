# Project Radius documentation

This directory contains the files to generate the https://radapp.dev site. Please go there to consume Project Radius docs. This document will describe how to build Project Radius docs locally.

## Pre-requisites

- [Hugo extended version](https://gohugo.io/getting-started/installing)
- [Node.js](https://nodejs.org/en/)

## Environment setup

1. Ensure pre-requisites are installed
2. Clone this repository
```sh
git clone https://github.com/Azure/radius.git
```
3. Generate CLI docs:
```sh
cd radius
go run ./cmd/docgen/main.go ./docs/content/reference/cli
```
4. Change to docs directory:
```sh
cd docs
```
5. Update submodules:
```sh
git submodule update --init --recursive
```
6. Install npm packages:
```sh
npm install
```

## Run local server
1. Make sure you're still in the `docs` directory
2. Run
```sh
hugo server
```
3. Navigate to `http://localhost:1313/`
