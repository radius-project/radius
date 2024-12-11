package manifestservice

// Import your logger, server, ucpclient, etc.
// "github.com/yourorg/yourproject/logger"
// "github.com/yourorg/yourproject/server"
// "github.com/yourorg/yourproject/ucpclient"

/*
// Service is a service to run AsyncReqeustProcessWorker.
type Service struct {
	worker.Service

	config ucpoptions.UCPConfig
}

// NewService creates new service instance to run AsyncRequestProcessWorker.
func NewService(options hostoptions.HostOptions, config ucpoptions.UCPConfig) *Service {
	return &Service{
		Service: worker.Service{
			ProviderName: UCPProviderName,
			Options:      options,
		},
		config: config,
	}
}

// Name returns a string containing the UCPProviderName and the text "async worker".
func (w *Service) Name() string {
	return fmt.Sprintf("%s async worker", UCPProviderName)
}

// Run starts the service and worker. It initializes the service and sets the worker options based on the configuration,
// then starts the service with the given worker options. It returns an error if the initialization fails.
func (w *Service) Run(ctx context.Context) error {
	if err := w.Init(ctx); err != nil {
		return err
	}

	workerOpts := worker.Options{}
	if w.Options.Config.WorkerServer != nil {
		if w.Options.Config.WorkerServer.MaxOperationConcurrency != nil {
			workerOpts.MaxOperationConcurrency = *w.Options.Config.WorkerServer.MaxOperationConcurrency
		}
		if w.Options.Config.WorkerServer.MaxOperationRetryCount != nil {
			workerOpts.MaxOperationRetryCount = *w.Options.Config.WorkerServer.MaxOperationRetryCount
		}
	}

	opts := ctrl.Options{
		DataProvider: w.StorageProvider,
	}

	defaultDownstream, err := url.Parse(w.config.Routing.DefaultDownstreamEndpoint)
	if err != nil {
		return err
	}

	transport := otelhttp.NewTransport(http.DefaultTransport)
	err = RegisterControllers(ctx, w.Controllers, w.Options.UCPConnection, transport, opts, defaultDownstream)
	if err != nil {
		return err
	}

	return w.Start(ctx, workerOpts)
}

// RegisterControllers registers the controllers for the UCP backend.
func RegisterControllers(ctx context.Context, registry *worker.ControllerRegistry, connection sdk.Connection, transport http.RoundTripper, opts ctrl.Options, defaultDownstream *url.URL) error {
	// Tracked resources
	err := errors.Join(nil, registry.Register(ctx, v20231001preview.ResourceType, v1.OperationMethod(datamodel.OperationProcess), func(opts ctrl.Options) (ctrl.Controller, error) {
		return resourcegroups.NewTrackedResourceProcessController(opts, transport, defaultDownstream)
	}, opts))

	// Resource providers and related types
	err = errors.Join(err, registry.Register(ctx, datamodel.ResourceProviderResourceType, v1.OperationPut, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.ResourceProviderPutController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(ctx, datamodel.ResourceProviderResourceType, v1.OperationDelete, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.ResourceProviderDeleteController{
			BaseController: ctrl.NewBaseAsyncController(opts),
			Connection:     connection,
		}, nil
	}, opts))
	err = errors.Join(err, registry.Register(ctx, datamodel.ResourceTypeResourceType, v1.OperationPut, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.ResourceTypePutController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(ctx, datamodel.ResourceTypeResourceType, v1.OperationDelete, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.ResourceTypeDeleteController{
			BaseController: ctrl.NewBaseAsyncController(opts),
			Connection:     connection,
		}, nil
	}, opts))
	err = errors.Join(err, registry.Register(ctx, datamodel.APIVersionResourceType, v1.OperationPut, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.APIVersionPutController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(ctx, datamodel.APIVersionResourceType, v1.OperationDelete, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.APIVersionDeleteController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(ctx, datamodel.LocationResourceType, v1.OperationPut, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.LocationPutController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))
	err = errors.Join(err, registry.Register(ctx, datamodel.LocationResourceType, v1.OperationDelete, func(opts ctrl.Options) (ctrl.Controller, error) {
		return &resourceproviders.LocationDeleteController{BaseController: ctrl.NewBaseAsyncController(opts)}, nil
	}, opts))

	if err != nil {
		return err
	}

	return nil
}

func waitForServer(ctx context.Context, host, port string, retryInterval time.Duration, timeout time.Duration) error {
	address := net.JoinHostPort(host, port)
	deadline := time.Now().Add(timeout)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("connection attempts canceled or timed out: %w", ctx.Err())
		default:
			conn, err := net.DialTimeout("tcp", address, retryInterval)
			if err == nil {
				conn.Close()
				return nil
			}

			if time.Now().After(deadline) {
				return fmt.Errorf("failed to connect to %s after %v: %w", address, timeout, err)
			}

			time.Sleep(retryInterval)
		}
	}
}

func main() {

	// Start the server in a separate goroutine
	go func() {
		if err := host.Start(); err != nil {
			logger.Error(err, "Server failed to start")
			os.Exit(1)
		}
	}()

	// Set up signal handling for graceful shutdown
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the manifest registration in a goroutine
	go func() {
		// Define connection parameters
		hostName := host.HostName() // Replace with actual method
		port := host.Port()         // Replace with actual method

		// Define context with timeout for the connection attempts
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Attempt to connect to the server
		err := waitForServer(ctx, hostName, port, 500*time.Millisecond, 30*time.Second)
		if err != nil {
			logger.Error(err, "Server is not available for manifest registration")
			return
		}

		// Server is up, proceed to register manifests
		manifestDir := options.Config.Manifests.ManifestDirectory
		if _, err := os.Stat(manifestDir); os.IsNotExist(err) {
			logger.Error(err, "Manifest directory does not exist", "directory", manifestDir)
			return
		} else if err != nil {
			logger.Error(err, "Error checking manifest directory", "directory", manifestDir)
			return
		}

		ucpclient, err := ucpclient.NewUCPClient(options.UCPConnection)
		if err != nil {
			logger.Error(err, "Failed to create UCP client")
			return
		}

		// Proceed with registering manifests
		if err := ucpclient.RegisterManifests(ctx, manifestDir); err != nil {
			logger.Error(err, "Failed to register manifests")
			return
		}

		logger.Info("Successfully registered manifests", "directory", manifestDir)
	}()

	// Wait for a termination signal
	<-stopChan
	logger.Info("Received termination signal. Shutting down...")

	// Gracefully shut down the server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := host.Shutdown(shutdownCtx); err != nil {
		logger.Error(err, "Server shutdown failed")
	} else {
		logger.Info("Server gracefully stopped")
	}

	// Additional cleanup if necessary
}
*/
