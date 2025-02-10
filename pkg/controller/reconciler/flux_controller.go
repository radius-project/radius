package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/filesystem"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
)

const (
	deploymentTemplateRepositoryField = "spec.repository"
	radiusConfigFileName              = "radius-config.yaml"
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
}

// RadiusConfig is the configuration for Radius in a Git repository
// TODO (willsmith): adapt this to .rad/config.yaml format
type RadiusConfig struct {
	RadiusResourceGroup string       `yaml:"radiusResourceGroup,omitempty"`
	AWSScope            string       `yaml:"awsScope,omitempty"`
	AzureScope          string       `yaml:"azureScope,omitempty"`
	BicepBuild          []BicepBuild `yaml:"bicepBuild,omitempty"`
}

// BicepBuild is the build configuration for a Bicep file in a Git repository
type BicepBuild struct {
	Name      string `yaml:"name"`
	Params    string `yaml:"params,omitempty"`
	Namespace string `yaml:"namespace,omitempty"`
}

// deploymentTemplateRepositoryIndexer indexes DeploymentTemplate objects by their repository field
func deploymentTemplateRepositoryIndexer(o client.Object) []string {
	deploymentTemplate, ok := o.(*radappiov1alpha3.DeploymentTemplate)
	if !ok {
		return nil
	}
	return []string{deploymentTemplate.Spec.Repository}
}

func (r *FluxController) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &radappiov1alpha3.DeploymentTemplate{}, deploymentTemplateRepositoryField, deploymentTemplateRepositoryIndexer); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&sourcev1.GitRepository{}, builder.WithPredicates(GitRepositoryRevisionChangePredicate{})).
		Complete(r)
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
	if err := r.ArchiveFetcher.Fetch(artifact.URL, artifact.Digest, tmpDir); err != nil {
		logger.Error(err, "unable to fetch artifact")
		return ctrl.Result{}, err
	}

	logger.Info("Fetched artifact", "url", artifact.URL)

	// Check if the radius-config.yaml file exists
	_, err = r.FileSystem.Stat(filepath.Join(tmpDir, radiusConfigFileName))
	if err != nil {
		if os.IsNotExist(err) {
			// No radius-config.yaml found in the repository, safe to ignore
			logger.Info(fmt.Sprintf("No radius-config.yaml found in the repository: %s", repository.Name))
			return ctrl.Result{}, nil
		} else {
			logger.Error(err, "failed to check if radius-config.yaml exists")
			return ctrl.Result{}, fmt.Errorf("failed to check if radius-config.yaml exists, error: %w", err)
		}
	}

	// Parse the radius-config.yaml file
	radiusConfig, err := r.parseAndValidateRadiusConfigFromFile(tmpDir, radiusConfigFileName)
	if err != nil {
		logger.Error(err, "failed to parse radius-config.yaml")
		return ctrl.Result{}, err
	}

	// Generate the provider config from the radius-config.yaml file
	providerConfig := sdkclients.GenerateProviderConfig(radiusConfig.RadiusResourceGroup, radiusConfig.AWSScope, radiusConfig.AzureScope)
	marshalledProviderConfig, err := json.MarshalIndent(providerConfig, "", "  ")
	if err != nil {
		return ctrl.Result{}, err
	}

	// Run bicep build on all bicep files specified in radius-config.yaml.
	for _, bicepFile := range radiusConfig.BicepBuild {
		fileName := bicepFile.Name
		paramFileName := bicepFile.Params
		namespace := bicepFile.Namespace
		if namespace == "" {
			// If the namespace is not set, use the name of the bicep file (without extension) as
			// the namespace
			namespace = strings.TrimSuffix(fileName, path.Ext(fileName))
		}

		// Run bicep build on the bicep file
		logger.Info("Running bicep build", "name", fileName)
		template, err := r.runBicepBuild(ctx, tmpDir, fileName)
		if err != nil {
			logger.Error(err, "failed to run bicep build")
			return ctrl.Result{}, err
		}

		// If the bicepparams file exists, run bicep build-params on it
		var parameters map[string]string
		if paramFileName != "" {
			if !os.IsNotExist(err) {
				logger.Info("Running bicep build-params", "name", fileName)
				parameters, err = r.runBicepBuildParams(ctx, tmpDir, bicepFile.Name)
				if err != nil {
					logger.Error(err, "failed to run bicep build-params")
					return ctrl.Result{}, err
				}
			} else {
				logger.Error(err, "failed to check if parameters file exists")
				return ctrl.Result{}, err
			}
		}

		// Now we should create (or update) each DeploymentTemplate for the bicep files
		// specified in the git repository.
		logger.Info("Creating or updating DeploymentTemplate", "name", bicepFile.Name)
		r.createOrUpdateDeploymentTemplate(ctx, bicepFile.Name, namespace, template, string(marshalledProviderConfig), parameters)
	}

	// List all DeploymentTemplates on the cluster that are from the same git repository
	deploymentTemplates := &radappiov1alpha3.DeploymentTemplateList{}
	err = r.Client.List(ctx, deploymentTemplates, client.MatchingFields{deploymentTemplateRepositoryField: repository.Name})
	if err != nil {
		logger.Error(err, "unable to list deployment templates")
		return ctrl.Result{}, err
	}

	// For all of the DeploymentTemplates on the cluster, check if the bicep file
	// that it was created from is still present in the git repository. If not, delete the
	// DeploymentTemplate from the cluster.
	for _, deploymentTemplate := range deploymentTemplates.Items {
		if _, err := r.FileSystem.Stat(path.Join(tmpDir, deploymentTemplate.Name)); os.IsNotExist(err) {
			// File does not exist in the git repository,
			// delete the DeploymentTemplate from the cluster
			logger.Info("Deleting DeploymentTemplate", "name", deploymentTemplate.Name)
			if err := r.Client.Delete(ctx, &deploymentTemplate); err != nil {
				logger.Error(err, "unable to delete deployment template")
				return ctrl.Result{}, err
			}

			logger.Info("Deleted DeploymentTemplate", "name", deploymentTemplate.Name)
		} else if err != nil {
			logger.Error(err, "failed to check if file exists")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *FluxController) runBicepBuild(ctx context.Context, filepath, filename string) (armJSON string, err error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	bicepFile := path.Join(filepath, filename)
	outfile := path.Join(filepath, strings.ReplaceAll(filename, ".bicep", ".json"))

	// Run bicep build on the bicep file
	logger.Info("Running bicep build on " + bicepFile)
	r.Bicep.Build(bicepFile, "--outfile", outfile)

	// Read the contents of the generated .json file
	contents, err := r.FileSystem.ReadFile(outfile)
	if err != nil {
		logger.Error(err, "failed to read bicep build output")
		return "", err
	}

	return string(contents), nil
}

func (r *FluxController) runBicepBuildParams(ctx context.Context, filepath, filename string) (map[string]string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	bicepParamsFile := path.Join(filepath, filename)
	outfile := path.Join(filepath, strings.ReplaceAll(filename, ".bicepparam", ".bicepparam.json"))

	// Run bicep build-params on the bicep file
	logger.Info("Running bicep build-params on " + bicepParamsFile)
	r.Bicep.BuildParams(bicepParamsFile, outfile)

	// Read the contents of the generated .bicepparam.json file
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
	if params["parameters"] == nil {
		logger.Info("No parameters found in bicep build-params output")
		return nil, nil
	}

	return nil, nil
}

func (r *FluxController) createOrUpdateDeploymentTemplate(ctx context.Context, fileName, namespace, template, providerConfig string, parameters map[string]string) {
	logger := ucplog.FromContextOrDiscard(ctx)

	deploymentTemplate := radappiov1alpha3.DeploymentTemplate{}
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: fileName}, &deploymentTemplate)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			// Error getting DeploymentTemplate
			logger.Error(err, "unable to get deployment template")
			return
		}

		// If the namespace doesn't exist, create it
		if err := r.Client.Get(ctx, client.ObjectKey{Name: namespace}, &corev1.Namespace{}); err != nil {
			if client.IgnoreNotFound(err) != nil {
				logger.Error(err, "unable to get namespace")
				return
			}

			// Create the namespace if it doesn't exist
			err := r.Client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: namespace}})
			if err != nil {
				logger.Error(err, "unable to create namespace")
				return
			}
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
			},
		}
		if err := r.Client.Create(ctx, deploymentTemplate); err != nil {
			logger.Error(err, "unable to create deployment template")
		}

		logger.Info("Created Deployment Template", "name", deploymentTemplate.Name)
		return
	}

	deploymentTemplate.Spec = radappiov1alpha3.DeploymentTemplateSpec{
		Template:       template,
		Parameters:     parameters,
		ProviderConfig: providerConfig,
	}
	// If the DeploymentTemplate already exists, update it
	if err := r.Client.Update(ctx, &deploymentTemplate); err != nil {
		logger.Error(err, "unable to update deployment template")
	}

	logger.Info("Updated Deployment Template", "name", deploymentTemplate.Name)
}

// parseAndValidateRadiusConfigFromFile reads the radius-config.yaml file from the given directory
// and parses it into a RadiusConfig struct. It then validates the Radius configuration in the
// radius-config.yaml file.
func (r *FluxController) parseAndValidateRadiusConfigFromFile(dir, configFileName string) (*RadiusConfig, error) {
	radiusConfig := RadiusConfig{}

	// Read the file contents
	b, err := r.FileSystem.ReadFile(path.Join(dir, configFileName))
	if err != nil {
		return nil, fmt.Errorf("failed to read radius-config.yaml, error: %w", err)
	}

	// Unmarshal the file contents into the RadiusConfig struct
	err = yaml.Unmarshal(b, &radiusConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse radius-config.yaml, error: %w", err)
	}

	// Validate the Radius configuration in radius-config.yaml
	// Check if the RadiusResourceGroup field is set
	if radiusConfig.RadiusResourceGroup == "" {
		return nil, fmt.Errorf("radiusResourceGroup field is required in radius-config.yaml")
	}

	for _, bicepFile := range radiusConfig.BicepBuild {
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
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to find bicep file %s, error: %w", bicepFile.Name, err)
			} else {
				return nil, fmt.Errorf("failed to check if bicep file exists, error: %w", err)
			}
		}

		// If the bicepFile.Params field is set, validate that the file exists
		if bicepFile.Params != "" {
			_, err := r.FileSystem.Stat(path.Join(dir, bicepFile.Params))
			if err != nil {
				if os.IsNotExist(err) {
					return nil, fmt.Errorf("failed to find bicepparams file %s, error: %w", bicepFile.Params, err)
				} else {
					return nil, fmt.Errorf("failed to check if bicepparams file exists, error: %w", err)
				}
			}
		}
	}
	return &radiusConfig, nil
}
