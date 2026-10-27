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
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
)

// TestScenarioZone は zone コマンドのライブ E2E です。
// ゾーン一覧の取得、jq 式での名前抽出、ID での参照を確認します。
// 読み取り専用のシナリオで、クラウドリソースは作成・変更・削除しません。
func TestScenarioZone(t *testing.T) {
	h := helper.New(t, "zone-api",
		"IaaS API の zone コマンドでゾーン一覧の取得、名前の抽出、ID での参照が正しく動作することを確認する")

	if err := runZoneScenario(t, skrZoneCommands{h: h}); err != nil {
		t.Fatalf("%v", err)
	}
}

// runZoneScenario はゾーンの一覧・名前抽出・参照を確認します。
// 失敗はエラーで返し、ライブ実行ではテストを失敗させます。
func runZoneScenario(t *testing.T, commands zoneCommands) error {
	t.Helper()

	// ゾーン一覧を取得して ID と名前を確認する
	zones, err := commands.findZones(t, "ゾーン一覧を取得する")
	if err != nil {
		return fmt.Errorf("find zones: %w", err)
	}
	if len(zones) == 0 {
		return errors.New("zone list is empty")
	}
	for i, z := range zones {
		if z.ID == 0 {
			return fmt.Errorf("zone %d has no ID", i)
		}
		if z.Name == "" {
			return fmt.Errorf("zone %d has an empty name", i)
		}
	}

	// jq 式でゾーン名を抽出して一覧と一致させる
	names, err := commands.findZoneNames(t, "jq 式でゾーン名を抽出する")
	if err != nil {
		return fmt.Errorf("find zone names: %w", err)
	}
	expected := make([]string, 0, len(zones))
	for _, z := range zones {
		expected = append(expected, z.Name)
	}
	if !reflect.DeepEqual(expected, names) {
		return fmt.Errorf("zone names from --query output mismatch: want %#v, got %#v", expected, names)
	}

	// 一覧の先頭ゾーンを ID で参照して一覧と一致させる
	first := zones[0]
	read, err := commands.readZone(t, "ゾーンを ID で参照する", first.ID)
	if err != nil {
		return fmt.Errorf("read zone %d: %w", first.ID, err)
	}
	if read.ID != first.ID {
		return fmt.Errorf("read zone ID mismatch: want %d, got %d", first.ID, read.ID)
	}
	if read.Name != first.Name {
		return fmt.Errorf("read zone name mismatch: want %q, got %q", first.Name, read.Name)
	}
	return nil
}
