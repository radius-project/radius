package reconciler

import (
	"context"
	"fmt"
	"io/fs"
	"os"

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
		// fetch.WithHostnameOverwrite(os.Getenv("SOURCE_CONTROLLER_LOCALHOST")),
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

	// get source object
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

	// download and extract artifact
	if err := r.artifactFetcher.Fetch(artifact.URL, artifact.Digest, tmpDir); err != nil {
		log.Error(err, "unable to fetch artifact")
		return ctrl.Result{}, err
	}

	// list artifact content
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list files, error: %w", err)
	}

	for _, f := range files {
		r.processFile(ctx, f, tmpDir+"/")
	}

	return ctrl.Result{}, nil
}

func (r *GitRepositoryWatcher) processFile(ctx context.Context, f fs.DirEntry, path string) {
	log := ctrl.LoggerFrom(ctx)

	if f.IsDir() {
		log.Info("Processing Directory " + f.Name())
		files, err := os.ReadDir(path + f.Name())
		if err != nil {
			log.Error(err, "failed to list files, error: %w", err)
		}

		for _, f := range files {
			r.processFile(ctx, f, path+f.Name()+"/")
		}
	} else {
		log.Info("Processing File" + f.Name())
		template, parameters, providerConfig := r.processBicepFile(ctx, path+f.Name())
		deploymentTemplate := &radappiov1alpha3.DeploymentTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      f.Name(),
				Namespace: "radius-system",
			},
			Spec: radappiov1alpha3.DeploymentTemplateSpec{
				Template:       template,
				Parameters:     parameters,
				ProviderConfig: providerConfig,
			},
		}

		if err := r.Create(ctx, deploymentTemplate); err != nil {
			log.Error(err, "unable to create deployment template")
		}

		log.Info("Created Deployment Template", "name", deploymentTemplate.Name)
	}
}

func (r *GitRepositoryWatcher) processBicepFile(ctx context.Context, path string) (string, string, string) {
	log := ctrl.LoggerFrom(ctx)

	_, err := os.ReadFile(path)
	if err != nil {
		log.Error(err, "unable to read file")
		return "", "", ""
	}

	// TODOWILLSMITH: compilebicep

	return "", "", ""
}
