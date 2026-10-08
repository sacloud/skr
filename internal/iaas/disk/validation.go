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

package diskresource

import (
	"fmt"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	"github.com/sacloud/skr/internal/iaas/zones"
)

func ValidateRequest(name string, request any) error {
	switch name {
	case "validateDiskFindRequest":
		value, ok := request.(*disk.FindRequest)
		if !ok {
			return fmt.Errorf("Find API リクエストの型が不正です")
		}
		return validateZone(value.Zone)
	case "validateDiskReadRequest":
		value, ok := request.(*disk.ReadRequest)
		if !ok {
			return fmt.Errorf("Read API リクエストの型が不正です")
		}
		return validateIdentity(value.Zone, value.ID)
	case "validateDiskCreateRequest":
		value, ok := request.(*disk.CreateRequest)
		if !ok {
			return fmt.Errorf("Create API リクエストの型が不正です")
		}
		if err := validateZone(value.Zone); err != nil {
			return err
		}
		if value.Name == "" {
			return fmt.Errorf("Name が必要です")
		}
		if value.DiskPlanID == 0 {
			return fmt.Errorf("DiskPlanID が必要です")
		}
		if value.Connection == "" {
			return fmt.Errorf("Connection が必要です")
		}
		if value.SizeGB <= 0 {
			return fmt.Errorf("SizeGB は正の値が必要です")
		}
		return nil
	case "validateDiskUpdateRequest":
		value, ok := request.(*disk.UpdateRequest)
		if !ok {
			return fmt.Errorf("Update API リクエストの型が不正です")
		}
		if err := validateIdentity(value.Zone, value.ID); err != nil {
			return err
		}
		if value.Name == nil && value.Description == nil && value.Tags == nil &&
			value.IconID == nil && value.Connection == nil && value.EditParameter == nil {
			return fmt.Errorf("更新するフィールドが必要です")
		}
		return nil
	case "validateDiskDeleteRequest":
		value, ok := request.(*disk.DeleteRequest)
		if !ok {
			return fmt.Errorf("Delete API リクエストの型が不正です")
		}
		return validateIdentity(value.Zone, value.ID)
	default:
		return fmt.Errorf("未知の Disk API リクエスト検証 %q", name)
	}
}

func validateIdentity(zone string, id types.ID) error {
	if err := validateZone(zone); err != nil {
		return err
	}
	if id == 0 {
		return fmt.Errorf("ID が必要です")
	}
	return nil
}

func validateZone(zone string) error {
	if err := validateZoneRequired(zone); err != nil {
		return err
	}
	return zones.ValidateSingleZone(zone)
}

func validateZoneRequired(zone string) error {
	if zone == "" {
		return fmt.Errorf("Zone が必要です (--request JSON も指定できます)")
	}
	return nil
}
