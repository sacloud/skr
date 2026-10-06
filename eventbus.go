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

package main

import "github.com/sacloud/skr/internal/eventbusapi"

func (c *cli) initEventbusAPI() {
	processConfigurationRuntime := eventbusapi.ProcessConfigurationRuntime{
		DecodeRequest:   eventbusapi.DecodeRequest,
		OutputType:      outputType,
		WriteOutput:     writeOutputWithFormat,
		ValidateRequest: eventbusapi.ValidateRequest,
	}
	scheduleRuntime := eventbusapi.ScheduleRuntime{
		DecodeRequest:   eventbusapi.DecodeRequest,
		OutputType:      outputType,
		WriteOutput:     writeOutputWithFormat,
		ValidateRequest: eventbusapi.ValidateRequest,
	}
	triggerRuntime := eventbusapi.TriggerRuntime{
		DecodeRequest:   eventbusapi.DecodeRequest,
		OutputType:      outputType,
		WriteOutput:     writeOutputWithFormat,
		ValidateRequest: eventbusapi.ValidateRequest,
	}
	c.EventbusAPI.ProcessConfiguration.SetRuntime(processConfigurationRuntime)
	c.EventbusAPI.ProcessConfiguration.SetFactory(func() (eventbusapi.ProcessConfigurationAPI, error) {
		return eventbusapi.NewProcessConfigurationAPI(c.Trace)
	})
	c.EventbusAPI.Schedule.SetRuntime(scheduleRuntime)
	c.EventbusAPI.Schedule.SetFactory(func() (eventbusapi.ScheduleAPI, error) {
		return eventbusapi.NewScheduleAPI(c.Trace)
	})
	c.EventbusAPI.Trigger.SetRuntime(triggerRuntime)
	c.EventbusAPI.Trigger.SetFactory(func() (eventbusapi.TriggerAPI, error) {
		return eventbusapi.NewTriggerAPI(c.Trace)
	})
}
