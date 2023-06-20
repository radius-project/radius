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

// clierrors defines error types that are useful in the CLI for customizing the experience when a failure occurs. This package
// is named clierrors to avoid a name collision with the standard library errors package. Code that needs to handle errors will
// need to import both packages.
//
// Most error handling code in the CLI should use the clierrors.Message function to return user-friendly error messages.
//
// For "expected" error cases that can be classified, use clierrors.MessageWithCause to return a user-friendly error message
// and wrap the original error.
//
// For "unexpected" or "unclassified" error cases the error should be returned as-is. This will result in a generic error
// experience for the user that encourages them to file a bug report.
//
// Types in other packages can also implement the FriendlyError interface to give their error types special handling. This
// removes the need for error handling at other levels of the code.
package clierrors
