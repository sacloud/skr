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

package packetfilterapi

import (
	"fmt"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/packetfilter"
)

func ValidateReadRequest(request *packetfilter.ReadRequest) error {
	return validateID(request.ID)
}

func ValidateCreateRequest(request *packetfilter.CreateRequest) error {
	if request.Name == "" {
		return fmt.Errorf("Name が必要です")
	}
	return nil
}

func ValidateUpdateRequest(request *packetfilter.UpdateRequest) error {
	if err := validateID(request.ID); err != nil {
		return err
	}
	if request.Name == nil && request.Description == nil && request.Expression == nil {
		return fmt.Errorf("更新するフィールドが必要です")
	}
	if request.Expression != nil && request.OriginalExpressionHash == "" {
		return fmt.Errorf("ルールを更新する場合は OriginalExpressionHash が必要です")
	}
	return nil
}

func ValidateDeleteRequest(request *packetfilter.DeleteRequest) error {
	return validateID(request.ID)
}

func validateID(id types.ID) error {
	if id == 0 {
		return fmt.Errorf("ID が必要です")
	}
	return nil
}
