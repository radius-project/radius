// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"

	"github.com/Azure/radius/pkg/radrp/schemav3"
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "rad",
	Short: "Radius RP manifest generator",
	Long: `Radius RP manifest generator
	
	The manfifest generator is used to update our Custom Provider manifest when the set of types we expose
	changes. We need to supply the list of resource types when registering a Custom Provider and we data-drive
	the authoring of this list to make things more maintainable.`,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE:          run,
}

func init() {
	RootCmd.Flags().String("input", "", "The input file (rp-full.input.json)")
	RootCmd.Flags().String("output", "", "The output file (rp-full.json). Will be overwritten")
	RootCmd.Flags().String("resources", "", "The resource manifest (resource-types.json)")

	_ = RootCmd.MarkPersistentFlagRequired("input")
	_ = RootCmd.MarkPersistentFlagRequired("output")
	_ = RootCmd.MarkPersistentFlagRequired("resources")
}

func main() {
	if err := RootCmd.ExecuteContext(context.Background()); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	inputFile, err := cmd.Flags().GetString("input")
	if err != nil {
		return err
	}

	outputFile, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}
	resourcesFile, err := cmd.Flags().GetString("resources")
	if err != nil {
		return err
	}

	inputBytes, err := ioutil.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", inputFile, err)
	}

	template := map[string]interface{}{}
	err = json.Unmarshal(inputBytes, &template)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON %q: %w", inputFile, err)
	}

	resourcesBytes, err := ioutil.ReadFile(resourcesFile)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", resourcesFile, err)
	}

	manifest := schemav3.Manifest{}
	err = json.Unmarshal(resourcesBytes, &manifest)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON %q: %w", inputFile, err)
	}

	// Update the template content in memory and then overwrite the file.
	err = update(template, manifest)
	if err != nil {
		return err
	}

	outputBytes, err := json.MarshalIndent(template, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	err = ioutil.WriteFile(outputFile, outputBytes, 0644)
	if err != nil {
		return err
	}
	return nil
}

type resourceTypeEntry struct {
	Name        string `json:"name"`
	RoutingType string `json:"routingType"`
	Endpoint    string `json:"endpoint"`
}

func NewResourceTypeEntry(resourceType string) resourceTypeEntry {
	if resourceType != "Application" {
		resourceType = "Application/" + resourceType
	}

	return resourceTypeEntry{
		Name:        resourceType,
		RoutingType: "Proxy",
		Endpoint:    "[concat('https://', parameters('siteName'), '.azurewebsites.net/{requestPath}')]",
	}
}

func update(template map[string]interface{}, manifest schemav3.Manifest) error {
	// We're going to modify the template by setting the 'resourceTypes' variable at the top level
	// of scope. This allows us to avoid coupling to the structure of the template, we're just coupled
	// to two variable names instead: 'siteName' and 'resourceTypes'
	//
	// We don't use parameters for this because we want to be able to view/diff the structure
	// of the template as part of a PR.
	variables, ok := template["variables"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("cannot find variables node")
	}

	// Sort types for consistency
	types := []string{}
	entries := []resourceTypeEntry{}
	for resourceType := range manifest.Resources {
		types = append(types, resourceType)
	}
	sort.Strings(types)

	for _, resourceType := range types {
		entries = append(entries, NewResourceTypeEntry(resourceType))
	}

	variables["resourceTypes"] = entries
	return nil
}
