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
	"strings"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/server"
)

func parseDeviceFields(input string, allowed ...string) (map[string]string, error) {
	fields := make(map[string]string)
	for _, part := range strings.Split(input, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" || value == "" || strings.Contains(value, "=") {
			return nil, fmt.Errorf("各要素は空でない key=value 形式で指定してください")
		}
		found := false
		for _, candidate := range allowed {
			found = found || key == candidate
		}
		if !found {
			return nil, fmt.Errorf("未対応のキーです。指定可能なキー: %s", strings.Join(allowed, ", "))
		}
		if _, exists := fields[key]; exists {
			return nil, fmt.Errorf("%s は重複して指定できません", key)
		}
		fields[key] = value
	}
	return fields, nil
}

func positiveDeviceInteger(fields map[string]string, key string, bits int) (int64, error) {
	value, exists := fields[key]
	if !exists {
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

func parseDisk(input, name string) (*disk.ApplyRequest, error) {
	fields, err := parseDeviceFields(input, "archive-id", "plan-id", "size-gb")
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
	if _, exists := fields["archive-id"]; exists {
		archive, err = positiveDeviceInteger(fields, "archive-id", 64)
		if err != nil {
			return nil, err
		}
	}
	return &disk.ApplyRequest{
		Name: name, DiskPlanID: types.ID(plan), SizeGB: int(size),
		SourceArchiveID: types.ID(archive), Connection: types.DiskConnections.VirtIO,
	}, nil
}

func parseNIC(input string) (*server.NetworkInterface, error) {
	fields, err := parseDeviceFields(input, "upstream")
	if err != nil {
		return nil, err
	}
	upstream, exists := fields["upstream"]
	if !exists {
		return nil, fmt.Errorf("upstream が必要です")
	}
	if upstream != "shared" && upstream != "disconnected" {
		if _, err := positiveDeviceInteger(fields, "upstream", 64); err != nil {
			return nil, fmt.Errorf("upstream は shared、disconnected、または正の整数のスイッチ ID で指定してください")
		}
	}
	return &server.NetworkInterface{Upstream: upstream}, nil
}
