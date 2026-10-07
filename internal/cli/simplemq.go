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

import "github.com/sacloud/skr/internal/simplemqapi"

func (c *cli) initSimpleMQAPI() {
	c.SimpleMQAPI.Queue.SetRuntime(simplemqapi.QueueRuntime{
		DecodeRequest:  decodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	})
	c.SimpleMQAPI.Queue.SetFactory(func() (simplemqapi.QueueAPI, error) {
		return simplemqapi.NewQueueAPI(c.Trace)
	})
	c.SimpleMQAPI.Message.SetRuntime(simplemqapi.MessageRuntime{
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	})
	c.SimpleMQAPI.Message.SetFactory(func(queueName, apiKeyFile string) (simplemqapi.MessageAPI, error) {
		return simplemqapi.NewMessageAPI(queueName, apiKeyFile, c.Trace)
	})
}
