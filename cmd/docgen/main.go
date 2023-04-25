// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/project-radius/radius/cmd/rad/cmd"
	"github.com/spf13/cobra/doc"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: go run cmd/docgen/main.go <output directory>") //nolint:forbidigo // this is OK inside the main function.
	}

	output := os.Args[1]
	_, err := os.Stat(output)
	if os.IsNotExist(err) {
		err = os.Mkdir(output, 0755)
		if err != nil {
			log.Fatal(err) //nolint:forbidigo // this is OK inside the main function.
		}
	} else if err != nil {
		log.Fatal(err) //nolint:forbidigo // this is OK inside the main function.
	}

	err = doc.GenMarkdownTreeCustom(cmd.RootCmd, output, frontmatter, link)
	if err != nil {
		log.Fatal(err) //nolint:forbidigo // this is OK inside the main function.
	}
}

const template = `---
type: docs
date: %s
title: "%s CLI reference"
linkTitle: "%s"
slug: %s
url: %s
description: "Details on the %s Radius CLI command"
---
`

func frontmatter(filename string) string {
	now := time.Now().Format(time.RFC3339)
	name := filepath.Base(filename)
	base := strings.TrimSuffix(name, path.Ext(name))
	command := strings.Replace(base, "_", " ", -1)
	url := "/reference/cli/" + strings.ToLower(base) + "/"
	return fmt.Sprintf(template, now, command, command, base, url, command)
}

func link(name string) string {
	base := strings.TrimSuffix(name, path.Ext(name))
	return "{{< ref " + strings.ToLower(base) + ".md >}}"
}
