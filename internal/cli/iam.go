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

import "github.com/sacloud/skr/internal/iamapi"

func (c *cli) initIAMAPI() {
	userRuntime := iamapi.UserRuntime{
		DecodeRequest:  iamapi.DecodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	}
	groupRuntime := iamapi.GroupRuntime{
		DecodeRequest:  iamapi.DecodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	}
	policyRuntime := iamapi.PolicyRuntime{
		DecodeRequest:  iamapi.DecodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	}
	c.IAMAPI.User.SetRuntime(userRuntime)
	c.IAMAPI.User.SetFactory(func() (iamapi.UserAPI, error) {
		return iamapi.NewUserAPI(c.Trace)
	})
	c.IAMAPI.Group.SetRuntime(groupRuntime)
	c.IAMAPI.Group.SetFactory(func() (iamapi.GroupAPI, error) {
		return iamapi.NewGroupAPI(c.Trace)
	})
	c.IAMAPI.Policy.SetRuntime(policyRuntime)
	c.IAMAPI.Policy.SetFactory(func() (iamapi.PolicyAPI, error) {
		return iamapi.NewPolicyAPI(c.Trace)
	})
}
