// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/bicep"
	app_switch "github.com/project-radius/radius/pkg/cli/cmd/app/appswitch"
	app_delete "github.com/project-radius/radius/pkg/cli/cmd/app/delete"
	app_list "github.com/project-radius/radius/pkg/cli/cmd/app/list"
	app_show "github.com/project-radius/radius/pkg/cli/cmd/app/show"
	app_status "github.com/project-radius/radius/pkg/cli/cmd/app/status"
	credential "github.com/project-radius/radius/pkg/cli/cmd/credential"
	cmd_deploy "github.com/project-radius/radius/pkg/cli/cmd/deploy"
	env_create "github.com/project-radius/radius/pkg/cli/cmd/env/create"
	env_delete "github.com/project-radius/radius/pkg/cli/cmd/env/delete"
	env_switch "github.com/project-radius/radius/pkg/cli/cmd/env/envswitch"
	env_list "github.com/project-radius/radius/pkg/cli/cmd/env/list"
	"github.com/project-radius/radius/pkg/cli/cmd/env/namespace"
	env_show "github.com/project-radius/radius/pkg/cli/cmd/env/show"
	env_update "github.com/project-radius/radius/pkg/cli/cmd/env/update"
	group "github.com/project-radius/radius/pkg/cli/cmd/group"
	recipe_list "github.com/project-radius/radius/pkg/cli/cmd/recipe/list"
	recipe_register "github.com/project-radius/radius/pkg/cli/cmd/recipe/register"
	recipe_show "github.com/project-radius/radius/pkg/cli/cmd/recipe/show"
	recipe_unregister "github.com/project-radius/radius/pkg/cli/cmd/recipe/unregister"
	resource_delete "github.com/project-radius/radius/pkg/cli/cmd/resource/delete"
	resource_list "github.com/project-radius/radius/pkg/cli/cmd/resource/list"
	resource_show "github.com/project-radius/radius/pkg/cli/cmd/resource/show"
	"github.com/project-radius/radius/pkg/cli/cmd/run"
	workspace_create "github.com/project-radius/radius/pkg/cli/cmd/workspace/create"
	workspace_delete "github.com/project-radius/radius/pkg/cli/cmd/workspace/delete"
	workspace_list "github.com/project-radius/radius/pkg/cli/cmd/workspace/list"
	workspace_show "github.com/project-radius/radius/pkg/cli/cmd/workspace/show"
	workspace_switch "github.com/project-radius/radius/pkg/cli/cmd/workspace/switch"
	"github.com/project-radius/radius/pkg/cli/config"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/deploy"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/kubernetes/logstream"
	"github.com/project-radius/radius/pkg/cli/kubernetes/portforward"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/cli/cmd/radinit"
	"github.com/project-radius/radius/pkg/trace"
	"go.opentelemetry.io/otel"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RootCmd is the root command of the rad CLI. This is exported so we can generate docs for it.
var RootCmd = &cobra.Command{
	Use:           "rad",
	Short:         "Project Radius CLI",
	Long:          `Project Radius CLI`,
	SilenceErrors: true,
	SilenceUsage:  true,
}

const (
	serviceName string = "cli"
	tracerName  string = "cli"
)

var applicationCmd = NewAppCommand()
var resourceCmd = NewResourceCommand()
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
func Execute() {
	ctx := context.WithValue(context.Background(), ConfigHolderKey, ConfigHolder)

	shutdown, err := trace.InitTracer(trace.Options{
		ServiceName: serviceName,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = shutdown(ctx)
	}()

	tr := otel.Tracer(tracerName)
	spanName := getRootSpanName()
	ctx, span := tr.Start(ctx, spanName)
	defer span.End()
	err = RootCmd.ExecuteContext(ctx)
	if errors.Is(&cli.FriendlyError{}, err) {
		fmt.Println(err.Error())
		fmt.Println("\nTraceId: ", span.SpanContext().TraceID().String())
		os.Exit(1)
	} else if err != nil {
		fmt.Println("Error:", prettyPrintRPError(err))
		fmt.Println("\nTraceId: ", span.SpanContext().TraceID().String())
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringVar(&ConfigHolder.ConfigFilePath, "config", "", "config file (default \"$HOME/.rad/config.yaml\")")

	outputDescription := fmt.Sprintf("output format (supported formats are %s)", strings.Join(output.SupportedFormats(), ", "))
	RootCmd.PersistentFlags().StringP("output", "o", output.DefaultFormat, outputDescription)
	initSubCommands()
}

func initSubCommands() {
	framework := &framework.Impl{
		Bicep:             &bicep.Impl{},
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
		HelmInterface:       &helm.Impl{},
		NamespaceInterface:  &namespace.Impl{},
		SetupInterface:      &setup.Impl{},
	}

	deployCmd, _ := cmd_deploy.NewCommand(framework)
	RootCmd.AddCommand(deployCmd)

	runCmd, _ := run.NewCommand(framework)
	RootCmd.AddCommand(runCmd)

	showCmd, _ := resource_show.NewCommand(framework)
	resourceCmd.AddCommand(showCmd)

	listCmd, _ := resource_list.NewCommand(framework)
	resourceCmd.AddCommand(listCmd)

	deleteCmd, _ := resource_delete.NewCommand(framework)
	resourceCmd.AddCommand(deleteCmd)

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

	appSwitchCmd, _ := app_switch.NewCommand(framework)
	applicationCmd.AddCommand(appSwitchCmd)

	envSwitchCmd, _ := env_switch.NewCommand(framework)
	envCmd.AddCommand(envSwitchCmd)
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
		os.Exit(1)
	}

	ConfigHolder.Config = v

	wd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error: failed to find current working directory: %v\n", err)
		os.Exit(1)
	}

	dc, err := config.LoadDirectoryConfig(wd)
	if err != nil {
		fmt.Printf("Error: failed to load config: %v\n", err)
		os.Exit(1)
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
