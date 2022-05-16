// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/ucp/server"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

var rootCmd = &cobra.Command{
	Use:   "ucpd",
	Short: "UCP server",
	Long:  `Server process for the Univeral Control Plane (UCP).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := ucplog.NewLogger()
		ctx := logr.NewContext(cmd.Context(), logger)
		ctx, cancel := context.WithCancel(ctx)

		options, err := server.NewServerOptionsFromEnvironment()
		if err != nil {
			return err
		}

		host, err := server.NewServer(options)
		if err != nil {
			return err
		}

		stopped, serviceErrors := host.RunAsync(ctx)

		// Monitor for failures to start, and shut down if a service fails.
		go func() {
			for message := range serviceErrors {
				if message.Err != nil {
					logger.Info("Service errored, shutting down....")
					cancel()
				}
			}
		}()

		// Finished shutting down. An error returned here is a failure to terminate
		// gracefully, so just crash if that happens.
		err = <-stopped
		if err != nil {
			return err
		}

		return nil
	},
}

func Execute() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c
		cancel()
	}()

	cobra.CheckErr(rootCmd.ExecuteContext(ctx))
}
