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

package diskapi

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (request *CreateRequest) UnmarshalJSON(data []byte) error {
	if err := rejectUnsupportedFields(data, "OSType", "EditParameter", "NoWait"); err != nil {
		return err
	}
	type createRequest CreateRequest
	var decoded createRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*request = CreateRequest(decoded)
	return nil
}

func (request *UpdateRequest) UnmarshalJSON(data []byte) error {
	if err := rejectUnsupportedFields(data, "EditParameter", "NoWait"); err != nil {
		return err
	}
	type updateRequest UpdateRequest
	var decoded updateRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*request = UpdateRequest(decoded)
	return nil
}

func rejectUnsupportedFields(data []byte, unsupported ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, field := range unsupported {
		for inputField := range fields {
			if strings.EqualFold(inputField, field) {
				switch field {
				case "OSType":
					return fmt.Errorf("OSType はこの作成操作ではサポートしていません。コピー元アーカイブを SourceArchiveID で指定してください")
				case "EditParameter":
					return fmt.Errorf("EditParameter はこの作成・更新操作ではサポートしていません")
				case "NoWait":
					return fmt.Errorf("NoWait はこの作成・更新操作ではサポートしていません")
				}
			}
		}
	}
	return nil
}
