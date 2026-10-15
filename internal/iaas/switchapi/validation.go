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

package switchapi

import (
	"fmt"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
)

func ValidateReadRequest(request *swytch.ReadRequest) error {
	return validateReadRequest(request)
}

func ValidateCreateRequest(request *swytch.CreateRequest) error {
	return validateCreateRequest(request)
}

func ValidateUpdateRequest(request *swytch.UpdateRequest) error {
	return validateUpdateRequest(request)
}

func ValidateDeleteRequest(request *swytch.DeleteRequest) error {
	return validateDeleteRequest(request)
}

func validateReadRequest(request *swytch.ReadRequest) error {
	return validateID(request.ID)
}

func validateCreateRequest(request *swytch.CreateRequest) error {
	if request.Name == "" {
		return fmt.Errorf("--name が必要です")
	}
	return nil
}

func validateUpdateRequest(request *swytch.UpdateRequest) error {
	return validateID(request.ID)
}

func validateDeleteRequest(request *swytch.DeleteRequest) error {
	return validateID(request.ID)
}

func validateID(id types.ID) error {
	if id == 0 {
		return fmt.Errorf("--id が必要です (--request JSON も指定できます)")
	}
	return nil
}
