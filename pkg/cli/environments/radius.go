package environments

type RadiusEnvironment struct {
	Name               string `mapstructure:"name" validate:"required"`
	Kind               string `mapstructure:"kind" validate:"required"`
	Context            string `mapstructure:"context" validate:"required"`
	ClusterName        string `mapstructure:"clustername" validate:"required"`
	Namespace          string `mapstructure:"namespace" validate:"required"`
	DefaultApplication string `mapstructure:"defaultapplication" yaml:",omitempty"`
	Scope              string `mapstructure:"scope,omitempty"`
	Id                 string `mapstructure:"id,omitempty"`

	// DEBUG STUFF:

	// RadiusRPLocalURL is an override for local debugging. This allows us us to run the controller + API Service outside the cluster.
	RadiusRPLocalURL         string `mapstructure:"radiusrplocalurl,omitempty"`
	DeploymentEngineLocalURL string `mapstructure:"deploymentenginelocalurl,omitempty"`
	UCPLocalURL              string `mapstructure:"ucplocalurl,omitempty"`
	EnableUCP                bool   `mapstructure:"enableucp,omitempty"`
	UCPResourceGroupName     string `mapstructure:"ucpresourcegroupname,omitempty"`

	// Capture arbitrary other properties
	// We tolerate and allow extra fields - this helps with forwards compat.
	Properties map[string]interface{} `mapstructure:",remain"`
}
