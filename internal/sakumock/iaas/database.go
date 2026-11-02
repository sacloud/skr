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

package iaas

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AddDatabase はテストの初期状態として Database を登録します。CLI 側の API factory から
// 差し替える caller が find・read で返す内容を用意します。value は API の naked 形式の
// JSON に変換できる値です。
func (s *Server) AddDatabase(zone, id string, value any) error {
	data, err := marshalJSON(value)
	if err != nil {
		return err
	}
	var item map[string]json.RawMessage
	if err := json.Unmarshal(data, &item); err != nil {
		return fmt.Errorf("decode mock Database: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.databases[zone+"/"+id] = item
	return nil
}

// doDatabase は Database (appliance) の操作を処理します。必要な操作に限って
// find・read・delete・boot・shutdown・reset・monitor・list-parameter を再現します。
func (s *Server) doDatabase(method string, parts []string, uri string, body any) ([]byte, error) {
	applianceIndex := -1
	for i, part := range parts {
		if part == "appliance" {
			applianceIndex = i
			break
		}
	}
	if applianceIndex < 0 {
		return nil, fmt.Errorf("unsupported IaaS request URL: %s", uri)
	}
	zoneIndex := applianceIndex - 4
	if zoneIndex < 0 {
		return nil, fmt.Errorf("missing zone in IaaS request URL: %s", uri)
	}
	zone := parts[zoneIndex]

	id := ""
	if applianceIndex+1 < len(parts) {
		id = parts[applianceIndex+1]
	}
	var suffix []string
	if applianceIndex+2 < len(parts) {
		suffix = parts[applianceIndex+2:]
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := zone + "/" + id
	switch {
	case method == "GET" && id == "":
		items := s.databasesAsSlice(zone)
		return marshalJSON(map[string]any{
			"Total":      len(items),
			"Count":      len(items),
			"From":       0,
			"Appliances": items,
		})
	case method == "GET" && id != "" && len(suffix) == 0:
		item, ok := s.databases[key]
		if !ok {
			return nil, fmt.Errorf("database %s in zone %s not found", id, zone)
		}
		return marshalJSON(map[string]any{"IsOk": true, "Appliance": item})
	case method == "DELETE" && id != "" && len(suffix) == 0:
		if _, ok := s.databases[key]; !ok {
			return nil, fmt.Errorf("database %s in zone %s not found", id, zone)
		}
		delete(s.databases, key)
		return nil, nil
	case method == "PUT" && id != "" && len(suffix) == 1 && suffix[0] == "power":
		if _, ok := s.databases[key]; !ok {
			return nil, fmt.Errorf("database %s in zone %s not found", id, zone)
		}
		// boot は PUT /power を送ります
		return nil, nil
	case method == "DELETE" && id != "" && len(suffix) == 1 && suffix[0] == "power":
		if _, ok := s.databases[key]; !ok {
			return nil, fmt.Errorf("database %s in zone %s not found", id, zone)
		}
		// shutdown は DELETE /power を送ります
		return nil, nil
	case method == "PUT" && id != "" && len(suffix) == 1 && suffix[0] == "reset":
		if _, ok := s.databases[key]; !ok {
			return nil, fmt.Errorf("database %s in zone %s not found", id, zone)
		}
		return nil, nil
	case method == "GET" && id != "" && len(suffix) == 2 && suffix[0] == "cpu" && suffix[1] == "monitor":
		if _, ok := s.databases[key]; !ok {
			return nil, fmt.Errorf("database %s in zone %s not found", id, zone)
		}
		// アクティビティモニタはタイムスタンプがキーの map を直接返します
		return marshalJSON(map[string]any{
			"IsOk": true,
			"Data": map[string]any{
				"2026-01-01T00:00:00+09:00": map[string]any{"CPU-TIME": 10},
			},
		})
	case method == "GET" && id != "" && len(suffix) == 2 && suffix[0] == "database" && suffix[1] == "monitor":
		if _, ok := s.databases[key]; !ok {
			return nil, fmt.Errorf("database %s in zone %s not found", id, zone)
		}
		return marshalJSON(map[string]any{
			"IsOk": true,
			"Data": map[string]any{
				"2026-01-01T00:00:00+09:00": map[string]any{
					"Total-Memory-Size": 100,
					"Used-Memory-Size":  50,
					"Total-Disk1-Size":  200,
					"Used-Disk1-Size":   100,
					"Total-Disk2-Size":  0,
					"Used-Disk2-Size":   0,
					"binlogUsedSizeKiB": 10,
				},
			},
		})
	case method == "GET" && id != "" && len(suffix) == 3 && suffix[0] == "disk" && suffix[2] == "monitor":
		if _, ok := s.databases[key]; !ok {
			return nil, fmt.Errorf("database %s in zone %s not found", id, zone)
		}
		return marshalJSON(map[string]any{
			"IsOk": true,
			"Data": map[string]any{
				"2026-01-01T00:00:00+09:00": map[string]any{
					"Read":  10,
					"Write": 20,
				},
			},
		})
	case method == "GET" && id != "" && len(suffix) == 2 && suffix[0] == "interface" && suffix[1] == "monitor":
		if _, ok := s.databases[key]; !ok {
			return nil, fmt.Errorf("database %s in zone %s not found", id, zone)
		}
		return marshalJSON(map[string]any{
			"IsOk": true,
			"Data": map[string]any{
				"2026-01-01T00:00:00+09:00": map[string]any{
					"Send":    10,
					"Receive": 20,
				},
			},
		})
	case method == "GET" && id != "" && len(suffix) == 2 && suffix[0] == "database" && suffix[1] == "parameter":
		if _, ok := s.databases[key]; !ok {
			return nil, fmt.Errorf("database %s in zone %s not found", id, zone)
		}
		return marshalJSON(map[string]any{
			"Database": map[string]any{
				"Parameter": map[string]any{
					"Attr": map[string]any{
						"max_connections": 100,
					},
				},
				"Remark": map[string]any{
					"Form": []map[string]any{
						{
							"Type":  "number",
							"Name":  "max_connections",
							"Label": "最大接続数",
						},
					},
				},
			},
		})
	default:
		return nil, fmt.Errorf("unsupported IaaS operation: %s %s", method, uri)
	}
}

func (s *Server) databasesAsSlice(zone string) []map[string]json.RawMessage {
	items := make([]map[string]json.RawMessage, 0, len(s.databases))
	prefix := zone + "/"
	for key, item := range s.databases {
		if strings.HasPrefix(key, prefix) {
			items = append(items, item)
		}
	}
	return items
}
