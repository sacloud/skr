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

package archiveapi

import (
	"fmt"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/ostype"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/archive"
)

func ValidateFindRequest(request *archive.FindRequest) error {
	return validateFindRequest(request)
}

func ValidateReadRequest(request *archive.ReadRequest) error {
	if request.ID == 0 {
		return fmt.Errorf("ID が必要です (--request JSON も指定できます)")
	}
	return nil
}

func validateFindRequest(request *archive.FindRequest) error {
	if request.OSType < 0 || !validOSType(request.OSType) {
		return fmt.Errorf("OSType が不正です")
	}
	if request.OSType != ostype.Custom && (len(request.Names) > 0 || len(request.Tags) > 0) {
		return fmt.Errorf("OSType は Names または Tags と併用できません")
	}
	return nil
}

func validOSType(value ostype.ArchiveOSType) bool {
	if value == ostype.Custom {
		return true
	}
	for _, candidate := range ostype.ArchiveOSTypes {
		if value == candidate {
			return true
		}
	}
	return false
}
