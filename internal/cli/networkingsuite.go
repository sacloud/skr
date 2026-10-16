// Copyright 2022-2026 The sacloud/skr Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	v1 "github.com/sacloud/sacloud-sdk-go/api/networking-suite/apis/v1"
	"github.com/sacloud/skr/internal/networkingsuiteapi"
)

func (c *cli) initNetworkingSuiteAPI() {
	c.NetworkingSuiteAPI = networkingsuiteapi.NewCommands(networkingsuiteapi.Runtime{
		NewClient: func() (*v1.Client, error) {
			return networkingsuiteapi.NewClient(c.Trace)
		},
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	})
}
