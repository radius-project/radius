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
	selectClusterPrompt = common.SelectClusterPrompt
)

func (r *Runner) enterClusterOptions(options *initOptions) error {
	result, err := common.EnterClusterOptions(r.KubernetesInterface, r.HelmInterface, r.Prompter, r.Full)
	if err != nil {
		return err
	}
	options.Cluster.Install = result.Install
	options.Cluster.Namespace = result.Namespace
	options.Cluster.Context = result.Context
	options.Cluster.Version = result.Version
	return nil
}
