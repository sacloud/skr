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

package gslbapi

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/gslb"
)

func createRequestFromFlags(command *CreateCommand) (*gslb.CreateRequest, error) {
	healthCheck, _, err := buildHealthCheck(command.HealthCheckProtocol, command.HealthCheckHostHeader,
		command.HealthCheckPath, command.HealthCheckResponseCode, command.HealthCheckPort)
	if err != nil {
		return nil, err
	}
	servers, err := parseDestinationServers(command.DestinationServers)
	if err != nil {
		return nil, err
	}

	request := &gslb.CreateRequest{
		Name:               *command.Name,
		HealthCheck:        healthCheck,
		DestinationServers: servers,
	}
	if command.Description != nil {
		request.Description = *command.Description
	}
	if command.Tags != nil {
		request.Tags = types.Tags(*command.Tags)
	}
	if command.IconID != nil {
		request.IconID = types.ID(*command.IconID)
	}
	if command.DelayLoop != nil {
		request.DelayLoop = *command.DelayLoop
	}
	if command.Weighted != nil {
		request.Weighted = types.StringFlag(*command.Weighted)
	}
	if command.SorryServer != nil {
		request.SorryServer = *command.SorryServer
	}
	return request, nil
}

func updateRequestFromFlags(command *UpdateCommand) (*gslb.UpdateRequest, error) {
	request := &gslb.UpdateRequest{ID: types.ID(*command.ID)}
	if command.Name != nil {
		request.Name = command.Name
	}
	if command.Description != nil {
		request.Description = command.Description
	}
	if command.Tags != nil {
		tags := types.Tags(*command.Tags)
		request.Tags = &tags
	}
	if command.IconID != nil {
		iconID := types.ID(*command.IconID)
		request.IconID = &iconID
	}
	if command.DelayLoop != nil {
		request.DelayLoop = command.DelayLoop
	}
	if command.Weighted != nil {
		weighted := types.StringFlag(*command.Weighted)
		request.Weighted = &weighted
	}
	if command.SorryServer != nil {
		request.SorryServer = command.SorryServer
	}
	healthCheck, set, err := buildHealthCheck(command.HealthCheckProtocol, command.HealthCheckHostHeader,
		command.HealthCheckPath, command.HealthCheckResponseCode, command.HealthCheckPort)
	if err != nil {
		return nil, err
	}
	if set {
		request.HealthCheck = healthCheck
	}
	if command.DestinationServers != nil {
		servers, err := parseDestinationServers(command.DestinationServers)
		if err != nil {
			return nil, err
		}
		request.DestinationServers = servers
	}
	if command.MonitoringSuiteLogEnabled != nil {
		request.MonitoringSuiteLog = &iaas.MonitoringSuiteLog{Enabled: *command.MonitoringSuiteLogEnabled}
	}
	return request, nil
}

func buildHealthCheck(protocol, hostHeader, path *string, responseCode, port *int) (*iaas.GSLBHealthCheck, bool, error) {
	set := protocol != nil || hostHeader != nil || path != nil || responseCode != nil || port != nil
	if !set {
		return nil, false, nil
	}
	if protocol == nil {
		return nil, true, fmt.Errorf("--health-check-protocol が必要です")
	}
	if !slices.Contains(types.GSLBHealthCheckProtocolStrings, *protocol) {
		return nil, true, fmt.Errorf("unsupported --health-check-protocol; choose http, https, tcp, or ping")
	}

	result := &iaas.GSLBHealthCheck{Protocol: types.EGSLBHealthCheckProtocol(*protocol)}
	if hostHeader != nil {
		result.HostHeader = *hostHeader
	}
	if path != nil {
		result.Path = *path
	}
	if responseCode != nil {
		result.ResponseCode = types.StringNumber(*responseCode)
	}
	if port != nil {
		result.Port = types.StringNumber(*port)
	}
	return result, true, nil
}

func parseDestinationServers(inputs []string) (iaas.GSLBServers, error) {
	if inputs == nil {
		return nil, nil
	}
	result := make(iaas.GSLBServers, 0, len(inputs))
	for i, input := range inputs {
		server, err := parseDestinationServer(input)
		if err != nil {
			return nil, fmt.Errorf("--destination-server %d: %w", i+1, err)
		}
		result = append(result, server)
	}
	return result, nil
}

func parseDestinationServer(input string) (*iaas.GSLBServer, error) {
	fields := strings.Split(input, ",")
	values := make(map[string]string, len(fields))
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok || key == "" || value == "" {
			return nil, fmt.Errorf("expected non-empty key=value entries")
		}
		switch key {
		case "ip-address", "enabled", "weight":
		default:
			return nil, fmt.Errorf("unknown key %q", key)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		values[key] = value
	}
	address, ok := values["ip-address"]
	if !ok {
		return nil, fmt.Errorf("ip-address is required")
	}

	server := iaas.NewGSLBServer(address)
	if enabled, exists := values["enabled"]; exists {
		if enabled != "true" && enabled != "false" {
			return nil, fmt.Errorf("enabled must be true or false")
		}
		server.Enabled = types.StringFlag(enabled == "true")
	}
	if weight, exists := values["weight"]; exists {
		n, err := strconv.Atoi(weight)
		if err != nil || n < 1 || n > 10000 {
			return nil, fmt.Errorf("weight must be an integer from 1 through 10000")
		}
		server.Weight = types.StringNumber(n)
	}
	return server, nil
}
