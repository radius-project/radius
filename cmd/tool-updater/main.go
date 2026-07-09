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
	terraformVersionPath := flags.String("terraform-version-file", "", "Terraform compatibility version file")
	err := flags.Parse(args)
	if err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	manifest, err := tooling.LoadManifest(*manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	terraformVersionPathValue := *terraformVersionPath
	if terraformVersionPathValue == "" {
		terraformVersionPathValue = manifest.TerraformVersionFile
	}
	changes, err := tooling.UpdateManifest(context.Background(), &manifest, tooling.NewClient(""))
	if err != nil {
		return fmt.Errorf("update tool metadata: %w", err)
	}
	if len(changes) == 0 {
		fmt.Println("tool metadata is current")
	} else {
		for _, change := range changes {
			fmt.Printf("updated %s\n", change)
		}
		if _, err := tooling.WriteManifest(*manifestPath, manifest); err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}
	}

	if terraform, ok := findTool(manifest, "terraform"); ok {
		if _, err := tooling.WriteTextFile(terraformVersionPathValue, terraform.Version+"\n"); err != nil {
			return fmt.Errorf("write Terraform version file: %w", err)
		}
		if err := tooling.SyncVersionFiles(".", terraform); err != nil {
			return fmt.Errorf("sync Terraform version consumers: %w", err)
		}
	}
	if _, err := tooling.WriteMakeFile(*makePath, manifest); err != nil {
		return fmt.Errorf("write Make metadata: %w", err)
	}
	return nil
}

func findTool(manifest tooling.Manifest, name string) (tooling.Tool, bool) {
	for _, tool := range manifest.Tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return tooling.Tool{}, false
}
