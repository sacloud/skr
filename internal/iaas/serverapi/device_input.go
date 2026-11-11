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

package serverapi

import (
	"fmt"
	"strconv"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/internal/textjson"
)

// parseDisk は反復可能な --disk の 1 回の指定をディスクのリクエストへ変換します。
// 構文は skr 全体で統一された text → JSON 簡易表記 (ADR 0078) の共通パーサーで解析し、
// 値の検証 (正の整数) と API リクエストへの変換はこの関数が行います。
func parseDisk(input, name string) (*DiskRequest, error) {
	fields, err := textjson.Parse(input, []textjson.Field{
		{Name: "archive-id"},
		{Name: "plan-id", Required: true},
		{Name: "size-gb", Required: true},
	})
	if err != nil {
		return nil, err
	}
	plan, err := positiveDeviceInteger(fields, "plan-id", 64)
	if err != nil {
		return nil, err
	}
	size, err := positiveDeviceInteger(fields, "size-gb", strconv.IntSize)
	if err != nil {
		return nil, err
	}
	var archive int64
	if fields.Has("archive-id") {
		archive, err = positiveDeviceInteger(fields, "archive-id", 64)
		if err != nil {
			return nil, err
		}
	}
	return &DiskRequest{
		Name: name, DiskPlanID: types.ID(plan), SizeGB: int(size),
		SourceArchiveID: types.ID(archive), Connection: types.DiskConnections.VirtIO,
	}, nil
}

// parseNIC は反復可能な --nic の 1 回の指定を NIC のリクエストへ変換します。
// 構文は skr 全体で統一された text → JSON 簡易表記 (ADR 0078) の共通パーサーで解析し、
// 値の検証 (列挙値または正の整数) と API リクエストへの変換はこの関数が行います。
func parseNIC(input string) (*NetworkInterface, error) {
	fields, err := textjson.Parse(input, []textjson.Field{
		{Name: "upstream", Required: true},
	})
	if err != nil {
		return nil, err
	}
	upstream := fields.Get("upstream")
	if upstream != "shared" && upstream != "disconnected" {
		if _, err := positiveDeviceInteger(fields, "upstream", 64); err != nil {
			return nil, fmt.Errorf("upstream は shared、disconnected、または正の整数のスイッチ ID で指定してください")
		}
	}
	return &NetworkInterface{Upstream: upstream}, nil
}

// positiveDeviceInteger は指定されたキーの値を正の整数として検証して返します。
// 値が未指定、10進数の数字以外を含む、範囲外、0以下の場合はエラーになります。
func positiveDeviceInteger(fields textjson.Fields, key string, bits int) (int64, error) {
	value := fields.Get(key)
	if value == "" {
		return 0, fmt.Errorf("%s が必要です", key)
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, fmt.Errorf("%s は正の整数で指定してください", key)
		}
	}
	number, err := strconv.ParseInt(value, 10, bits)
	if err != nil || number <= 0 {
		return 0, fmt.Errorf("%s は正の整数で指定してください", key)
	}
	return number, nil
}
