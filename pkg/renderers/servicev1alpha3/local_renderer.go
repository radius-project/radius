// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicev1alpha3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
)

type LocalRenderer struct {
}

func (r *LocalRenderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	properties, err := convert(resource)
	if err != nil {
		return nil, err
	}

	// Right now we only have things in connections and ports as rendering dependencies - we'll add more things
	// in the future... eg: volumes
	//
	// Anywhere we accept a resource ID in the model should have its value returned from here
	deps := []azresources.ResourceID{}
	for _, connection := range properties.Connections {
		resourceId, err := azresources.Parse(connection.Source)
		if err != nil {
			return nil, err
		}
		deps = append(deps, resourceId)
	}

	for _, port := range properties.Ports {
		if port.Provides == "" {
			continue
		}

		resourceId, err := azresources.Parse(port.Provides)
		if err != nil {
			return nil, err
		}
		deps = append(deps, resourceId)
	}

	return deps, nil
}

// Render is the WorkloadRenderer implementation for containerized workload.
func (r *LocalRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties, err := convert(options.Resource)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	if properties.Run["kind"].(string) == "executable" {
		b, err := json.Marshal(&properties.Run)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		executable := Executable{}
		err = json.Unmarshal(b, &executable)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		output, err := r.makeExecutable(ctx, options.Resource, executable, options.Dependencies, properties)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		return renderers.RendererOutput{Resources: []outputresource.OutputResource{output}}, nil
	} else {
		return renderers.RendererOutput{}, errors.New("executable is required right now")
	}
}

func (r *LocalRenderer) makeExecutable(ctx context.Context, resource renderers.RendererResource, executable Executable, dependencies map[string]renderers.RendererDependency, properties *ServiceProperties) (outputresource.OutputResource, error) {
	ports := []radiusv1alpha3.ExecutablePort{}
	for name, port := range properties.Ports {
		converted := radiusv1alpha3.ExecutablePort{
			Name:    name,
			Dynamic: port.Dynamic,
			Port:    port.Port,
		}

		if port.Dynamic {
			if len(properties.Ports) == 1 {
				converted.Env = append(converted.Env, "PORT")
			}

			converted.Env = append(converted.Env, fmt.Sprintf("%s_PORT", strings.ToUpper(name)))
		}

		ports = append(ports, converted)
	}

	env := map[string]string{}
	for k, v := range properties.Env {
		switch val := v.(type) {
		case string:
			env[k] = val
		case float64: // Float is used by the JSON serializer
			env[k] = strconv.Itoa(int(val))
		case int:
			env[k] = strconv.Itoa(val)
		}
	}

	connectionVars := makeEnvironmentVariablesForConnections(properties.Connections, dependencies)
	for k, v := range connectionVars {
		env[k] = v
	}

	deployment := radiusv1alpha3.Executable{
		ObjectMeta: metav1.ObjectMeta{
			Name:   kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
			Labels: kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Spec: radiusv1alpha3.ExecutableSpec{
			Executable:       executable.Name,
			WorkingDirectory: executable.WorkingDirectory,
			Args:             executable.Args,
			Env:              env,
			Ports:            ports,
		},
	}

	output := outputresource.NewKubernetesOutputResource(outputresource.LocalIDDeployment, &deployment, deployment.ObjectMeta)
	return output, nil
}
