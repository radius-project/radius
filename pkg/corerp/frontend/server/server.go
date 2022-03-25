// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/corerp/middleware"
	"github.com/project-radius/radius/pkg/version"
)

type ServerOptions struct {
	Address string
	// TODO: implement client cert based authentication for arm
	EnableAuth bool
	Configure  func(*mux.Router)
}

// NewServer will create a server that can listen on the provided address and serve requests.
func NewServer(ctx context.Context, options ServerOptions) *http.Server {
	r := mux.NewRouter()
	if options.Configure != nil {
		options.Configure(r)
	}

	r.Use(middleware.AppendLogValues)
	r.Path("/version").Methods(http.MethodGet).HandlerFunc(reportVersion)

	server := &http.Server{
		Addr:    options.Address,
		Handler: middleware.LowercaseURLPath(r),
		BaseContext: func(ln net.Listener) context.Context {
			return ctx
		},
	}

	return server
}

func reportVersion(w http.ResponseWriter, req *http.Request) {
	info := version.NewVersionInfo()

	b, err := json.MarshalIndent(&info, "", "  ")
	if err != nil {
		w.WriteHeader(500)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	_, _ = w.Write(b)
}
