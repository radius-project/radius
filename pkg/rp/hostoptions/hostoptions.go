// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// hostoptions defines and reads options for the RP's execution environment.
package hostoptions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/rp/k8sauth"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

// HostOptions defines all of the settings that our RP's execution environment provides.
type HostOptions struct {
	Address         string
	Arm             *armauth.ArmConfig
	Authenticate    bool
	BasePath        string
	DBClientFactory func(ctx context.Context) (*mongo.Database, error)

	// HealthChannels supports loosely-coupled communication between the Resource Provider
	// backend and the Health Monitor. This is part of the options for now because it's
	// based on in-memory communication.
	HealthChannels healthcontract.HealthChannels
	K8sConfig      *rest.Config
	TLSCertDir     string
}

func NewHostOptionsFromEnvironment() (HostOptions, error) {
	port, authenticate, basePath, tlsCertDir, err := getRest()
	if err != nil {
		return HostOptions{}, err
	}

	dbClientFactory, err := getMongo()
	if err != nil {
		return HostOptions{}, err
	}

	arm, err := getArm()
	if err != nil {
		return HostOptions{}, err
	}

	k8s, err := getKubernetes()
	if err != nil {
		return HostOptions{}, err
	}

	return HostOptions{
		Address:         ":" + port,
		Arm:             arm,
		Authenticate:    authenticate,
		BasePath:        basePath,
		DBClientFactory: dbClientFactory,
		HealthChannels:  healthcontract.NewHealthChannels(),
		K8sConfig:       k8s,
		TLSCertDir:      tlsCertDir,
	}, nil
}

func getRest() (string, bool, string, string, error) {
	// App Service uses this env-var to tell us what port to listen on.
	port, ok := os.LookupEnv("PORT")
	if !ok {
		return "", false, "", "", errors.New("env: PORT is required")
	}

	authenticate := true
	skipAuth, ok := os.LookupEnv("SKIP_AUTH")
	if ok && strings.EqualFold(skipAuth, "true") {
		fmt.Println("Authentication will be skipped! This is a development-time only setting")
		authenticate = false
	}

	basePath, ok := os.LookupEnv("BASE_PATH")
	if ok && len(basePath) > 0 && (!strings.HasPrefix(basePath, "/") || strings.HasSuffix(basePath, "/")) {
		return "", false, "", "", errors.New("env: BASE_PATH must begin with '/' and must not end with '/'")
	}

	tlsCertDir := os.Getenv("TLS_CERT_DIR")
	return port, authenticate, basePath, tlsCertDir, nil
}

func getMongo() (func(context.Context) (*mongo.Database, error), error) {
	// Create DB client and connect
	connString, ok := os.LookupEnv("MONGODB_CONNECTION_STRING")
	if !ok {
		return nil, errors.New("env: MONGODB_CONNECTION_STRING is required")
	}

	dbName, ok := os.LookupEnv("MONGODB_DATABASE")
	if !ok {
		return nil, errors.New("env: MONGODB_DATABASE is required")
	}

	// Now we create a temporary connection to verify that it actually works.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// cosmos does not support retrywrites, but it's the default for the golang driver
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(connString).SetRetryWrites(false))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	defer func() {
		_ = client.Disconnect(ctx)
	}()

	err = client.Ping(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to ping the database: %w", err)
	}

	return func(ctx context.Context) (*mongo.Database, error) {
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(connString).SetRetryWrites(false))
		if err != nil {
			return nil, fmt.Errorf("failed to connect to database: %w", err)
		}

		return client.Database(dbName), nil
	}, nil
}

func getArm() (*armauth.ArmConfig, error) {
	skipARM, ok := os.LookupEnv("SKIP_ARM")
	if ok && strings.EqualFold(skipARM, "true") {
		return nil, nil
	}

	arm, err := armauth.GetArmConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build ARM config: %w", err)
	}

	if arm != nil {
		fmt.Println("Initializing RP with the provided ARM credentials")
	}

	return arm, nil
}

func getKubernetes() (*rest.Config, error) {
	cfg, err := k8sauth.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes config: %w", err)
	}

	// Verify that we can connect to the cluster before handing out the config
	s := scheme.Scheme
	c, err := controller_runtime.New(cfg, controller_runtime.Options{Scheme: s})
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	ns := &corev1.NamespaceList{}
	err = c.List(context.Background(), ns, &controller_runtime.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to kubernetes: %w", err)
	}

	return cfg, nil
}
