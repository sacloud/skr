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

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// zoneItem はゾーンの一覧と参照で確認する項目です。
type zoneItem struct {
	ID          int64
	Name        string
	Description string
}

// zoneCommands はシナリオが実行する zone コマンドの操作です。
// ライブ実行では helper 経由の skr CLI、単体テストでは偽の実装を使います。
type zoneCommands interface {
	// findZones はゾーン一覧を取得します。
	findZones(t *testing.T, label string) ([]zoneItem, error)
	// findZoneNames は jq 式でゾーン名の配列を取得します。
	findZoneNames(t *testing.T, label string) ([]string, error)
	// readZone は指定した ID のゾーンを参照します。
	readZone(t *testing.T, label string, id int64) (zoneItem, error)
}

// skrZoneCommands は helper 経由で skr CLI の zone コマンドを実行します。
type skrZoneCommands struct {
	h *helper.Helper
}

func (c skrZoneCommands) findZones(t *testing.T, label string) ([]zoneItem, error) {
	data, _, err := c.h.CallAllowingFailure(t, label, "iaas-api", "zone", "find")
	if err != nil {
		return nil, err
	}
	return decodeZoneList(data)
}

func (c skrZoneCommands) findZoneNames(t *testing.T, label string) ([]string, error) {
	data, _, err := c.h.CallAllowingFailure(t, label, "iaas-api", "zone", "find", "--query", "map(.Name)")
	if err != nil {
		return nil, err
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil, fmt.Errorf("decode zone names from --query output: %w", err)
	}
	return names, nil
}

func (c skrZoneCommands) readZone(t *testing.T, label string, id int64) (zoneItem, error) {
	data, _, err := c.h.CallAllowingFailure(t, label, "iaas-api", "zone", "read",
		"--id", strconv.FormatInt(id, 10), "--query", "{ID,Name,Description}")
	if err != nil {
		return zoneItem{}, err
	}
	return parseZone(data)
}

// decodeZoneList は find の JSON 応答をデコードします。
func decodeZoneList(data []byte) ([]zoneItem, error) {
	var zones []zoneItem
	if err := json.Unmarshal(data, &zones); err != nil {
		return nil, fmt.Errorf("decode zone list: %w", err)
	}
	return zones, nil
}

// parseZone は read の --query 出力をデコードします。
func parseZone(data []byte) (zoneItem, error) {
	var item zoneItem
	if err := json.Unmarshal(data, &item); err != nil {
		return zoneItem{}, fmt.Errorf("decode zone: %w", err)
	}
	return item, nil
}
