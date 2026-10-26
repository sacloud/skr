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

// Package iaas は IaaS API 呼び出し用の Switch・PacketFilter・Zone に特化したインメモリモックを提供します。
// コンストラクタとライフサイクルは sakumock のテストサーバー規約に合わせています。
package iaas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
)

// Config はモックのオプションを保持します。
type Config struct {
	Zones []string
}

// Server はモックの Switch と PacketFilter のリソースを保持し、IaaS SDK の caller インターフェースを実装します。
type Server struct {
	mu            sync.Mutex
	switches      map[string]map[string]json.RawMessage
	packetFilters map[string]map[string]json.RawMessage
	icons         map[string]map[string]json.RawMessage
	zones         []string
	nextID        int
}

// NewTestServer はインメモリのモック API caller を作成します。
func NewTestServer(config Config) *Server {
	return &Server{
		switches:      make(map[string]map[string]json.RawMessage),
		packetFilters: make(map[string]map[string]json.RawMessage),
		icons:         make(map[string]map[string]json.RawMessage),
		zones:         append([]string(nil), config.Zones...),
		nextID:        1000,
	}
}

// Close は sakumock のテストサーバー規約に合わせて提供されています。
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
	for _, part := range parts {
		if part == "packetfilter" {
			return s.doPacketFilter(method, parts, uri, body)
		}
		if part == "icon" {
			return s.doIcon(method, parts, uri, body)
		}
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

func (s *Server) doIcon(method string, parts []string, uri string, body any) ([]byte, error) {
	iconIndex := -1
	for i, part := range parts {
		if part == "icon" {
			iconIndex = i
			break
		}
	}
	if iconIndex < 0 || iconIndex+2 < len(parts) {
		return nil, fmt.Errorf("unsupported IaaS request URL: %s", uri)
	}
	id := ""
	if iconIndex+1 < len(parts) {
		id = parts[iconIndex+1]
	}
	request, err := requestIcon(body)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case method == "GET" && id == "":
		items, total, err := s.findIcons(request)
		if err != nil {
			return nil, err
		}
		return marshalJSON(map[string]any{
			"Total": total,
			"Count": len(items),
			"From":  request["From"],
			"Icons": items,
		})
	case method == "GET":
		item, ok := s.icons[id]
		if !ok {
			return nil, iconNotFound(method, uri, id)
		}
		return marshalJSON(map[string]any{"Icon": item})
	case method == "POST" && id == "":
		if request == nil {
			return nil, errors.New("create icon request body is required")
		}
		s.nextID++
		id = fmt.Sprint(s.nextID)
		delete(request, "Image")
		request["Scope"] = json.RawMessage(`"user"`)
		request["ID"], err = json.Marshal(s.nextID)
		if err != nil {
			return nil, fmt.Errorf("encode mock Icon ID: %w", err)
		}
		s.icons[id] = request
		return marshalJSON(map[string]any{"Icon": request})
	case method == "PUT" && id != "":
		current, ok := s.icons[id]
		if !ok {
			return nil, iconNotFound(method, uri, id)
		}
		for field, value := range request {
			current[field] = value
		}
		s.icons[id] = current
		return marshalJSON(map[string]any{"Icon": current})
	case method == "DELETE" && id != "":
		if _, ok := s.icons[id]; !ok {
			return nil, iconNotFound(method, uri, id)
		}
		delete(s.icons, id)
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported IaaS operation: %s %s", method, uri)
	}
}

func iconNotFound(method, uri, id string) error {
	requestURL, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("parse Icon request URL: %w", err)
	}
	return iaas.NewAPIError(method, requestURL, http.StatusNotFound, &iaas.APIErrorResponse{
		ErrorCode: "not_found", ErrorMessage: fmt.Sprintf("icon %s not found", id),
	})
}

func (s *Server) findIcons(request map[string]json.RawMessage) ([]map[string]json.RawMessage, int, error) {
	var condition struct {
		Count  int
		From   int
		Sort   []string
		Filter struct {
			Name  string
			Tags  [][]string `json:"Tags.Name"`
			Scope []string
		}
	}
	data, err := json.Marshal(request)
	if err != nil {
		return nil, 0, fmt.Errorf("encode Icon search: %w", err)
	}
	if err := json.Unmarshal(data, &condition); err != nil {
		return nil, 0, fmt.Errorf("decode Icon search: %w", err)
	}
	if condition.Count < 0 || condition.From < 0 {
		return nil, 0, errors.New("Icon search Count and From must not be negative")
	}
	name, err := url.QueryUnescape(condition.Filter.Name)
	if err != nil {
		return nil, 0, fmt.Errorf("decode Icon name filter: %w", err)
	}
	items := make([]map[string]json.RawMessage, 0)
	for _, item := range s.iconsAsSlice() {
		var icon struct {
			Name  string
			Tags  []string
			Scope string
		}
		data, err := json.Marshal(item)
		if err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal(data, &icon); err != nil {
			return nil, 0, fmt.Errorf("decode stored Icon: %w", err)
		}
		matches := true
		for _, part := range strings.Fields(name) {
			matches = matches && strings.Contains(icon.Name, part)
		}
		for _, group := range condition.Filter.Tags {
			for _, tag := range group {
				found := false
				for _, actual := range icon.Tags {
					found = found || actual == tag
				}
				matches = matches && found
			}
		}
		if len(condition.Filter.Scope) != 0 {
			found := false
			for _, scope := range condition.Filter.Scope {
				found = found || icon.Scope == scope
			}
			matches = matches && found
		}
		if matches {
			items = append(items, item)
		}
	}
	sortKeys := append(append([]string(nil), condition.Sort...), "ID")
	for _, key := range sortKeys {
		switch strings.TrimLeft(key, "+-") {
		case "ID", "Name", "Scope":
		default:
			return nil, 0, fmt.Errorf("unsupported Icon mock sort key %q", key)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		for _, key := range sortKeys {
			field := strings.TrimLeft(key, "+-")
			a, b := string(items[i][field]), string(items[j][field])
			if a != b {
				if field == "ID" && len(a) != len(b) {
					if strings.HasPrefix(key, "-") {
						return len(a) > len(b)
					}
					return len(a) < len(b)
				}
				if strings.HasPrefix(key, "-") {
					return a > b
				}
				return a < b
			}
		}
		return false
	})
	total := len(items)
	from := min(condition.From, total)
	end := total
	if condition.Count > 0 {
		end = from + min(condition.Count, total-from)
	}
	return items[from:end], total, nil
}

func (s *Server) doPacketFilter(method string, parts []string, uri string, body any) ([]byte, error) {
	packetFilterIndex := -1
	for i, part := range parts {
		if part == "packetfilter" {
			packetFilterIndex = i
			break
		}
	}
	if packetFilterIndex < 0 || packetFilterIndex+2 < len(parts) {
		return nil, fmt.Errorf("unsupported IaaS request URL: %s", uri)
	}
	zoneIndex := packetFilterIndex - 4
	if zoneIndex < 0 {
		return nil, fmt.Errorf("missing zone in IaaS request URL: %s", uri)
	}
	zone := parts[zoneIndex]
	id := ""
	if packetFilterIndex+1 < len(parts) {
		id = parts[packetFilterIndex+1]
	}
	key := zone + "/" + id
	request, err := requestPacketFilter(body)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case method == "GET" && id == "":
		items := s.packetFiltersAsSlice(zone)
		return marshalJSON(map[string]any{
			"Total":         len(items),
			"Count":         len(items),
			"PacketFilters": items,
		})
	case method == "GET":
		item, ok := s.packetFilters[key]
		if !ok {
			return nil, fmt.Errorf("PacketFilter %s in zone %s not found", id, zone)
		}
		return marshalJSON(map[string]any{"PacketFilter": item})
	case method == "POST" && id == "":
		if request == nil {
			return nil, errors.New("create PacketFilter request body is required")
		}
		s.nextID++
		id = fmt.Sprint(s.nextID)
		key = zone + "/" + id
		request["ID"], err = json.Marshal(s.nextID)
		if err != nil {
			return nil, fmt.Errorf("encode mock PacketFilter ID: %w", err)
		}
		request["ExpressionHash"] = json.RawMessage(`"mock-expression-hash"`)
		s.packetFilters[key] = request
		return marshalJSON(map[string]any{"PacketFilter": request})
	case method == "PUT" && id != "":
		current, ok := s.packetFilters[key]
		if !ok {
			return nil, fmt.Errorf("PacketFilter %s in zone %s not found", id, zone)
		}
		for field, value := range request {
			current[field] = value
		}
		s.packetFilters[key] = current
		return marshalJSON(map[string]any{"PacketFilter": current})
	case method == "DELETE" && id != "":
		if _, ok := s.packetFilters[key]; !ok {
			return nil, fmt.Errorf("PacketFilter %s in zone %s not found", id, zone)
		}
		delete(s.packetFilters, key)
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported IaaS operation: %s %s", method, uri)
	}
}

func (s *Server) iconsAsSlice() []map[string]json.RawMessage {
	items := make([]map[string]json.RawMessage, 0, len(s.icons))
	for _, item := range s.icons {
		items = append(items, item)
	}
	return items
}

func requestIcon(value any) (map[string]json.RawMessage, error) {
	var data []byte
	if value != nil {
		var err error
		data, err = json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode Icon request: %w", err)
		}
	}
	var body map[string]json.RawMessage
	if len(data) > 0 {
		if err := json.Unmarshal(data, &body); err != nil {
			return nil, fmt.Errorf("decode Icon request: %w", err)
		}
	}
	if item, ok := body["Icon"]; ok {
		if err := json.Unmarshal(item, &body); err != nil {
			return nil, fmt.Errorf("decode Icon request body: %w", err)
		}
	}
	return body, nil
}

func (s *Server) packetFiltersAsSlice(zone string) []map[string]json.RawMessage {
	items := make([]map[string]json.RawMessage, 0, len(s.packetFilters))
	prefix := zone + "/"
	for key, item := range s.packetFilters {
		if strings.HasPrefix(key, prefix) {
			items = append(items, item)
		}
	}
	return items
}

func requestPacketFilter(value any) (map[string]json.RawMessage, error) {
	var data []byte
	if value != nil {
		var err error
		data, err = json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode PacketFilter request: %w", err)
		}
	}
	var body map[string]json.RawMessage
	if len(data) > 0 {
		if err := json.Unmarshal(data, &body); err != nil {
			return nil, fmt.Errorf("decode PacketFilter request: %w", err)
		}
	}
	if item, ok := body["PacketFilter"]; ok {
		if err := json.Unmarshal(item, &body); err != nil {
			return nil, fmt.Errorf("decode PacketFilter request body: %w", err)
		}
	}
	return body, nil
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
