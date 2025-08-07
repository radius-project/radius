/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package terraform

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// getGitURLWithSecrets returns the git URL with secrets information added.
func getGitURLWithSecrets(secrets map[string]string, url *url.URL) string {
	// accessing the secret values and creating the git url with secret information.
	path := fmt.Sprintf("%s://", url.Scheme)
	user := secrets["username"]
	if strings.TrimSpace(user) == "" {
		user = "oauth2"
	}
	path += fmt.Sprintf("%s:", user)

	token, ok := secrets["pat"]
	if ok {
		path += token
	}
	path += fmt.Sprintf("@%s", url.Hostname())

	return path
}

// getURLConfigKeyValue is used to get the key and value details of the url config.
// get the secret values pat and username from secrets and create a git url in
// the format : https://<username>:<pat>@<git>.com
func getURLConfigKeyValue(secrets map[string]string, templatePath string) (string, string, error) {
	url, err := GetGitURL(templatePath)
	if err != nil {
		return "", "", err
	}

	path := getGitURLWithSecrets(secrets, url)

	// git config key will be in the format of url.<git url with secret details>.insteadOf
	// and value returned will the original git url domain, e.g github.com
	return fmt.Sprintf("url.%s.insteadOf", path), fmt.Sprintf("%s://%s", url.Scheme, url.Hostname()), nil
}

// addSecretsToGitConfigIfApplicable adds secrets to the Git configuration file if applicable.
// It is a wrapper function to addSecretsToGitConfig()
func addSecretsToGitConfigIfApplicable(ctx context.Context, config recipes.Configuration, secrets map[string]recipes.SecretData, workingDirectory string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	if len(config.RecipeConfig.Terraform.Authentication.Git.PAT) == 0 {
		return nil
	}

	// Initialize a new Git repository in the terraform working directory.
	_, err := git.PlainInit(workingDirectory, false)
	if err != nil {
		return fmt.Errorf("failed to initialize git in the working directory:%w", err)
	}

	err = setGitConfigForDir(workingDirectory)
	if err != nil {
		return err
	}

	for host, patConfig := range config.RecipeConfig.Terraform.Authentication.Git.PAT {
		secretData, ok := secrets[patConfig.Secret]
		if !ok {
			logger.Error(nil, "Secrets not found for secret store", "secretStoreID", patConfig.Secret)
			return fmt.Errorf("secrets not found for secret store ID %q", patConfig.Secret)
		}

		logger.Info("Configuring PAT/username authentication for Git",
			"host", host,
			"hasUsername", secretData.Data["username"] != "")

		// Handle PAT/username authentication
		urlConfigKey, urlConfigValue, err := getURLConfigKeyValue(secretData.Data, "https://"+host)
		if err != nil {
			return err
		}

		// git config --file .git/config <urlConfigKey> <urlConfigValue>
		cmd := exec.Command("git", "config", "--file", filepath.Join(workingDirectory, ".git", "config"), urlConfigKey, urlConfigValue)
		cmd.Dir = workingDirectory
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to set git config: %w", err)
		}
	}

	return nil
}

// setGitConfigForDir sets a conditional include directive in the global Git configuration file.
// This function modifies the global Git configuration to include a specific Git configuration file
// when the repository is located in the given working directory. The `includeIf` directive is used
// to conditionally include the configuration file located at "<workingDirectory>/.git/config".
func setGitConfigForDir(workingDirectory string) error {
	cmd := exec.Command("git", "config", "--global", fmt.Sprintf("includeIf.gitdir:%s/.path", workingDirectory), workingDirectory+"/.git/config")
	_, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to add conditional include directive : %w", err)
	}

	return nil
}

// unsetGitConfigForDir removes a conditional include directive from the global Git configuration.
// This function modifies the global Git configuration to remove a previously set `includeIf` directive
// for a given working directory.
func unsetGitConfigForDir(workingDirectory string) error {
	cmd := exec.Command("git", "config", "--global", "--unset", fmt.Sprintf("includeIf.gitdir:%s/.path", workingDirectory))
	// We use Run here instead of Output because on success, there is no output, but on failure,
	// there might be output on stderr. We don't capture it here, but Run will return an error.
	if err := cmd.Run(); err != nil {
		// It's possible the config was never set or already removed, so we don't treat errors here as fatal.
		// For instance, if the process was interrupted after setting the config but before the operation completed.
		// We can log the error for debugging purposes.
		ucplog.FromContextOrDiscard(context.Background()).V(ucplog.LevelDebug).Info("failed to unset conditional includeIf directive, this may not be an error", "error", err, "workingDirectory", workingDirectory)
	}

	return nil
}

// GetGitURL returns git url from generic git module source.
// git::https://example.com/project/module -> https://exmaple.com/project/module
func GetGitURL(templatePath string) (*url.URL, error) {
	paths := strings.Split(templatePath, "git::")
	gitURL := paths[len(paths)-1]

	if len(strings.Split(gitURL, "://")) <= 1 {
		gitURL = fmt.Sprintf("https://%s", gitURL)
	}

	url, err := url.Parse(gitURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse git url %s : %w", gitURL, err)
	}

	return url, nil
}

// unsetGitConfigForDirIfApplicable removes a conditional include directive from the global Git configuration if applicable.
// It is a wrapper function to unsetGitConfigForDir()
func unsetGitConfigForDirIfApplicable(ctx context.Context, config recipes.Configuration, workingDirectory string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	if len(config.RecipeConfig.Terraform.Authentication.Git.PAT) == 0 {
		return nil
	}

	logger.Info("Unsetting git config for directory", "directory", workingDirectory)

	return unsetGitConfigForDir(workingDirectory)
}

// configureSSHAuth sets up SSH authentication for Git operations
func configureSSHAuth(workingDirectory string, privateKey string, secrets map[string]string) error {
	logger := ucplog.FromContextOrDiscard(context.Background())

	// Create SSH directory
	sshDir := filepath.Join(workingDirectory, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("failed to create SSH directory: %w", err)
	}

	// Write private key to file
	keyPath := filepath.Join(sshDir, "id_rsa")
	logger.Info("Writing SSH private key", "keyPath", keyPath)
	if err := os.WriteFile(keyPath, []byte(privateKey), 0600); err != nil {
		logger.Error(err, "Failed to write SSH private key")
		return fmt.Errorf("failed to write SSH private key: %w", err)
	}

	// Configure Git to use SSH command with custom key
	sshCommand := fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes", keyPath)

	// Handle StrictHostKeyChecking based on secrets
	strictHostKey := "yes"
	if strict, ok := secrets["strictHostKeyChecking"]; ok && strict == "false" {
		strictHostKey = "no"
		logger.Info("SSH strict host key checking disabled")
	}
	sshCommand += fmt.Sprintf(" -o StrictHostKeyChecking=%s", strictHostKey)

	// Set GIT_SSH_COMMAND for the git config
	cmd := exec.Command("git", "config", "--file", workingDirectory+"/.git/config", "core.sshCommand", sshCommand)
	if _, err := cmd.Output(); err != nil {
		return fmt.Errorf("failed to set SSH command in git config: %w", err)
	}

	return nil
}
