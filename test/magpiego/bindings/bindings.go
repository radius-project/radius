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

func LoadBindings(registeredProviders map[string]BindingProvider) []Providers {
	valueByBinding := make(map[string]map[string]string)
	for _, env := range os.Environ() {
		typeKeyPair, value := parseEnvVariable(env)
		if typeKeyPair == nil {
			return nil
		}
		tp := typeKeyPair.bindingType
		key := typeKeyPair.bindingKey
		values := valueByBinding[tp]
		if values == nil {
			valueByBinding[tp] = values

		}
		values[key] = value
	}

	var bindings []Providers
	for typ, val := range valueByBinding {
		provider := registeredProviders[typ]
		if provider == nil {
			log.Fatal("no provider could be found for binding of type - ", registeredProviders[typ])
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
	kvPair := strings.SplitN(env, "=", 2)
	envName := kvPair[0]
	if !strings.HasPrefix(envName, "BINDING_") && !strings.HasPrefix(envName, "CONNECTION_") {
		return &typeKey, ""
	}
	parts := strings.Split(envName, "_")
	if len(parts) != 3 {
		return &typeKey, ""
	}
	typeKey.bindingKey = strings.ToUpper(parts[2])
	typeKey.bindingType = strings.ToUpper(parts[1])
	return &typeKey, kvPair[1]
}
