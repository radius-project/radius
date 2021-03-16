package utils

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// RunRadInitCommand runs rad env init command and times out after specified timeout
func RunRadInitCommand(subscriptionID, resourceGroupName, location string, template string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	// Create the command with our context
	cmd := exec.CommandContext(ctx, "rad", "env", "init", "azure", "--resource-group", resourceGroupName, "--subscription-id", subscriptionID, "--location", location, "--deployment-template", template)
	err := RunCommand(ctx, cmd)
	return err
}

// RunRadDeployCommand runs rad deploy command and times out after specified timeout
func RunRadDeployCommand(templateFilePath, configFilePath string, timeout time.Duration) error {
	// Check if the template file path exists
	if _, err := os.Stat(templateFilePath); err != nil {
		log.Fatalf("error deploying template file: %s - %s\n", templateFilePath, err.Error())
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	// Create the command with our context
	var cmd *exec.Cmd
	if configFilePath == "" {
		cmd = exec.CommandContext(ctx, "rad", "deploy", templateFilePath)
	} else {
		if _, err := os.Stat(configFilePath); err != nil {
			log.Fatalf("error deploying template using configfile: %s - %s\n", configFilePath, err.Error())
			return err
		}
		cmd = exec.CommandContext(ctx, "rad", "deploy", templateFilePath, "--config", configFilePath)
	}
	err := RunCommand(ctx, cmd)
	return err
}

// RunRadMergeCredentialsCommand merges the kubernetes credentials created by the deployment step
func RunRadMergeCredentialsCommand(configFilePath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	var cmd *exec.Cmd
	if configFilePath == "" {
		cmd = exec.CommandContext(ctx, "rad", "env", "merge-credentials", "--name", "azure")
	} else {
		if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
			log.Fatalf("template file: %s specified does not exist\n", configFilePath)
			return err
		}
		fmt.Printf("Using config file: %s for merge credentials", configFilePath)
		cmd = exec.CommandContext(ctx, "rad", "env", "merge-credentials", "--name", "azure", "--config", configFilePath)
	}
	err := RunCommand(ctx, cmd)
	if err != nil {
		log.Fatal("Could not merge kubernetes credentials for cluster: " + err.Error())
	}
	return err
}

// RunRadDeleteApplicationsCommand deletes all applications deployed by Radius in the specified resource group
func RunRadDeleteApplicationsCommand(resourceGroupName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	// TODO: Once we have a rad env delete command, replace this logic with that
	currentPath, _ := os.Getwd()
	scriptPath := filepath.Join(currentPath, "delete-applications")
	cmd := exec.CommandContext(ctx, scriptPath, resourceGroupName)
	err := RunCommand(ctx, cmd)
	if err != nil {
		fmt.Println("non-zero exit code:", err.Error())
	}
	return err
}

// RunCommand runs a shell command
func RunCommand(ctx context.Context, cmd *exec.Cmd) error {
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		fmt.Println("command timed out")
		return ctx.Err()
	}

	// If there's no context error, we know the command completed (or errored).
	fmt.Println("Output:", string(out))
	if err != nil {
		fmt.Println("non-zero exit code:", err.Error())
	}

	return err
}
