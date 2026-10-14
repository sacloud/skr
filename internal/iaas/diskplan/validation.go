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

package diskplan

import (
	"fmt"

	diskplanSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/diskplan"
	"github.com/sacloud/skr/internal/iaas/zones"
)

func ValidateRequest(name string, request any) error {
	switch name {
	case "validateDiskPlanFindRequest":
		value, ok := request.(*diskplanSDK.FindRequest)
		if !ok {
			return fmt.Errorf("Find API リクエストの型が不正です")
		}
		return validateZone(value.Zone)
	case "validateDiskPlanReadRequest":
		value, ok := request.(*diskplanSDK.ReadRequest)
		if !ok {
			return fmt.Errorf("Read API リクエストの型が不正です")
		}
		if err := validateZone(value.Zone); err != nil {
			return err
		}
		if value.ID == 0 {
			return fmt.Errorf("ID が必要です (--request JSON も指定できます)")
		}
		return nil
	default:
		return fmt.Errorf("未知の DiskPlan API リクエスト検証 %q", name)
	}
}

func validateZone(zone string) error {
	if zone == "" {
		return fmt.Errorf("Zone が必要です (--request JSON も指定できます)")
	}
	return zones.ValidateSingleZone(zone)
}
