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

package driver

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/radius-project/radius/pkg/recipes"
)

// getGitURLWithSecrets returns the git URL with secrets information added.
func getGitURLWithSecrets(secrets map[string]string, url *url.URL) string {
	// accessing the secret values and creating the git url with secret information.
	path := fmt.Sprintf("%s://", url.Scheme)
	user, ok := secrets["username"]
	if ok {
		path += fmt.Sprintf("%s:", user)
	}

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

// Updates the local Git configuration in terraform working directory with credentials for a recipe template path, and global git configuration with includeif directive to point to the local config file
// in the working directory which will be used when terraform(in turn calls git) runs from that working directory.
//
// Retrieves the git credentials from the provided secrets object
// and adds them to the Git config by running
// git config --file .git/config url<template_path_domain_with_credentails>.insteadOf <template_path_domain>.
func addSecretsToGitConfig(workingDirectory string, secrets map[string]string, templatePath string) error {
	if !strings.HasPrefix(templatePath, "git::") || secrets == nil || len(secrets) == 0 {
		return nil
	}

	// Initialize a new Git repository in the terraform working directory.
	_, err := git.PlainInit(workingDirectory, false)
	if err != nil {
		return fmt.Errorf("falied to initialize git in the working directory:%w", err)
	}

	err = setGitConfigForDir(workingDirectory)
	if err != nil {
		return err
	}

	urlConfigKey, urlConfigValue, err := getURLConfigKeyValue(secrets, templatePath)
	if err != nil {
		return err
	}

	cmd := exec.Command("git", "config", "--file", workingDirectory+"/.git/config", urlConfigKey, urlConfigValue)
	_, err = cmd.Output()
	if err != nil {
		return errors.New("failed to add git config")
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
func unsetGitConfigForDir(workingDirectory string, secrets map[string]string, templatePath string) error {
	if !strings.HasPrefix(templatePath, "git::") || secrets == nil || len(secrets) == 0 {
		return nil
	}

	cmd := exec.Command("git", "config", "--global", "--unset", fmt.Sprintf("includeIf.gitdir:%s/.path", workingDirectory))
	_, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to unset conditional includeIf directive : %w", err)
	}

	return nil
}

// GetGitURL returns git url from generic git module source.
// git::https://exmaple.com/project/module -> https://exmaple.com/project/module
func GetGitURL(templatePath string) (*url.URL, error) {
	paths := strings.Split(templatePath, "git::")
	gitUrl := paths[len(paths)-1]

	if !(len(strings.Split(gitUrl, "://")) > 1) {
		gitUrl = fmt.Sprintf("https://%s", gitUrl)
	}

	url, err := url.Parse(gitUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse git url %s : %w", gitUrl, err)
	}

	return url, nil
}

// addSecretsToGitConfigIfApplicable adds secrets to the Git configuration file if applicable.
// It is a wrapper function to addSecretsToGitConfig()
func addSecretsToGitConfigIfApplicable(secretStoreID string, secretData map[string]recipes.SecretData, requestDirPath string, templatePath string) error {
	if secretStoreID == "" || secretData == nil {
		return nil
	}

	secrets, ok := secretData[secretStoreID]
	if !ok {
		return fmt.Errorf("secrets not found for secret store ID %q", secretStoreID)
	}

	err := addSecretsToGitConfig(requestDirPath, secrets.Data, templatePath)
	if err != nil {
		return err
	}

	return nil
}

// unsetGitConfigForDir removes a conditional include directive from the global Git configuration if applicable.
// It is a wrapper function to unsetGitConfigForDir()
func unsetGitConfigForDirIfApplicable(secretStoreID string, secretData map[string]recipes.SecretData, requestDirPath string, templatePath string) error {
	if secretStoreID == "" || secretData == nil {
		return nil
	}

	secrets, ok := secretData[secretStoreID]
	if !ok {
		return fmt.Errorf("secrets not found for secret store ID %q", secretStoreID)
	}

	err := unsetGitConfigForDir(requestDirPath, secrets.Data, templatePath)
	if err != nil {
		return err
	}

	return nil
}
