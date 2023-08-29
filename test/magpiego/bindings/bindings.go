package bindings

import (
	"log"
	"os"
	"strings"
)

type BindingStatus struct {
	Ok      bool
	Message string
}

type BindingProvider func(map[string]string) BindingStatus

type Providers struct {
	BindingProviders BindingProvider
	EnvVars          map[string]string
}

type bindingTypeKey struct {
	bindingType string
	bindingKey  string
}

// LoadBindings parses environment variables and creates a slice of Providers based on the registered BindingProviders.
func LoadBindings(registeredProviders map[string]BindingProvider) []Providers {
	valueByBinding := make(map[string]map[string]string)
	// We match env-vars using the form CONNECTION_<KIND>_VALUE, so group them by that structure.
	// Each binding type get a collection of key-value pairs
	for _, env := range os.Environ() {
		typeKeyPair, value := parseEnvVariable(strings.Trim(env, " "))
		if typeKeyPair == nil || typeKeyPair.bindingKey != "" && typeKeyPair.bindingType != "" {
			tp := typeKeyPair.bindingType
			key := typeKeyPair.bindingKey
			values, ok := valueByBinding[tp]
			if !ok {
				values = make(map[string]string)
				valueByBinding[tp] = values
			}
			values[key] = value
		}
	}
	//We've got all the values grouped by type, we can walk that list and instantiate the bindings.
	var bindings []Providers
	for typ, val := range valueByBinding {
		provider := registeredProviders[typ]
		if provider == nil {
			log.Println("no provider could be found for binding of type - ", registeredProviders[typ])
			return bindings
		}
		bnding := Providers{
			BindingProviders: provider,
			EnvVars:          val,
		}
		bindings = append(bindings, bnding)
	}
	return bindings
}

func parseEnvVariable(env string) (*bindingTypeKey, string) {
	var typeKey bindingTypeKey
	kvPair := strings.Split(env, "=")
	envName := kvPair[0]
	if !strings.HasPrefix(envName, "CONNECTION_") {
		return &typeKey, ""
	}
	parts := strings.Split(strings.ToUpper(envName), "_")
	if len(parts) != 3 {
		return &typeKey, ""
	}
	typeKey.bindingKey = parts[2]
	typeKey.bindingType = parts[1]
	return &typeKey, os.Getenv(envName)
}
