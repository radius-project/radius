/*
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
*/

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
