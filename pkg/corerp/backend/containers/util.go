package containers

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sort"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
	corev1 "k8s.io/api/core/v1"
)

func GetDependencyIDs(ctx context.Context, resource *datamodel.ContainerResource) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error) {
	properties := resource.Properties

	// Right now we only have things in connections and ports as rendering dependencies - we'll add more things
	// in the future... eg: volumes
	//
	// Anywhere we accept a resource ID in the model should have its value returned from here

	// ensure that users cannot use DNS-SD and httproutes simultaneously.
	for _, connection := range properties.Connections {
		if isURL(connection.Source) {
			continue
		}

		// if the source is not a URL, it either a resourceID or invalid.
		resourceID, err := resources.ParseResource(connection.Source)
		if err != nil {
			return nil, nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid source: %s. Must be either a URL or a valid resourceID", connection.Source))
		}

		// Non-radius Azure connections that are accessible from Radius container resource.
		if connection.IAM.Kind.IsKind(datamodel.KindAzure) {
			azureResourceIDs = append(azureResourceIDs, resourceID)
			continue
		}

		if resources_radius.IsRadiusResource(resourceID) {
			radiusResourceIDs = append(radiusResourceIDs, resourceID)
			continue
		}
	}

	for _, port := range properties.Container.Ports {
		provides := port.Provides

		// if provides is empty, skip this port. A service for this port will be generated later on.
		if provides == "" {
			continue
		}

		resourceID, err := resources.ParseResource(provides)
		if err != nil {
			return nil, nil, v1.NewClientErrInvalidRequest(err.Error())
		}

		if resources_radius.IsRadiusResource(resourceID) {
			radiusResourceIDs = append(radiusResourceIDs, resourceID)
			continue
		}
	}

	for _, volume := range properties.Container.Volumes {
		switch volume.Kind {
		case datamodel.Persistent:
			resourceID, err := resources.ParseResource(volume.Persistent.Source)
			if err != nil {
				return nil, nil, v1.NewClientErrInvalidRequest(err.Error())
			}

			if resources_radius.IsRadiusResource(resourceID) {
				radiusResourceIDs = append(radiusResourceIDs, resourceID)
				continue
			}
		}
	}

	return radiusResourceIDs, azureResourceIDs, nil
}

func getSortedKeys(env map[string]corev1.EnvVar) []string {
	keys := []string{}
	for k := range env {
		key := k
		keys = append(keys, key)
	}

	sort.Strings(keys)
	return keys
}

func isURL(input string) bool {
	_, err := url.ParseRequestURI(input)

	// if first character is a slash, it's not a URL. It's a path.
	if input == "" || err != nil || input[0] == '/' {
		return false
	}
	return true
}

func parseURL(sourceURL string) (scheme, hostname, port string, err error) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return "", "", "", err
	}

	scheme = u.Scheme
	host := u.Host

	hostname, port, err = net.SplitHostPort(host)
	if err != nil {
		return "", "", "", err
	}

	return scheme, hostname, port, nil
}
