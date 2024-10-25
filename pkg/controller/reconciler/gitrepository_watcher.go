package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/martian/log"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fluxcd/pkg/http/fetch"
	"github.com/fluxcd/pkg/tar"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
)

const (
	repositoryField = "spec.repository"
)

// GitRepositoryWatcher watches GitRepository objects for revision changes
type GitRepositoryWatcher struct {
	client.Client
	artifactFetcher *fetch.ArchiveFetcher
	HttpRetry       int
}

func (r *GitRepositoryWatcher) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &radappiov1alpha3.DeploymentTemplate{}, repositoryField, repositoryIndexer); err != nil {
		return err
	}

	r.artifactFetcher = fetch.New(
		fetch.WithRetries(r.HttpRetry),
		fetch.WithMaxDownloadSize(tar.UnlimitedUntarSize),
		fetch.WithUntar(tar.WithMaxUntarSize(tar.UnlimitedUntarSize)),
		fetch.WithLogger(nil),
	)

	return ctrl.NewControllerManagedBy(mgr).
		For(&sourcev1.GitRepository{}, builder.WithPredicates(GitRepositoryRevisionChangePredicate{})).
		Complete(r)
}

// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories,verbs=get;list;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories/status,verbs=get

func (r *GitRepositoryWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("kind", "GitRepositoryWatcher", "name", req.Name, "namespace", req.Namespace)
	ctx = logr.NewContext(ctx, logger)

	// Get the GitRepository object from the cluster
	var repository sourcev1.GitRepository
	if err := r.Get(ctx, req.NamespacedName, &repository); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	artifact := repository.Status.Artifact
	log.Info("New revision detected", "revision", artifact.Revision)

	// Create temp dir to store the fetched artifact
	tmpDir, err := os.MkdirTemp("", repository.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create temp dir, error: %w", err)
	}

	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			log.Error(err, "unable to remove temp dir")
		}
	}(tmpDir)

	// Fetch the artifact from the Source Controller
	log.Info("fetching artifact...", "url", artifact.URL)
	if err := r.artifactFetcher.Fetch(artifact.URL, artifact.Digest, tmpDir); err != nil {
		log.Error(err, "unable to fetch artifact")
		return ctrl.Result{}, err
	}

	log.Info("fetched artifact", "url", artifact.URL)

	files, err := os.ReadDir(tmpDir)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list files, error: %w", err)
	}

	// TODOWILLSMITH: Where to get ProviderConfig def?
	var config sdkclients.ProviderConfig

	config.Radius = &sdkclients.Radius{
		Type: "Radius",
		Value: sdkclients.Value{
			Scope: "/planes/radius/local/resourceGroups/default",
		},
	}
	config.Deployments = &sdkclients.Deployments{
		Type: "Microsoft.Resources",
		Value: sdkclients.Value{
			Scope: "/planes/radius/local/resourceGroups/default",
		},
	}

	providerConfig, err := json.Marshal(config)
	if err != nil {
		log.Error(err, "failed to run bicep build-params")
		return ctrl.Result{}, err
	}

	// Run bicep build on all root bicep files
	for _, f := range files {
		extension := path.Ext(f.Name())
		if extension == ".bicep" {
			fileNameBase := strings.TrimSuffix(f.Name(), path.Ext(f.Name()))
			deploymentTemplateName := repository.Name + "-" + fileNameBase

			template, err := r.runBicepBuild(ctx, tmpDir, f.Name())
			if err != nil {
				log.Error(err, "failed to run bicep build")
				return ctrl.Result{}, err
			}

			// Run bicep build-params on the bicepparams that matches the bicep file
			// e.g. if the bicep file is main.bicep, the bicepparams file should be main.bicepparam
			parameters := "{}"
			parametersFileName := fileNameBase + ".bicepparam"

			// If the bicepparams file exists, run bicep build-params. Otherwise, use the
			// default (empty) parameters.
			if _, err := os.Stat(path.Join(tmpDir, parametersFileName)); err == nil {
				parameters, err = r.runBicepBuildParams(ctx, tmpDir, parametersFileName)
				if err != nil {
					log.Error(err, "failed to run bicep build-params")
					return ctrl.Result{}, err
				}
			}

			// Now we should create (or update) each DeploymentTemplate for the bicep files
			// specified in the git repository.

			// Create or update the deployment template.
			log.Info("Creating or updating Deployment Template", "name", deploymentTemplateName)
			r.createOrUpdateDeploymentTemplate(ctx, deploymentTemplateName, template, parameters, string(providerConfig), repository.Name)
		}
	}

	// Get all DeploymentTemplates on the cluster that are associated with the git repository.
	deploymentTemplates := &radappiov1alpha3.DeploymentTemplateList{}
	err = r.Client.List(ctx, deploymentTemplates, client.MatchingFields{repositoryField: repository.Name})
	if err != nil {
		log.Error(err, "unable to list deployment templates")
		return ctrl.Result{}, err
	}

	// For all of the DeploymentTemplates on the cluster, check if the bicep file
	// that it was created from still exists in the git repository. If it does not,
	// delete the DeploymentTemplate.
	for _, deploymentTemplate := range deploymentTemplates.Items {
		deploymentTemplateFilename := fmt.Sprintf(strings.TrimPrefix(deploymentTemplate.Name, repository.Name+"-"), ".bicep")
		if _, err := os.Stat(path.Join(tmpDir, deploymentTemplateFilename)); err != nil {
			// File does not exist in the git repository,
			// delete the DeploymentTemplate from the cluster
			log.Info("Deleting DeploymentTemplate", "name", deploymentTemplate.Name)
			if err := r.Client.Delete(ctx, &deploymentTemplate); err != nil {
				log.Error(err, "unable to delete deployment template")
				return ctrl.Result{}, err
			}

			log.Info("Deleted DeploymentTemplate", "name", deploymentTemplate.Name)
		}
	}

	return ctrl.Result{}, nil
}

func repositoryIndexer(o client.Object) []string {
	deploymentTemplate, ok := o.(*radappiov1alpha3.DeploymentTemplate)
	if !ok {
		return nil
	}
	return []string{deploymentTemplate.Spec.Repository}
}

func (r *GitRepositoryWatcher) runBicepBuild(ctx context.Context, filepath, filename string) (armJSON string, err error) {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("kind", "GitRepositoryWatcher", "name", req.Name, "namespace", req.Namespace)
	ctx = logr.NewContext(ctx, logger)

	log.Info("Running bicep build on " + path.Join(filepath, filename))

	outfile := path.Join(filepath, strings.ReplaceAll(filename, ".bicep", ".json"))

	cmd := exec.Command("bicep", "build", path.Join(filepath, filename), "--outfile", outfile)
	cmd.Dir = filepath

	// Run the bicep build command
	err = cmd.Run()
	if err != nil {
		log.Error(err, "failed to run bicep build")
		return "", err
	}

	// Read the contents of the generated .json file
	contents, err := os.ReadFile(outfile)
	if err != nil {
		log.Error(err, "failed to read bicep build output")
		return "", err
	}

	return string(contents), nil
}

func (r *GitRepositoryWatcher) runBicepBuildParams(ctx context.Context, filepath, filename string) (armJSON string, err error) {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("kind", "GitRepositoryWatcher", "name", req.Name, "namespace", req.Namespace)
	ctx = logr.NewContext(ctx, logger)

	log.Info("Running bicep build-params on " + filename)

	outfile := path.Join(filepath, strings.ReplaceAll(filename, ".bicepparam", ".bicepparam.json"))

	cmd := exec.Command("bicep", "build-params", path.Join(filepath, filename), "--outfile", outfile)

	// Run the bicep build-params command
	err = cmd.Run()
	if err != nil {
		log.Error(err, "failed to run bicep build")
		return "", err
	}

	// Read the contents of the generated .bicepparam.json file
	contents, err := os.ReadFile(outfile)
	if err != nil {
		log.Error(err, "failed to read bicep build-params output")
		return "", err
	}

	var params map[string]interface{}
	err = json.Unmarshal(contents, &params)

	if params["parameters"] == nil {
		logger.Info("No parameters found in bicep build-params output")
		return "{}", nil
	}

	specifiedParams, err := json.Marshal(params["parameters"])

	return specifiedParams, nil
}

func (r *GitRepositoryWatcher) createOrUpdateDeploymentTemplate(ctx context.Context, fileName, template, parameters, providerConfig, repository string) {
	log := ctrl.LoggerFrom(ctx)

	deploymentTemplate := &radappiov1alpha3.DeploymentTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fileName,
			Namespace: RadiusSystemNamespace,
		},
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
			Repository:     repository,
		},
	}

	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(deploymentTemplate), deploymentTemplate); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to get deployment template")
			return
		}

		if err := r.Client.Create(ctx, deploymentTemplate); err != nil {
			log.Error(err, "unable to create deployment template")
		}

		log.Info("Created Deployment Template", "name", deploymentTemplate.Name)
		return
	}

	if err := r.Client.Update(ctx, deploymentTemplate); err != nil {
		log.Error(err, "unable to create deployment template")
	}

	log.Info("Updated Deployment Template", "name", deploymentTemplate.Name)
}
