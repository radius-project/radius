#!/bin/sh

# Switch to source directory.
cd "`dirname "$0"`"

#git submodule update --init --recursive

if go build && go test && go build cli/canonicaljson.go &&
	canonicaljson-spec/test.sh ./canonicaljson; then

	echo PASS
else
	echo FAIL
	exit 1
fi
