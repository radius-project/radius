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

package rediscaches

import (
	"encoding/json"

	"github.com/radius-project/radius/pkg/linkrp/api/v20220315privatepreview"
	"github.com/radius-project/radius/pkg/linkrp/datamodel"
	"github.com/radius-project/radius/test/testutil"
)

const testHeaderfile = "20220315privatepreview_requestheaders.json"

func getTestModels20220315privatepreview() (input *v20220315privatepreview.RedisCacheResource, dataModel *datamodel.RedisCache, output *v20220315privatepreview.RedisCacheResource) {
	rawDataModel := testutil.ReadFixture("20220315privatepreview_datamodel.json")
	dataModel = &datamodel.RedisCache{}
	_ = json.Unmarshal(rawDataModel, dataModel)

	return input, dataModel, output
}
