# Test CRD Generator

## Overview

This directory contains a script that is used to download CRDs that we need for unit testing. 

The pattern is based on the following Kubebuilder documentation: https://book-v3.book.kubebuilder.io/reference/using_an_external_type#prepare-for-testing

## Usage

To download the CRDs, run the following command:

```bash
go run ./test/crd/generator.go
```

This will download the CRDs to the `test/crd` directory, under the appropriate subdirectory.

## Updating CRDs

To update the CRDs that get generated, update the `crds` variable in the `generator.go` script.
