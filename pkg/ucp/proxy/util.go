// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package proxy

import "net/http"

func appendDirector(original DirectorFunc, added ...DirectorFunc) DirectorFunc {
	return func(r *http.Request) {
		original(r)
		for _, director := range added {
			director(r)
		}
	}
}

func appendResponder(original ResponderFunc, added ...ResponderFunc) ResponderFunc {
	return func(r *http.Response) error {
		err := original(r)
		if err != nil {
			return err
		}

		for _, director := range added {
			err := director(r)
			if err != nil {
				return err
			}
		}

		return nil
	}
}

func appendErrorHandler(original ErrorHandlerFunc, added ...ErrorHandlerFunc) ErrorHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		original(w, r, err)
		for _, director := range added {
			director(w, r, err)
		}
	}
}
