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

	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/recipes"
)

// getGitURLWithSecrets returns the git URL with secrets information added.
func getGitURLWithSecrets(secrets v20231001preview.SecretStoresClientListSecretsResponse, url *url.URL) string {
	// accessing the secret values and creating the git url with secret information.
	var username, pat *string
	path := "https://"
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
	return fmt.Sprintf("url.%s.insteadOf", path), url.Hostname(), nil
}

// Updates the global Git configuration with credentials for a recipe template path and prefixes the path with environment, application, and resource name to make the entry unique to each recipe execution operation.
//
// Retrieves the git credentials from the provided secrets object
// and adds them to the Git config by running
// git config --global url<template_path_domain_with_credentails>.insteadOf <template_path_domain>.
func addSecretsToGitConfig(secrets v20231001preview.SecretStoresClientListSecretsResponse, recipeMetadata *recipes.ResourceMetadata, templatePath string) error {
	urlConfigKey, urlConfigValue, err := getURLConfigKeyValue(secrets, templatePath)
	if err != nil {
		return err
	}

	prefix, err := recipes.GetURLPrefix(recipeMetadata)
	if err != nil {
		return err
	}
	urlConfigValue = fmt.Sprintf("%s%s", prefix, urlConfigValue)
	cmd := exec.Command("git", "config", "--global", urlConfigKey, urlConfigValue)
	_, err = cmd.Output()
	if err != nil {
		return errors.New("failed to add git config")
	}

	return nil
}

// Unset the git credentials information from .gitconfig by running
// git config --global --unset url<template_path_domain_with_credentails>.insteadOf
func unsetSecretsFromGitConfig(secrets v20231001preview.SecretStoresClientListSecretsResponse, templatePath string) error {
	urlConfigKey, _, err := getURLConfigKeyValue(secrets, templatePath)
	if err != nil {
		return err
	}

	cmd := exec.Command("git", "config", "--global", "--unset", urlConfigKey)
	_, err = cmd.Output()
	if err != nil {
		return errors.New("failed to unset git config")
	}

	return nil
}
