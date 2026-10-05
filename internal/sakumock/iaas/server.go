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

// Package iaas provides a Switch- and Zone-focused in-memory mock for IaaS API calls.
// Its constructor and lifecycle mirror the sakumock test-server convention.
package iaas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// Config holds mock options.
type Config struct {
	Zones []string
}

// Server stores mock Switch resources and implements the IaaS SDK caller interface.
type Server struct {
	mu       sync.Mutex
	switches map[string]map[string]json.RawMessage
	zones    []string
	nextID   int
}

// NewTestServer creates an in-memory mock API caller.
func NewTestServer(config Config) *Server {
	return &Server{
		switches: make(map[string]map[string]json.RawMessage),
		zones:    append([]string(nil), config.Zones...),
		nextID:   1000,
	}
}

// Close is provided to match the sakumock test-server lifecycle.
func (s *Server) Close() {}

func (s *Server) Do(ctx context.Context, method, uri string, body any) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	parsedURL, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("parse IaaS request URL: %w", err)
	}
	parts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	if method == "GET" && parts[len(parts)-1] == "zone" {
		s.mu.Lock()
		defer s.mu.Unlock()
		zones := make([]map[string]string, 0, len(s.zones))
		for _, name := range s.zones {
			zones = append(zones, map[string]string{"Name": name})
		}
		return marshalJSON(map[string]any{
			"Total": len(zones),
			"Count": len(zones),
			"Zones": zones,
		})
	}
	switchIndex := -1
	for i, part := range parts {
		if part == "switch" {
			switchIndex = i
		}
	}
	if switchIndex < 0 || switchIndex+2 < len(parts) {
		return nil, fmt.Errorf("unsupported IaaS request URL: %s", uri)
	}
	zoneIndex := switchIndex - 4
	if zoneIndex < 0 {
		return nil, fmt.Errorf("missing zone in IaaS request URL: %s", uri)
	}
	zone := parts[zoneIndex]

	id := ""
	if switchIndex+1 < len(parts) {
		id = parts[switchIndex+1]
	}
	key := zone + "/" + id
	request, err := requestSwitch(body)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case method == "GET" && id == "":
		items := s.switchesAsSlice(zone)
		return marshalJSON(map[string]any{
			"Total":    len(items),
			"Count":    len(items),
			"Switches": items,
		})
	case method == "GET":
		item, ok := s.switches[key]
		if !ok {
			return nil, fmt.Errorf("switch %s in zone %s not found", id, zone)
		}
		return marshalJSON(map[string]any{"Switch": item})
	case method == "POST" && id == "":
		if request == nil {
			return nil, errors.New("create switch request body is required")
		}
		s.nextID++
		id = fmt.Sprint(s.nextID)
		key = zone + "/" + id
		request["ID"], err = json.Marshal(s.nextID)
		if err != nil {
			return nil, fmt.Errorf("encode mock Switch ID: %w", err)
		}
		s.switches[key] = request
		return marshalJSON(map[string]any{"Switch": request})
	case method == "PUT" && id != "":
		current, ok := s.switches[key]
		if !ok {
			return nil, fmt.Errorf("switch %s in zone %s not found", id, zone)
		}
		for key, value := range request {
			current[key] = value
		}
		s.switches[key] = current
		return marshalJSON(map[string]any{"Switch": current})
	case method == "DELETE" && id != "":
		if _, ok := s.switches[key]; !ok {
			return nil, fmt.Errorf("switch %s in zone %s not found", id, zone)
		}
		delete(s.switches, key)
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported IaaS operation: %s %s", method, uri)
	}
}

func (s *Server) switchesAsSlice(zone string) []map[string]json.RawMessage {
	items := make([]map[string]json.RawMessage, 0, len(s.switches))
	prefix := zone + "/"
	for key, item := range s.switches {
		if strings.HasPrefix(key, prefix) {
			items = append(items, item)
		}
	}
	return items
}

func requestSwitch(value any) (map[string]json.RawMessage, error) {
	var data []byte
	if value != nil {
		var err error
		data, err = json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode switch request: %w", err)
		}
	}
	var body map[string]json.RawMessage
	if len(data) > 0 {
		if err := json.Unmarshal(data, &body); err != nil {
			return nil, fmt.Errorf("decode switch request: %w", err)
		}
	}
	if item, ok := body["Switch"]; ok {
		if err := json.Unmarshal(item, &body); err != nil {
			return nil, fmt.Errorf("decode Switch request: %w", err)
		}
	}
	return body, nil
}

func marshalJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode mock IaaS response: %w", err)
	}
	return data, nil
}
