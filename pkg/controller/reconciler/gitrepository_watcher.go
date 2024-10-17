package reconciler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fluxcd/pkg/http/fetch"
	"github.com/fluxcd/pkg/tar"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
)

// GitRepositoryWatcher watches GitRepository objects for revision changes
type GitRepositoryWatcher struct {
	client.Client
	artifactFetcher *fetch.ArchiveFetcher
	HttpRetry       int
}

func (r *GitRepositoryWatcher) SetupWithManager(mgr ctrl.Manager) error {
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
	log := ctrl.LoggerFrom(ctx)

	var repository sourcev1.GitRepository
	if err := r.Get(ctx, req.NamespacedName, &repository); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	artifact := repository.Status.Artifact
	log.Info("New revision detected", "revision", artifact.Revision)

	// create tmp dir
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

	log.Info("fetching artifact...", "url", artifact.URL)
	if err := r.artifactFetcher.Fetch(artifact.URL, artifact.Digest, tmpDir); err != nil {
		log.Error(err, "unable to fetch artifact")
		return ctrl.Result{}, err
	}

	// list artifact content
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list files, error: %w", err)
	}

	// TODOWILLSMITH: how do we decide which files to run bicep build on?
	// for now, we'll just run it on all root files
	for _, f := range files {
		extension := path.Ext(f.Name())
		if extension == ".bicep" {
			template, err := r.runBicepBuild(ctx, tmpDir, f.Name())
			if err != nil {
				log.Error(err, "failed to run bicep build")
				return ctrl.Result{}, err
			}

			// TODOWILLSMITH: how do we decide which parameters file to use?
			// for now, we assume the parameters file is the same name as the bicep file
			// in the same directory
			// e.g. main.bicep -> main.bicepparam
			parametersFile := strings.ReplaceAll(f.Name(), ".bicep", ".bicepparam")

			parameters, err := r.runBicepBuildParams(ctx, tmpDir, parametersFile)
			providerConfig := "providerConfig"
			if err != nil {
				log.Error(err, "failed to run bicep build-params")
				return ctrl.Result{}, err
			}

			// TODOWILLSMITH: create/update or delete
			// determine if this bicep file has already been deployed, if so update
			// if not, create,
			// if the bicep file has been deleted, delete the deployment template

			// get all deployment templates on the cluster
			// think ab multiple git repos scenario
			// need to save name of git repo in deployment template?

			r.createOrUpdateDeploymentTemplate(ctx, f.Name(), template, parameters, providerConfig)
		}
	}

	return ctrl.Result{}, nil
}

func (r *GitRepositoryWatcher) runBicepBuild(ctx context.Context, filepath, filename string) (armJSON string, err error) {
	// TODOWILLSMITH: bicep build is broken
	log := ctrl.LoggerFrom(ctx)

	log.Info("Running bicep build on " + path.Join(filepath, filename))

	cmd := exec.Command("/work-dir/bicep", "build", path.Join(filepath, filename), "--stdout")
	cmd.Dir = filepath

	stdout, err := cmd.CombinedOutput()
	if err != nil {
		log.Error(err, "failed to run bicep build", "out", string(stdout))
		return "", err
	}

	log.Info("Bicep build output", "output", string(stdout))

	return string(stdout), nil
}

func (r *GitRepositoryWatcher) runBicepBuildParams(ctx context.Context, filepath, filename string) (armJSON string, err error) {
	log := ctrl.LoggerFrom(ctx)

	log.Info("Running bicep build-params on " + filename)

	cmd := exec.Command("/work-dir/bicep", "build-params", path.Join(filepath, filename), "--stdout")

	stdout, err := cmd.Output()
	if err != nil {
		log.Error(err, "failed to run bicep build")
		return "", err
	}

	log.Info("Bicep build output", "output", string(stdout))

	return string(stdout), nil
}

func (r *GitRepositoryWatcher) createOrUpdateDeploymentTemplate(ctx context.Context, fileName, template, parameters, providerConfig string) {
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
