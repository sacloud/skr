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

import "github.com/sacloud/skr/internal/eventbusapi"

func (c *cli) initEventbusAPI() {
	c.EventbusAPI.ProcessConfiguration = eventbusapi.NewProcessConfigurationCommands(eventbusapi.ProcessConfigurationRuntime{
		DecodeRequest:  eventbusapi.DecodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	}, func() (eventbusapi.ProcessConfigurationAPI, error) {
		return eventbusapi.NewProcessConfigurationAPI(c.Trace)
	})
	c.EventbusAPI.Schedule = eventbusapi.NewScheduleCommands(eventbusapi.ScheduleRuntime{
		DecodeRequest:  eventbusapi.DecodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	}, func() (eventbusapi.ScheduleAPI, error) {
		return eventbusapi.NewScheduleAPI(c.Trace)
	})
	c.EventbusAPI.Trigger = eventbusapi.NewTriggerCommands(eventbusapi.TriggerRuntime{
		DecodeRequest:  eventbusapi.DecodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	}, func() (eventbusapi.TriggerAPI, error) {
		return eventbusapi.NewTriggerAPI(c.Trace)
	})
}
