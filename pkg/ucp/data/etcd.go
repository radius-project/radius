/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package data

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

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
	ClientConfigSink *hosting.AsyncValue[etcdclient.Client]

	// AssignRandomPorts will choose random ports so that each instance of the etcd service
	// is isolated.
	AssignRandomPorts bool

	// Quiet will prevent etcd from logging to the console.
	Quiet bool
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
	logger := ucplog.FromContextOrDiscard(ctx)
	defer s.cleanup(ctx)

	config := embed.NewConfig()

	if s.options.AssignRandomPorts {
		// We need to auto-assign ports to avoid crosstalk when tests create multiple clusters. ETCD uses
		// hardcoded ports by default.
		peerPort, clientPort, err := s.assignPorts(ctx)
		if err != nil {
			return fmt.Errorf("failed to assign listening ports for etcd: %w", err)
		}

		logger.Info(fmt.Sprintf("etcd will listen on ports %d %d", *peerPort, *clientPort))

		config.APUrls = []url.URL{makeURL(*peerPort)}
		config.LPUrls = []url.URL{makeURL(*peerPort)}
		config.ACUrls = []url.URL{makeURL(*clientPort)}
		config.LCUrls = []url.URL{makeURL(*clientPort)}

		// Needs to be updated based on the ports that were chosen
		config.ForceNewCluster = true
		config.InitialCluster = config.InitialClusterFromName("default")
		config.InitialClusterToken = fmt.Sprintf("cluster-%d", clientPort)
	}

	// Using temp directories for storage
	dataDir, err := os.MkdirTemp(os.TempDir(), "ucp-etcd-data-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary data directory: %w", err)
	}
	s.dirs = append(s.dirs, dataDir)

	walDir, err := os.MkdirTemp(os.TempDir(), "ucp-etcd-wal-*")
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
	} else if !s.options.Quiet {
		config.LogLevel = "info"
		config.LogOutputs = []string{"stdout"}
	} else {
		config.LogLevel = "fatal"
		config.LogOutputs = []string{}
	}

	// Use generated self-signed certs for authentication.
	config.ClientAutoTLS = true
	config.PeerAutoTLS = true
	config.SelfSignedCertValidity = 1 // One year

	logger.Info("Starting etcd server")
	server, err := embed.StartEtcd(config)
	if err != nil {
		if strings.HasPrefix(err.Error(), "listen tcp ") {
			logger.Info("failed to start etcd server due to port conflict, assuming another instance of etcd is already running")
			clientconfig := etcdclient.Config{
				Endpoints: []string{
					"http://localhost:2379",
				},
			}
			if zaplog != nil {
				clientconfig.Logger = zaplog.Named("etcd.client")
			}

			client, err := etcdclient.New(clientconfig)
			if err != nil {
				s.options.ClientConfigSink.PutErr(err)
			} else {
				s.options.ClientConfigSink.Put(client)
			}

			<-ctx.Done()
		}

		return err
	}
	logger.Info("Waiting for etcd server ready...")

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

	client, err := etcdclient.New(clientconfig)
	if err != nil {
		s.options.ClientConfigSink.PutErr(err)
	} else {
		s.options.ClientConfigSink.Put(client)
	}

	<-ctx.Done()

	logger.Info("Stopping etcd...")
	server.Close()

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

	logger.Info("Stopped etcd")
	return nil
}

func (s *EmbeddedETCDService) cleanup(ctx context.Context) {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("Cleaning up etcd directories")
	for _, dir := range s.dirs {
		err := os.RemoveAll(dir)
		if err != nil {
			logger.Error(err, "Failed to delete temp directory", "directory", dir)
		}
	}
}

func makeURL(port int) url.URL {
	u, err := url.Parse(fmt.Sprintf("http://localhost:%d", port))
	if err != nil {
		// This should never happen.
		panic(fmt.Sprintf("failed to parse URL: %v", err))
	}

	return *u
}

func (s *EmbeddedETCDService) assignPorts(ctx context.Context) (*int, *int, error) {
	listener1, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, nil, err
	}
	defer listener1.Close()

	listener2, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, nil, err
	}
	defer listener2.Close()

	port1 := listener1.Addr().(*net.TCPAddr).Port
	port2 := listener2.Addr().(*net.TCPAddr).Port

	return &port1, &port2, nil
}
