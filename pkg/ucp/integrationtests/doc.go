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

// integrationtests contains integration tests that run UCP using an in-memory server and in-memory data stores.
//
// Many of UCP's features are stateful in a global way (planes, credentials, etc) so testing in-memory is faster and more convenient
// than testing a deployed instance of UCP.
//
// We also know that UCP's use-cases are different from that of a traditional RP, therefore we want to pay special attention to the details.
package integrationtests
