package reconciler

import (
	"github.com/fluxcd/pkg/http/fetch"
	"github.com/fluxcd/pkg/tar"
)

// NewArchiveFetcher creates a new ArchiveFetcher
// with the default options.
func NewArchiveFetcher() ArchiveFetcher {
	archiveFetcher := fetch.New(
		fetch.WithRetries(GitRepositoryHttpRetryCount),
		fetch.WithMaxDownloadSize(tar.UnlimitedUntarSize),
		fetch.WithUntar(tar.WithMaxUntarSize(tar.UnlimitedUntarSize)),
		fetch.WithLogger(nil),
	)

	return &ArchiveFetcherImpl{
		inner: archiveFetcher,
	}
}

type ArchiveFetcher interface {
	Fetch(archiveURL string, digest string, dir string) error
}

type ArchiveFetcherImpl struct {
	inner *fetch.ArchiveFetcher
}

func (a *ArchiveFetcherImpl) Fetch(archiveURL string, digest string, dir string) error {
	return a.inner.Fetch(archiveURL, digest, dir)
}
