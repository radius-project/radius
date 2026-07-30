package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/radius-project/radius/internal/tooling"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "tool-updater: %v\n", err)
		os.Exit(1) //nolint:forbidigo // this is OK inside the main function.
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("a command is required: generate-make or update")
	}

	switch args[0] {
	case "generate-make":
		return generateMake(args[1:])
	case "update":
		return update(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func generateMake(args []string) error {
	flags := flag.NewFlagSet("generate-make", flag.ContinueOnError)
	manifestPath := flags.String("manifest", "build/tools.yaml", "tool manifest path")
	outputPath := flags.String("output", "build/tools.generated.mk", "generated Make include path")
	err := flags.Parse(args)
	if err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	manifest, err := tooling.LoadManifest(*manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	changed, err := tooling.WriteMakeFile(*outputPath, manifest)
	if err != nil {
		return fmt.Errorf("write Make metadata: %w", err)
	}
	if changed {
		fmt.Printf("generated %s\n", *outputPath)
	}
	return nil
}

func update(args []string) error {
	flags := flag.NewFlagSet("update", flag.ContinueOnError)
	manifestPath := flags.String("manifest", "build/tools.yaml", "tool manifest path")
	makePath := flags.String("makefile", "build/tools.generated.mk", "generated Make include path")
	err := flags.Parse(args)
	if err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	manifest, err := tooling.LoadManifest(*manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	result, err := tooling.UpdateManifest(context.Background(), &manifest, tooling.NewClient(""))
	if err != nil {
		return fmt.Errorf("update tool metadata: %w", err)
	}
	for _, held := range result.Held {
		fmt.Printf("held %s\n", held)
	}
	switch {
	case len(result.Changes) > 0:
		for _, change := range result.Changes {
			fmt.Printf("updated %s\n", change)
		}
		if _, err := tooling.WriteManifest(*manifestPath, manifest); err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}
	case len(result.Held) > 0:
		fmt.Println("no tool metadata changes applied; every newer release is still inside the cooldown")
	default:
		fmt.Println("tool metadata is current")
	}

	if err := syncVersionFiles(".", manifest); err != nil {
		return err
	}
	if _, err := tooling.WriteMakeFile(*makePath, manifest); err != nil {
		return fmt.Errorf("write Make metadata: %w", err)
	}
	return nil
}

func syncVersionFiles(root string, manifest tooling.Manifest) error {
	for _, tool := range manifest.Tools {
		if err := tooling.SyncVersionFiles(root, tool); err != nil {
			return fmt.Errorf("sync %s version consumers: %w", tool.Name, err)
		}
	}
	return nil
}
