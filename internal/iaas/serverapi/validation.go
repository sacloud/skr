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

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/server"
)

func ValidateReadRequest(request *server.ReadRequest) error {
	return validateID(request.ID)
}

func ValidateCreateRequest(request *server.CreateRequest) error {
	if request.Name == "" {
		return fmt.Errorf("Name が必要です")
	}
	if request.CPU <= 0 || request.MemoryGB <= 0 {
		return fmt.Errorf("対象ゾーンで利用可能な CPU と MemoryGB の組み合わせが必要です")
	}
	if request.NoWait && request.BootAfterCreate {
		return fmt.Errorf("NoWait=true と BootAfterCreate=true は併用できません")
	}
	return nil
}

func ValidateBootRequest(request *server.BootRequest) error {
	if err := validateID(request.ID); err != nil {
		return err
	}
	return request.Validate()
}

func ValidateUpdateRequest(request *server.UpdateRequest) error {
	if err := validateID(request.ID); err != nil {
		return err
	}
	if request.Name == nil && request.Description == nil && request.Tags == nil &&
		request.IconID == nil && request.CPU == nil && request.MemoryGB == nil &&
		request.GPU == nil && request.GPUModel == nil && request.CPUModel == nil &&
		request.Commitment == nil && request.Generation == nil && request.InterfaceDriver == nil &&
		request.CDROMID == nil && request.PrivateHostID == nil &&
		request.NetworkInterfaces == nil && request.Disks == nil {
		return fmt.Errorf("更新するフィールドが必要です")
	}
	return nil
}

func ValidateDeleteRequest(request *server.DeleteRequest) error {
	return validateID(request.ID)
}

func validateID(id types.ID) error {
	if id == 0 {
		return fmt.Errorf("ID が必要です")
	}
	return nil
}
