package shellhelpers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// RunRadInitCommand runs rad env init command and times out after specified timeout
func RunRadInitCommand(subscriptionID, resourceGroupName, location string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	// Create the command with our context
	cmd := exec.CommandContext(ctx, "rad", "env", "init", "azure", "--resource-group", resourceGroupName, "--subscription-id", subscriptionID, "--location", location)
	err := runCommand(ctx, cmd)
	return err
}

// RunRadDeployCommand runs rad deploy command and times out after specified timeout
func RunRadDeployCommand(templateFilePath string, timeout time.Duration) error {
	// Check if the template file path exists
	if _, err := os.Stat(templateFilePath); os.IsNotExist(err) {
		fmt.Printf("template file: %s specified does not exist\n", templateFilePath)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	// Create the command with our context
	cmd := exec.CommandContext(ctx, "rad", "deploy", templateFilePath)
	err := runCommand(ctx, cmd)
	return err
}

// RunRadMergeCredentialsCommand merges the kubernetes credentials created by the deployment step
func RunRadMergeCredentialsCommand() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	cmd := exec.CommandContext(ctx, "rad", "env", "merge-credentials", "--name", "azure")
	err := runCommand(ctx, cmd)
	return err
}

func runCommand(ctx context.Context, cmd *exec.Cmd) error {
	out, err := cmd.Output()

	if ctx.Err() == context.DeadlineExceeded {
		fmt.Println("command timed out")
		return ctx.Err()
	}

	// If there's no context error, we know the command completed (or errored).
	fmt.Println("Output:", string(out))
	if err != nil {
		fmt.Println("non-zero exit code:", err)
	}

	return err
}
