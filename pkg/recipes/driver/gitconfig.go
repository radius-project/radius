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
	"reflect"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/recipes"
)

// getGitURLWithSecrets returns the git URL with secrets information added.
func getGitURLWithSecrets(secrets v20231001preview.SecretStoresClientListSecretsResponse, url *url.URL) string {
	// accessing the secret values and creating the git url with secret information.
	var username, pat *string
	path := fmt.Sprintf("%s://", url.Scheme)
	user, ok := secrets.Data["username"]
	if ok {
		username = user.Value
		path += fmt.Sprintf("%s:", *username)
	}

	token, ok := secrets.Data["pat"]
	if ok {
		pat = token.Value
		path += *pat
	}
	path += fmt.Sprintf("@%s", url.Hostname())

	return path
}

// getURLConfigKeyValue is used to get the key and value details of the url config.
// get the secret values pat and username from secrets and create a git url in
// the format : https://<username>:<pat>@<git>.com and adds it to gitconfig
func getURLConfigKeyValue(secrets v20231001preview.SecretStoresClientListSecretsResponse, templatePath string) (string, string, error) {
	url, err := recipes.GetGitURL(templatePath)
	if err != nil {
		return "", "", err
	}

	path := getGitURLWithSecrets(secrets, url)

	// git config key will be in the format of url.<git url with secret details>.insteadOf
	// and value returned will the original git url domain, e.g github.com
	return fmt.Sprintf("url.%s.insteadOf", path), fmt.Sprintf("%s://%s", url.Scheme, url.Hostname()), nil
}

// Updates the local Git configuration in terraform working directory with credentials for a recipe template path and prefixes the path with environment, application, and resource name to make the entry unique to each recipe execution operation.
//
// Retrieves the git credentials from the provided secrets object
// and adds them to the Git config by running
// git config --file .git/config url<template_path_domain_with_credentails>.insteadOf <template_path_domain>.
func addSecretsToGitConfig(workingDirectory string, secrets v20231001preview.SecretStoresClientListSecretsResponse, templatePath string) error {
	if !strings.HasPrefix(templatePath, "git::") || reflect.DeepEqual(secrets, v20231001preview.SecretStoresClientListSecretsResponse{}) {
		return nil
	}
	// Initialize a new Git repository in the terraform working directory.
	_, err := git.PlainInit(workingDirectory, false)
	if err != nil {
		return fmt.Errorf("falied to initialize git in the working directory:%w", err)
	}

	urlConfigKey, urlConfigValue, err := getURLConfigKeyValue(secrets, templatePath)
	if err != nil {
		return err
	}

	err = setGitConfigForDir(workingDirectory)
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
		return errors.New("failed to add conditional include directive")
	}
	return nil
}

// unsetGitConfigForDir removes a conditional include directive from the global Git configuration.
// This function modifies the global Git configuration to remove a previously set `includeIf` directive
// for a given working directory.
func unsetGitConfigForDir(workingDirectory string, secrets v20231001preview.SecretStoresClientListSecretsResponse, templatePath string) error {
	if !strings.HasPrefix(templatePath, "git::") || reflect.DeepEqual(secrets, v20231001preview.SecretStoresClientListSecretsResponse{}) {
		return nil
	}
	cmd := exec.Command("git", "config", "--global", "--unset", fmt.Sprintf("includeIf.gitdir:%s/.path", workingDirectory))
	_, err := cmd.Output()
	if err != nil {
		return errors.New("failed to unset conditional include directive")
	}
	return nil
}
