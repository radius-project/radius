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

package logstream

import (
	"context"
	"regexp"
	"text/template"
	"time"

	"github.com/fatih/color"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/stern/stern/stern"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

// Format used for each log line.
//
// As long as we use stern for streaming logs, we're limited to the set of fields that they provide to build
// our log messages. For example, we can't use the Radius container name because stern does not provide that to us.
const outputFormat = "{{color .PodColor .PodName}} {{color .ContainerColor .ContainerName}} {{.Message}}\n"

// Impl is the implementation of logstream.Interface.
type Impl struct {
}

// Stream opens a log stream and writes the application's log to the provided writer.
// This function will block until the context is cancelled.
//
// # Function Explanation
//
// Stream() configures and runs Stern, a library for streaming logs from Kubernetes pods, with custom filters and output formats
// based on the provided parameters. It returns an error if there is an issue configuring or running Stern.
func (i *Impl) Stream(ctx context.Context, options Options) error {

	// The functionality of the package is provided almost entirely be github.com/stern/stern.
	// Under the covers, stern is watching pods based on a set of filters and then piping
	// all of the matching logstreams to the writer.
	//
	// If we need to customize the behavior more, we could replace this functionality. The main
	// value of stern is that it's reactive to changes in the cluster. As new pods come online they
	// automatically added to the log stream.
	cfg := stern.Config{
		// Almost ALL of the fields on stern.Config are required. Most of what's here is replicating
		// the defaults of the stern CLI. The library does not provide access to stern's defaults.

		// Fields used to select/filter pods
		ContextName:         options.KubeContext,
		Namespaces:          []string{options.Namespace},
		PodQuery:            regexp.MustCompile(`.*`),
		ContainerQuery:      regexp.MustCompile(`.*`),
		FieldSelector:       fields.Everything(),
		ContainerStates:     []stern.ContainerState{stern.RUNNING},
		InitContainers:      true,
		EphemeralContainers: true,

		// Fields used to configure the lifetime of the command
		Since:     48 * time.Hour,
		TailLines: nil,
		Follow:    true,

		// Fields used to configure output
		Timestamps: false,
		Template:   template.Must(template.New("output").Funcs(functionTable()).Parse(outputFormat)),
		Out:        options.Out,
		ErrOut:     options.Out,
	}

	// This is the only Radius-specific customization we make.
	//
	// We use the `radius.dev/application` label to include pods that are part of an application.
	// This can include the user's Radius containers as well as any Kubernetes resources that are labeled
	// as part of the application (eg: something created with a recipe).
	req, err := labels.NewRequirement(kubernetes.LabelRadiusApplication, selection.Equals, []string{options.ApplicationName})
	if err != nil {
		return err
	}

	cfg.LabelSelector = labels.NewSelector().Add(*req)

	// This will block until the context is cancelled.
	err = stern.Run(ctx, &cfg)
	if err != nil {
		return err
	}

	return nil
}

// functionTable sets the functions available to the text template.
func functionTable() map[string]any {
	return map[string]any{
		"color": func(color color.Color, text string) string {
			// Use the provided color to add ascii escapes.
			return color.SprintFunc()(text)
		},
	}
}
