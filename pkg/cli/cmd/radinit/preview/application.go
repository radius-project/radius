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

package preview

import (
	"github.com/radius-project/radius/pkg/cli/cmd/radinit/common"
)

const (
	confirmSetupApplicationPrompt = common.ConfirmSetupApplicationPrompt
	enterApplicationNamePrompt    = common.EnterApplicationNamePrompt
)

func (r *Runner) enterApplicationOptions(options *initOptions) error {
	scaffold, name, err := common.EnterApplicationOptions(r.Prompter)
	if err != nil {
		return err
	}
	options.Application.Scaffold = scaffold
	options.Application.Name = name
	return nil
}
