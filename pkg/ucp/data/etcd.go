// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package data

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/ucp/hosting"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	etcdclient "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

const (
	ETCDStartTimeout = time.Second * 60
	ETCDStopTimeout  = time.Second * 10
)

var _ hosting.Service = (*EmbeddedETCDService)(nil)

type EmbeddedETCDServiceOptions struct {
	ClientConfigSink *hosting.AsyncValue
}

type EmbeddedETCDService struct {
	options EmbeddedETCDServiceOptions
	dirs    []string
}

func NewEmbeddedETCDService(options EmbeddedETCDServiceOptions) *EmbeddedETCDService {
	return &EmbeddedETCDService{options: options}
}

func (s *EmbeddedETCDService) Name() string {
	return "etcd"
}

func (s *EmbeddedETCDService) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)
	defer s.cleanup(ctx)

	config := embed.NewConfig()

	// Using temp directories for storage
	dataDir, err := ioutil.TempDir(os.TempDir(), "ucp-etcd-data-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary data directory: %w", err)
	}
	s.dirs = append(s.dirs, dataDir)

	walDir, err := ioutil.TempDir(os.TempDir(), "ucp-etcd-wal-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary wal directory: %w", err)
	}
	s.dirs = append(s.dirs, walDir)

	config.Dir = dataDir
	config.WalDir = walDir

	// If we're using Zap we can just log to it directly. Otherwise send logging
	// to the console.
	zaplog := ucplog.Unwrap(logger)
	if zaplog != nil {
		config.ZapLoggerBuilder = embed.NewZapLoggerBuilder(zaplog.Named("etcd.server"))
	} else {
		config.LogLevel = "info"
		config.LogOutputs = []string{"stdout"}
	}

	// Use generated self-signed certs for authentication.
	config.ClientAutoTLS = true
	config.PeerAutoTLS = true
	config.SelfSignedCertValidity = 1 // One year

	server, err := embed.StartEtcd(config)
	if err != nil {
		return err
	}

	select {
	case <-server.Server.ReadyNotify():
		logger.Info("Started etcd server")
		break
	case <-time.After(ETCDStartTimeout):
		server.Server.Stop() // trigger a shutdown
		s.cleanup(ctx)
		return fmt.Errorf("etcd start timed out after %v", ETCDStartTimeout)
	case err := <-server.Err():
		s.cleanup(ctx)
		return fmt.Errorf("etcd start failed: %w", err)
	}

	clientconfig := etcdclient.Config{
		Endpoints: server.Server.Cluster().ClientURLs(),
	}
	if zaplog != nil {
		clientconfig.Logger = zaplog.Named("etcd.client")
	}

	s.options.ClientConfigSink.Put(&clientconfig)

	<-ctx.Done()

	logger.Info("Stopping etcd")
	server.Server.Stop()

	select {
	case <-server.Server.StopNotify():
		break
	case <-time.After(ETCDStopTimeout):
		server.Server.HardStop()
		return fmt.Errorf("etcd stop timed out after %v", ETCDStopTimeout)
	case err := <-server.Err():
		if err != nil {
			return fmt.Errorf("etcd stop failed: %w", err)
		}
	}

	return nil
}

func (s *EmbeddedETCDService) cleanup(ctx context.Context) {
	logger := logr.FromContextOrDiscard(ctx)
	for _, dir := range s.dirs {
		err := os.RemoveAll(dir)
		if err != nil {
			logger.Error(err, "Failed to delete temp directory", "directory", dir)
		}
	}
}
