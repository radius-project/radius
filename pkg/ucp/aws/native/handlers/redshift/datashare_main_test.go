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

package redshift

import "github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"

func init() {
	// The fake shows a change on the next read, so tests don't wait between reads. A recording waits as UCP does.
	if !awsreplay.Recording() {
		dataShareConfirmWait = 0
	}
}
