package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/filesystem"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
)

const (
	deploymentTemplateRepositoryField = "spec.repository"
	radiusConfigFileName              = "radius-gitops-config.yaml"
	armJSONParametersKeyName          = "parameters"
)

// FluxController watches GitRepository objects for revision changes
// and processes the artifacts fetched from the Source Controller.
// It reads the git repository configuration, builds the bicep files.
// specified in the configuration, and creates DeploymentTemplate objects
// on the cluster.
type FluxController struct {
	client.Client
	Bicep          bicep.Interface
	FileSystem     filesystem.FileSystem
	ArchiveFetcher ArchiveFetcher
	initialized    *atomic.Bool // Track if we've initialized the controller
}

// RadiusGitOpsConfig is the configuration for Radius in a Git repository.
type RadiusGitOpsConfig struct {
	Config []ConfigEntry `yaml:"config"`
}

// ConfigEntry is the build configuration for a Bicep file in a Git repository.
type ConfigEntry struct {
	// Name is the name of the Bicep (.bicep) file.
	Name string `yaml:"name"`
	// Params is the name of the Bicep parameters (.bicepparam) file.
	Params string `yaml:"params,omitempty"`
	// Namespace is the Kubernetes namespace that the generated DeploymentTemplate should be created in.
	Namespace string `yaml:"namespace,omitempty"`
	// ResourceGroup is the Radius resource group that the Bicep file should be deployed to.
	ResourceGroup string `yaml:"resourceGroup,omitempty"`
}

// deploymentTemplateRepositoryIndexer indexes DeploymentTemplate objects by their repository field.
func deploymentTemplateRepositoryIndexer(o client.Object) []string {
	deploymentTemplate, ok := o.(*radappiov1alpha3.DeploymentTemplate)
	if !ok {
		return nil
	}
	return []string{deploymentTemplate.Spec.Repository}
}

func (r *FluxController) SetupWithManager(mgr ctrl.Manager) error {
	r.initialized = &atomic.Bool{}

	// Register a controller for CustomResourceDefinition events.
	// We want to watch for the GitRepository CRD to be created or updated.
	err := ctrl.NewControllerManagedBy(mgr).
		For(&apiextensionsv1.CustomResourceDefinition{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return r.registerFluxController(e.Object, mgr)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return r.registerFluxController(e.ObjectNew, mgr)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		}).
		Complete(r)

	return err
}

// registerFluxController registers the FluxController with the manager
// when the GitRepository CRD is created or updated.
func (r *FluxController) registerFluxController(obj client.Object, mgr ctrl.Manager) bool {
	// Only process if:
	// 1. It's the GitRepository CRD
	// 2. The controller isn't already initialized
	// 3. The CRD is established (fully registered with the API server)
	if !isGitRepositoryCRD(obj) || r.initialized.Load() {
		return false
	}

	// Check if the CRD is established before setting up the controller
	crd := obj.(*apiextensionsv1.CustomResourceDefinition)
	for _, condition := range crd.Status.Conditions {
		if condition.Type == apiextensionsv1.Established &&
			condition.Status == apiextensionsv1.ConditionTrue {
			// CRD is established, set up the GitRepository controller
			err := r.setupFluxController(mgr)
			if err != nil {
				ucplog.FromContextOrDiscard(context.Background()).Error(
					err, "failed to setup GitRepository controller")
			}
			return false // We don't need to reconcile CRDs
		}
	}

	return false // Not established yet
}

// setupFluxController creates a new controller for GitRepository objects
// and sets it up with the manager.
func (r *FluxController) setupFluxController(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &radappiov1alpha3.DeploymentTemplate{}, deploymentTemplateRepositoryField, deploymentTemplateRepositoryIndexer); err != nil {
		return err
	}

	err := ctrl.NewControllerManagedBy(mgr).
		For(&sourcev1.GitRepository{}, builder.WithPredicates(&GitRepositoryRevisionChangePredicate{})).
		Complete(r)

	if err != nil {
		return err
	}

	r.initialized.Store(true)
	return nil
}

// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories,verbs=get;list;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories/status,verbs=get

func (r *FluxController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("kind", "FluxController", "name", req.Name, "namespace", req.Namespace)
	ctx = logr.NewContext(ctx, logger)

	// Get the GitRepository object from the cluster
	var repository sourcev1.GitRepository
	if err := r.Get(ctx, req.NamespacedName, &repository); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the Artifact field is set
	artifact := repository.Status.Artifact
	if artifact == nil {
		logger.Info("No artifact found for GitRepository", "name", repository.Name)
		return ctrl.Result{}, nil
	}

	logger.Info("New revision detected", "revision", artifact.Revision)

	// Create temp dir to store the fetched artifact
	tmpDir, err := r.FileSystem.MkdirTemp("", repository.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create temp dir, error: %w", err)
	}

	defer func(path string) {
		err := r.FileSystem.RemoveAll(path)
		if err != nil {
			logger.Error(err, "unable to remove temp dir")
		}
	}(tmpDir)

	// Fetch the artifact from the Source Controller
	logger.Info("Fetching artifact", "url", artifact.URL)
	err = r.ArchiveFetcher.Fetch(artifact.URL, artifact.Digest, tmpDir)
	if err != nil {
		logger.Error(err, "Failed to fetch artifact", "url", artifact.URL)
		return ctrl.Result{}, fmt.Errorf("failed to fetch artifact, error: %w", err)
	}

	logger.Info("Successfully fetched artifact", "url", artifact.URL)

	// Check if the radius-gitops-config.yaml file exists
	_, err = r.FileSystem.Stat(filepath.Join(tmpDir, radiusConfigFileName))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// No radius-gitops-config.yaml found in the repository, safe to ignore
			logger.Info(fmt.Sprintf("No radius-gitops-config.yaml found in the repository: %s", repository.Name))
			return ctrl.Result{}, nil
		} else {
			logger.Error(err, "failed to check if radius-gitops-config.yaml exists")
			return ctrl.Result{}, fmt.Errorf("failed to check if radius-gitops-config.yaml exists, error: %w", err)
		}
	}

	// Parse the radius-gitops-config.yaml file
	radiusConfig, err := r.parseAndValidateRadiusGitOpsConfigFromFile(tmpDir, radiusConfigFileName)
	if err != nil {
		logger.Error(err, "failed to parse radius-gitops-config.yaml")
		return ctrl.Result{}, err
	}

	// Run bicep build on all bicep files specified in radius-gitops-config.yaml
	for _, bicepFile := range radiusConfig.Config {
		fileName := bicepFile.Name
		paramFileName := bicepFile.Params
		namespace := bicepFile.Namespace
		nameBase := strings.TrimSuffix(fileName, path.Ext(fileName))

		if namespace == "" {
			// If the namespace is not set, use the name of the bicep file
			// (without extension) as the namespace. e.g. "example.bicep" -> "example"
			namespace = nameBase
		}
		resourceGroup := bicepFile.ResourceGroup
		if resourceGroup == "" {
			// If the resource group is not set, use the name of the bicep file
			// (without extension) as the resource group. e.g. "example.bicep" -> "example"
			resourceGroup = nameBase
		}

		// Run bicep build on the bicep file
		logger.Info("Running bicep build", "name", fileName)
		template, err := r.runBicepBuild(ctx, tmpDir, fileName)
		if err != nil {
			logger.Error(err, "failed to run bicep build")
			return ctrl.Result{}, err
		}

		// If the bicepparams file is specified, run bicep build-params on it
		var armJSONParameters map[string]any
		if paramFileName != "" {
			logger.Info("Running bicep build-params", "name", paramFileName)
			armJSONParameters, err = r.runBicepBuildParams(ctx, tmpDir, paramFileName)
			if err != nil {
				logger.Error(err, "failed to run bicep build-params")
				return ctrl.Result{}, err
			}
		}

		// Generate the provider config from the radius-gitops-config.yaml file
		providerConfig := sdkclients.GenerateProviderConfig(resourceGroup, "", "")
		marshalledProviderConfig, err := json.MarshalIndent(providerConfig, "", "  ")
		if err != nil {
			return ctrl.Result{}, err
		}

		// Create the namespace if it doesn't exist
		namespaceObj := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}

		if err := r.Client.Create(ctx, namespaceObj); err != nil {
			if !k8serrors.IsAlreadyExists(err) {
				logger.Error(err, "unable to create namespace")
				return ctrl.Result{}, err
			}
		}

		// Now we should create (or update) each DeploymentTemplate for the bicep files
		// specified in the git repository.
		logger.Info("Creating or updating DeploymentTemplate", "name", bicepFile.Name)
		parameters := convertFromARMJSONParameters(armJSONParameters)
		err = r.createOrUpdateDeploymentTemplate(ctx, bicepFile.Name, namespace, template, string(marshalledProviderConfig), parameters, repository.Name)
		if err != nil {
			logger.Error(err, "failed to create or update deployment template")
			return ctrl.Result{}, err
		}

		logger.Info("Successfully created or updated DeploymentTemplate", "name", bicepFile.Name)
	}

	// List all DeploymentTemplates on the cluster that are from the same git repository
	deploymentTemplates := &radappiov1alpha3.DeploymentTemplateList{}
	err = r.Client.List(ctx, deploymentTemplates, client.MatchingFields{deploymentTemplateRepositoryField: repository.Name}, client.InNamespace(""))
	if err != nil {
		logger.Error(err, "unable to list deployment templates")
		return ctrl.Result{}, err
	}

	logger.Info("Found DeploymentTemplates", "count", len(deploymentTemplates.Items))

	// For all of the DeploymentTemplates on the cluster, check if the bicep file
	// that it was created from is still present in config. If not, delete the DeploymentTemplate.
	for _, deploymentTemplate := range deploymentTemplates.Items {
		if !isSpecifiedInConfig(deploymentTemplate.Name, radiusConfig.Config) {
			// The DeploymentTemplate is not specified in the config, so we should delete it
			logger.Info("Deleting DeploymentTemplate", "name", deploymentTemplate.Name)
			if err := r.Client.Delete(ctx, &deploymentTemplate); err != nil {
				logger.Error(err, "unable to delete deployment template")
				return ctrl.Result{}, err
			}

			logger.Info("Deleted DeploymentTemplate", "name", deploymentTemplate.Name)
		}
	}

	return ctrl.Result{}, nil
}

func (r *FluxController) runBicepBuild(ctx context.Context, filepath, filename string) (armJSON string, err error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	bicepFile := path.Join(filepath, filename)
	outFile := path.Join(filepath, strings.ReplaceAll(filename, ".bicep", ".json"))

	// Run bicep build on the bicep file
	logger.Info(fmt.Sprintf("Running command: bicep build %s --outfile %s", bicepFile, outFile))
	_, err = r.Bicep.Call("build", bicepFile, "--outfile", outFile)
	if err != nil {
		logger.Error(err, "failed to run bicep build")
		return "", err
	}

	// Read the contents of the generated .json file
	contents, err := r.FileSystem.ReadFile(outFile)
	if err != nil {
		logger.Error(err, "failed to read bicep build output")
		return "", err
	}

	return string(contents), nil
}

func (r *FluxController) runBicepBuildParams(ctx context.Context, filepath, filename string) (map[string]any, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	bicepParamsFile := path.Join(filepath, filename)
	outfile := path.Join(filepath, strings.ReplaceAll(filename, ".bicepparam", ".parameters.json"))

	// Run bicep build-params on the bicep file
	logger.Info("Running bicep build-params on " + bicepParamsFile)
	_, err := r.Bicep.Call("build-params", bicepParamsFile, "--outfile", outfile)
	if err != nil {
		logger.Error(err, "failed to run bicep build-params")
		return map[string]any{}, err
	}

	// Read the contents of the generated .parameters.json file
	contents, err := r.FileSystem.ReadFile(outfile)
	if err != nil {
		logger.Error(err, "failed to read bicep build-params output")
		return nil, err
	}

	params := make(map[string]any)
	err = json.Unmarshal(contents, &params)
	if err != nil {
		logger.Error(err, "failed to unmarshal bicep build-params output")
		return nil, err
	}

	if params[armJSONParametersKeyName] == nil {
		return nil, fmt.Errorf("parameters field not found in bicep build-params output")
	}

	if _, ok := params[armJSONParametersKeyName].(map[string]any); !ok {
		typeGot := fmt.Sprintf("%T", params[armJSONParametersKeyName])
		return nil, fmt.Errorf("unexpected format for parameters field in bicep build-params output, got %s", typeGot)
	}

	parameters := params[armJSONParametersKeyName].(map[string]any)

	return parameters, nil
}

// createOrUpdateDeploymentTemplate creates or updates a DeploymentTemplate object in the cluster
// with the given spec.
func (r *FluxController) createOrUpdateDeploymentTemplate(ctx context.Context, fileName, namespace, template, providerConfig string, parameters map[string]string, repository string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Try to get the DeploymentTemplate object from the cluster
	deploymentTemplate := radappiov1alpha3.DeploymentTemplate{}
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: fileName}, &deploymentTemplate)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			// Error getting the DeploymentTemplate object that is not a NotFound error
			logger.Error(err, "unable to get deployment template")
			return err
		}

		// If the DeploymentTemplate doesn't exist, create it
		deploymentTemplate := &radappiov1alpha3.DeploymentTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fileName,
				Namespace: namespace,
			},
			Spec: radappiov1alpha3.DeploymentTemplateSpec{
				Template:       template,
				Parameters:     parameters,
				ProviderConfig: providerConfig,
				Repository:     repository,
			},
		}
		if err := r.Client.Create(ctx, deploymentTemplate); err != nil {
			logger.Error(err, "unable to create deployment template")
			return err
		}

		logger.Info("Created Deployment Template", "name", deploymentTemplate.Name)
		return nil
	}

	// If the DeploymentTemplate already exists, update it
	deploymentTemplate.Spec = radappiov1alpha3.DeploymentTemplateSpec{
		Template:       template,
		Parameters:     parameters,
		ProviderConfig: providerConfig,
		Repository:     repository,
	}
	if err := r.Client.Update(ctx, &deploymentTemplate); err != nil {
		logger.Error(err, "unable to update deployment template")
		return err
	}

	logger.Info("Updated Deployment Template", "name", deploymentTemplate.Name)
	return nil
}

// parseAndValidateRadiusGitOpsConfigFromFile reads the radius-gitops-config.yaml file from the given directory
// and parses it into a RadiusGitOpsConfig struct. It then validates the Radius configuration in the
// radius-gitops-config.yaml file.
func (r *FluxController) parseAndValidateRadiusGitOpsConfigFromFile(dir, configFileName string) (*RadiusGitOpsConfig, error) {
	radiusConfig := RadiusGitOpsConfig{}

	// Read the file contents
	b, err := r.FileSystem.ReadFile(path.Join(dir, configFileName))
	if err != nil {
		return nil, fmt.Errorf("failed to read radius-gitops-config.yaml, error: %w", err)
	}

	// Unmarshal the file contents into the RadiusGitOpsConfig struct
	err = yaml.Unmarshal(b, &radiusConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse radius-gitops-config.yaml, error: %w", err)
	}

	// Validate the Radius configuration in radius-gitops-config.yaml
	for _, bicepFile := range radiusConfig.Config {
		// Validate if the Name field is set
		if bicepFile.Name == "" {
			return nil, fmt.Errorf("name field is required in bicepBuild")
		}

		// Validate that the file extension is .bicep
		if path.Ext(bicepFile.Name) != ".bicep" {
			return nil, fmt.Errorf("bicep file must have a .bicep extension")
		}

		// Validate that the file exists
		_, err := r.FileSystem.Stat(path.Join(dir, bicepFile.Name))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("failed to find bicep file %s, error: %w", bicepFile.Name, err)
			} else {
				return nil, fmt.Errorf("failed to check if bicep file exists, error: %w", err)
			}
		}

		// If the bicepFile.Params field is set, validate that the file exists
		if bicepFile.Params != "" {
			_, err := r.FileSystem.Stat(path.Join(dir, bicepFile.Params))
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil, fmt.Errorf("failed to find bicepparams file %s, error: %w", bicepFile.Params, err)
				} else {
					return nil, fmt.Errorf("failed to check if bicepparams file exists, error: %w", err)
				}
			}
		}
	}
	return &radiusConfig, nil
}

func isSpecifiedInConfig(fileName string, config []ConfigEntry) bool {
	for _, bicepFile := range config {
		if bicepFile.Name == fileName {
			return true
		}
	}

	return false
}

// isGitRepositoryCRD checks if obj is a source.toolkit.fluxcd.io/v1/GitRepository CustomResourceDefinition
func isGitRepositoryCRD(obj client.Object) bool {
	crd, ok := obj.(*apiextensionsv1.CustomResourceDefinition)
	if !ok {
		return false
	}

	// Check if this is the GitRepository CRD
	return crd.Spec.Group == "source.toolkit.fluxcd.io" &&
		containsVersion(crd.Spec.Versions, "v1") &&
		crd.Spec.Names.Kind == "GitRepository"
}

// containsVersion checks if the version list contains the target version
func containsVersion(versions []apiextensionsv1.CustomResourceDefinitionVersion, targetVersion string) bool {
	for _, v := range versions {
		if v.Name == targetVersion {
			return true
		}
	}
	return false
}
