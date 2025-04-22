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

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/acarl005/stripansi"
	"github.com/radius-project/radius/pkg/azure/clientv2"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/aws"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	app_delete "github.com/radius-project/radius/pkg/cli/cmd/app/delete"
	app_graph "github.com/radius-project/radius/pkg/cli/cmd/app/graph"
	app_list "github.com/radius-project/radius/pkg/cli/cmd/app/list"
	app_show "github.com/radius-project/radius/pkg/cli/cmd/app/show"
	app_status "github.com/radius-project/radius/pkg/cli/cmd/app/status"
	bicep_generate_kubernetes_manifest "github.com/radius-project/radius/pkg/cli/cmd/bicep/generatekubernetesmanifest"
	bicep_publish "github.com/radius-project/radius/pkg/cli/cmd/bicep/publish"
	bicep_publishextension "github.com/radius-project/radius/pkg/cli/cmd/bicep/publishextension"
	credential "github.com/radius-project/radius/pkg/cli/cmd/credential"
	cmd_deploy "github.com/radius-project/radius/pkg/cli/cmd/deploy"
	env_create "github.com/radius-project/radius/pkg/cli/cmd/env/create"
	env_delete "github.com/radius-project/radius/pkg/cli/cmd/env/delete"
	env_switch "github.com/radius-project/radius/pkg/cli/cmd/env/envswitch"
	env_list "github.com/radius-project/radius/pkg/cli/cmd/env/list"
	"github.com/radius-project/radius/pkg/cli/cmd/env/namespace"
	env_show "github.com/radius-project/radius/pkg/cli/cmd/env/show"
	env_update "github.com/radius-project/radius/pkg/cli/cmd/env/update"
	group "github.com/radius-project/radius/pkg/cli/cmd/group"
	"github.com/radius-project/radius/pkg/cli/cmd/install"
	install_kubernetes "github.com/radius-project/radius/pkg/cli/cmd/install/kubernetes"
	"github.com/radius-project/radius/pkg/cli/cmd/radinit"
	recipe_list "github.com/radius-project/radius/pkg/cli/cmd/recipe/list"
	recipe_register "github.com/radius-project/radius/pkg/cli/cmd/recipe/register"
	recipe_show "github.com/radius-project/radius/pkg/cli/cmd/recipe/show"
	recipe_unregister "github.com/radius-project/radius/pkg/cli/cmd/recipe/unregister"
	resource_create "github.com/radius-project/radius/pkg/cli/cmd/resource/create"
	resource_delete "github.com/radius-project/radius/pkg/cli/cmd/resource/delete"
	resource_list "github.com/radius-project/radius/pkg/cli/cmd/resource/list"
	resource_show "github.com/radius-project/radius/pkg/cli/cmd/resource/show"
	resourceprovider_create "github.com/radius-project/radius/pkg/cli/cmd/resourceprovider/create"
	resourceprovider_delete "github.com/radius-project/radius/pkg/cli/cmd/resourceprovider/delete"
	resourceprovider_list "github.com/radius-project/radius/pkg/cli/cmd/resourceprovider/list"
	resourceprovider_show "github.com/radius-project/radius/pkg/cli/cmd/resourceprovider/show"
	resourcetype_create "github.com/radius-project/radius/pkg/cli/cmd/resourcetype/create"
	resourcetype_delete "github.com/radius-project/radius/pkg/cli/cmd/resourcetype/delete"
	resourcetype_list "github.com/radius-project/radius/pkg/cli/cmd/resourcetype/list"
	resourcetype_show "github.com/radius-project/radius/pkg/cli/cmd/resourcetype/show"
	"github.com/radius-project/radius/pkg/cli/cmd/run"
	"github.com/radius-project/radius/pkg/cli/cmd/uninstall"
	uninstall_kubernetes "github.com/radius-project/radius/pkg/cli/cmd/uninstall/kubernetes"
	upgrade "github.com/radius-project/radius/pkg/cli/cmd/upgrade"
	upgrade_kubernetes "github.com/radius-project/radius/pkg/cli/cmd/upgrade/kubernetes"
	version "github.com/radius-project/radius/pkg/cli/cmd/version"
	workspace_create "github.com/radius-project/radius/pkg/cli/cmd/workspace/create"
	workspace_delete "github.com/radius-project/radius/pkg/cli/cmd/workspace/delete"
	workspace_list "github.com/radius-project/radius/pkg/cli/cmd/workspace/list"
	workspace_show "github.com/radius-project/radius/pkg/cli/cmd/workspace/show"
	workspace_switch "github.com/radius-project/radius/pkg/cli/cmd/workspace/switch"
	"github.com/radius-project/radius/pkg/cli/config"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/deploy"
	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/kubernetes/logstream"
	"github.com/radius-project/radius/pkg/cli/kubernetes/portforward"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// RootCmd is the root command of the rad CLI. This is exported so we can generate docs for it.
var RootCmd = &cobra.Command{
	Use:               "rad",
	Short:             "Radius CLI",
	Long:              `Radius CLI`,
	SilenceErrors:     true,
	SilenceUsage:      true,
	DisableAutoGenTag: true,
}

const (
	serviceName string = "cli"
	tracerName  string = "cli"
)

var applicationCmd = NewAppCommand()
var resourceCmd = NewResourceCommand()
var resourceProviderCmd = NewResourceProviderCommand()
var resourceTypeCmd = NewResourceTypeCommand()
var recipeCmd = NewRecipeCommand()
var envCmd = NewEnvironmentCommand()
var workspaceCmd = NewWorkspaceCommand()

var ConfigHolderKey = framework.NewContextKey("config")
var ConfigHolder = &framework.ConfigHolder{}

func prettyPrintRPError(err error) string {
	if new := clientv2.TryUnfoldResponseError(err); new != nil {
		m, err := prettyPrintJSON(new)
		if err == nil {
			return m
		}
	}

	return err.Error()
}

func prettyPrintJSON(o any) (string, error) {
	b, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
// It also initializes the tracerprovider for cli.
//
// Execute returns true
func Execute() error {
	ctx := context.WithValue(context.Background(), ConfigHolderKey, ConfigHolder)

	shutdown, err := initTracer()
	if err != nil {
		fmt.Println("Error:", err)
		return err
	}

	defer func() {
		_ = shutdown(ctx)
	}()

	tr := otel.Tracer(tracerName)
	spanName := getRootSpanName()
	ctx, span := tr.Start(ctx, spanName)
	defer span.End()
	err = RootCmd.ExecuteContext(ctx)
	if clierrors.IsFriendlyError(err) {
		fmt.Println(err.Error())
		fmt.Println("") // Output an extra blank line for readability
		return err
	} else if err != nil {
		errText := prettyPrintRPError(err)

		// Remove any ANSI escape sequences from the error text. We may be displaying untrusted
		// data in an error message for an "unhandled" error. This will prevent the error text
		// from potentially corrupting the terminal.
		errText = stripansi.Strip(errText)

		fmt.Println("Error:", errText)
		fmt.Println("\nTraceId: ", span.SpanContext().TraceID().String())
		fmt.Println("") // Output an extra blank line for readability
		return err
	}

	return nil
}

func initTracer() (func(context.Context) error, error) {
	// Intialize the tracer provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		)),
	)

	// Set the tracer provider as "global" for the CLI process.
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}))

	return tp.Shutdown, nil
}

func init() {
	cobra.OnInitialize(initConfig)

	// Must set the default logger to use controller-runtime.
	runtimelog.SetLogger(zap.New())

	RootCmd.PersistentFlags().StringVar(&ConfigHolder.ConfigFilePath, "config", "", "config file (default \"$HOME/.rad/config.yaml\")")

	outputDescription := fmt.Sprintf("output format (supported formats are %s)", strings.Join(output.SupportedFormats(), ", "))
	RootCmd.PersistentFlags().StringP("output", "o", output.DefaultFormat, outputDescription)
	initSubCommands()
}

func initSubCommands() {
	framework := &framework.Impl{
		Bicep: &bicep.Impl{
			FileSystem: filesystem.OSFileSystem{},
			Output:     &output.OutputWriter{Writer: RootCmd.OutOrStdout()},
		},
		ConnectionFactory: connections.DefaultFactory,
		ConfigHolder:      ConfigHolder,
		Deploy:            &deploy.Impl{},
		Logstream:         &logstream.Impl{},
		Output: &output.OutputWriter{
			Writer: RootCmd.OutOrStdout(),
		},
		Portforward:         &portforward.Impl{},
		Prompter:            &prompt.Impl{},
		ConfigFileInterface: &framework.ConfigFileInterfaceImpl{},
		KubernetesInterface: &kubernetes.Impl{},
		HelmInterface: &helm.Impl{
			Helm: helm.NewHelmClient(),
		},
		NamespaceInterface: &namespace.Impl{},
		AWSClient:          aws.NewClient(),
		AzureClient:        azure.NewClient(),
	}

	deployCmd, _ := cmd_deploy.NewCommand(framework)
	RootCmd.AddCommand(deployCmd)

	runCmd, _ := run.NewCommand(framework)
	RootCmd.AddCommand(runCmd)

	resourceShowCmd, _ := resource_show.NewCommand(framework)
	resourceCmd.AddCommand(resourceShowCmd)

	resourceListCmd, _ := resource_list.NewCommand(framework)
	resourceCmd.AddCommand(resourceListCmd)

	resourceCreateCmd, _ := resource_create.NewCommand(framework)
	resourceCmd.AddCommand(resourceCreateCmd)

	resourceDeleteCmd, _ := resource_delete.NewCommand(framework)
	resourceCmd.AddCommand(resourceDeleteCmd)

	resourceProviderShowCmd, _ := resourceprovider_show.NewCommand(framework)
	resourceProviderCmd.AddCommand(resourceProviderShowCmd)

	resourceProviderListCmd, _ := resourceprovider_list.NewCommand(framework)
	resourceProviderCmd.AddCommand(resourceProviderListCmd)

	resourceProviderCreateCmd, _ := resourceprovider_create.NewCommand(framework)
	resourceProviderCmd.AddCommand(resourceProviderCreateCmd)

	resourceProviderDeleteCmd, _ := resourceprovider_delete.NewCommand(framework)
	resourceProviderCmd.AddCommand(resourceProviderDeleteCmd)

	resourceTypeShowCmd, _ := resourcetype_show.NewCommand(framework)
	resourceTypeCmd.AddCommand(resourceTypeShowCmd)

	resourceTypeListCmd, _ := resourcetype_list.NewCommand(framework)
	resourceTypeCmd.AddCommand(resourceTypeListCmd)

	resourceTypeDeleteCmd, _ := resourcetype_delete.NewCommand(framework)
	resourceTypeCmd.AddCommand(resourceTypeDeleteCmd)

	resourceTypeCreateCmd, _ := resourcetype_create.NewCommand(framework)
	resourceTypeCmd.AddCommand(resourceTypeCreateCmd)

	listRecipeCmd, _ := recipe_list.NewCommand(framework)
	recipeCmd.AddCommand(listRecipeCmd)

	registerRecipeCmd, _ := recipe_register.NewCommand(framework)
	recipeCmd.AddCommand(registerRecipeCmd)

	showRecipeCmd, _ := recipe_show.NewCommand(framework)
	recipeCmd.AddCommand(showRecipeCmd)

	unregisterRecipeCmd, _ := recipe_unregister.NewCommand(framework)
	recipeCmd.AddCommand(unregisterRecipeCmd)

	providerCmd := credential.NewCommand(framework)
	RootCmd.AddCommand(providerCmd)

	groupCmd := group.NewCommand(framework)
	RootCmd.AddCommand(groupCmd)

	initCmd, _ := radinit.NewCommand(framework)
	RootCmd.AddCommand(initCmd)

	envCreateCmd, _ := env_create.NewCommand(framework)
	envCmd.AddCommand(envCreateCmd)

	envDeleteCmd, _ := env_delete.NewCommand(framework)
	envCmd.AddCommand(envDeleteCmd)

	envListCmd, _ := env_list.NewCommand(framework)
	envCmd.AddCommand(envListCmd)

	envShowCmd, _ := env_show.NewCommand(framework)
	envCmd.AddCommand(envShowCmd)

	envUpdateCmd, _ := env_update.NewCommand(framework)
	envCmd.AddCommand(envUpdateCmd)

	workspaceCreateCmd, _ := workspace_create.NewCommand(framework)
	workspaceCmd.AddCommand(workspaceCreateCmd)

	workspaceDeleteCmd, _ := workspace_delete.NewCommand(framework)
	workspaceCmd.AddCommand(workspaceDeleteCmd)

	workspaceListCmd, _ := workspace_list.NewCommand(framework)
	workspaceCmd.AddCommand(workspaceListCmd)

	workspaceShowCmd, _ := workspace_show.NewCommand(framework)
	workspaceCmd.AddCommand(workspaceShowCmd)

	workspaceSwitchCmd, _ := workspace_switch.NewCommand(framework)
	workspaceCmd.AddCommand(workspaceSwitchCmd)

	appDeleteCmd, _ := app_delete.NewCommand(framework)
	applicationCmd.AddCommand(appDeleteCmd)

	appListCmd, _ := app_list.NewCommand(framework)
	applicationCmd.AddCommand(appListCmd)

	appShowCmd, _ := app_show.NewCommand(framework)
	applicationCmd.AddCommand(appShowCmd)

	appStatusCmd, _ := app_status.NewCommand(framework)
	applicationCmd.AddCommand(appStatusCmd)

	appGraphCmd, _ := app_graph.NewCommand(framework)
	applicationCmd.AddCommand(appGraphCmd)

	envSwitchCmd, _ := env_switch.NewCommand(framework)
	envCmd.AddCommand(envSwitchCmd)

	bicepPublishCmd, _ := bicep_publish.NewCommand(framework)
	bicepCmd.AddCommand(bicepPublishCmd)

	bicepGenerateKubernetesManifestCmd, _ := bicep_generate_kubernetes_manifest.NewCommand(framework)
	bicepCmd.AddCommand(bicepGenerateKubernetesManifestCmd)

	bicepPublishExtensionCmd, _ := bicep_publishextension.NewCommand(framework)
	bicepCmd.AddCommand(bicepPublishExtensionCmd)

	installCmd := install.NewCommand()
	RootCmd.AddCommand(installCmd)

	installKubernetesCmd, _ := install_kubernetes.NewCommand(framework)
	installCmd.AddCommand(installKubernetesCmd)

	uninstallCmd := uninstall.NewCommand()
	RootCmd.AddCommand(uninstallCmd)

	uninstallKubernetesCmd, _ := uninstall_kubernetes.NewCommand(framework)
	uninstallCmd.AddCommand(uninstallKubernetesCmd)

	versionCmd, _ := version.NewCommand(framework)
	RootCmd.AddCommand(versionCmd)

	upgradeCmd := upgrade.NewCommand()
	RootCmd.AddCommand(upgradeCmd)

	upgradeKubernetesCmd, _ := upgrade_kubernetes.NewCommand(framework)
	upgradeCmd.AddCommand(upgradeKubernetesCmd)
}

// The dance we do with config is kinda complex. We want commands to be able to retrieve a config (*viper.Viper)
// from context. However we need to initialize the context before we can read the config (before argument parsing).
//
// The solution is a double-indirection. We add a "ConfigHolder" to the context, and then initialize it later. This
// way the context is still immutable, but we can add the config when we're ready to (before any command runs).

func initConfig() {
	v, err := cli.LoadConfig(ConfigHolder.ConfigFilePath)
	if err != nil {
		fmt.Printf("Error: failed to load config: %v\n", err)
		os.Exit(1) //nolint:forbidigo // this is OK inside the CLI startup.
	}

	ConfigHolder.Config = v

	wd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error: failed to find current working directory: %v\n", err)
		os.Exit(1) //nolint:forbidigo // this is OK inside the CLI startup.
	}

	dc, err := config.LoadDirectoryConfig(wd)
	if err != nil {
		fmt.Printf("Error: failed to load config: %v\n", err)
		os.Exit(1) //nolint:forbidigo // this is OK inside the CLI startup.
	}

	ConfigHolder.DirectoryConfig = dc
}

// TODO: Deprecate once all the commands are moved to new framework
func ConfigFromContext(ctx context.Context) *viper.Viper {
	holder := ctx.Value(framework.NewContextKey("config")).(*framework.ConfigHolder)
	if holder == nil {
		return nil
	}

	return holder.Config
}

// TODO: Deprecate once all the commands are moved to new framework
func DirectoryConfigFromContext(ctx context.Context) *config.DirectoryConfig {
	holder := ctx.Value(framework.NewContextKey("config")).(*framework.ConfigHolder)
	if holder == nil {
		return nil
	}

	return holder.DirectoryConfig
}

func getRootSpanName() string {
	args := os.Args
	if len(args) > 1 {
		return args[0] + " " + args[1]
	} else {
		return args[0]
	}
}
