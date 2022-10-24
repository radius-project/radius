// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package awsproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/project-radius/radius/pkg/middleware"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func ParseAWSRequest(ctx context.Context, opts ctrl.Options, r *http.Request) (awsclient.AWSCloudControlClient, string, resources.ID, error) {
	// Common parsing in AWS plane requests
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, "", resources.ID{}, err
	}

	var client awsclient.AWSCloudControlClient
	if opts.AWSCloudControlClient == nil {
		client = cloudcontrol.NewFromConfig(cfg)
	} else {
		client = opts.AWSCloudControlClient
	}
	path := middleware.GetRelativePath(opts.BasePath, r.URL.Path)
	id, err := resources.ParseByMethod(path, r.Method)
	if err != nil {
		return nil, "", resources.ID{}, err
	}

	resourceType := resources.ToAWSResourceType(id)
	return client, resourceType, id, nil
}

func lookupAWSResourceSchema(opts ctrl.Options, resourceType string) ([]interface{}, error) {
	sess, err := session.NewSession()
	if err != nil {
		return nil, err
	}

	var svc awsclient.AWSCloudFormationClient
	if opts.AWSCloudFormationClient == nil {
		svc = cloudformation.New(sess, aws.NewConfig())
	} else {
		svc = opts.AWSCloudFormationClient
	}

	output, err := svc.DescribeType(&cloudformation.DescribeTypeInput{
		TypeName: aws.String(resourceType),
		Type:     aws.String("RESOURCE"),
	})
	if err != nil {
		return nil, err
	}

	description := map[string]interface{}{}
	err = json.Unmarshal([]byte(*output.Schema), &description)
	if err != nil {
		return nil, err
	}
	primaryIdentifier := description["primaryIdentifier"].([]interface{})
	return primaryIdentifier, nil
}

func getResourceIDWithMultiIdentifiers(opts ctrl.Options, url string, resourceType string, properties map[string]interface{}) (string, error) {
	primaryIdentifiers, err := lookupAWSResourceSchema(opts, resourceType)
	if err != nil {
		return "", err
	}
	var resourceID string
	for _, pi := range primaryIdentifiers {
		// Primary identifier is of the form /properties/<property-name>
		propertyName := strings.Split(pi.(string), "/")[2]

		if _, ok := properties[propertyName]; !ok {
			// Mandatory property is missing
			err := fmt.Errorf("Mandatory property %s is missing", propertyName)
			return "", err
		}
		resourceID += properties[propertyName].(string) + "|"
	}

	resourceID = strings.TrimSuffix(resourceID, "|")
	return resourceID, nil

}

func readPropertiesFromBody(req *http.Request) (map[string]interface{}, error) {
	decoder := json.NewDecoder(req.Body)
	defer req.Body.Close()

	body := map[string]interface{}{}
	err := decoder.Decode(&body)
	if err != nil {
		return nil, err
	}

	properties := map[string]interface{}{}
	obj, ok := body["properties"]
	if ok {
		pp, ok := obj.(map[string]interface{})
		if ok {
			properties = pp
		}
	}
	return properties, nil
}

func computeResourceID(id resources.ID, resourceID string) string {
	computedID := strings.Split(id.String(), "/:")[0] + resources.SegmentSeparator + resourceID
	return computedID
}
