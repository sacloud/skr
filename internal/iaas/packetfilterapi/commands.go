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
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/packetfilter"
)

// TransformCreateRequest は create 時の --expressions を API リクエストの Expression へ変換します (ADR 0042)。
func TransformCreateRequest(command *CreateCommand, request *packetfilter.CreateRequest) error {
	expressions, err := decodeExpressions(command.Expression)
	if err != nil {
		return err
	}
	if expressions != nil {
		request.Expression = expressions
	}
	return nil
}

// TransformUpdateRequest は update 時の --expressions を API リクエストの Expression へ変換します (ADR 0042)。
func TransformUpdateRequest(command *UpdateCommand, request *packetfilter.UpdateRequest) error {
	expressions, err := decodeExpressions(command.Expression)
	if err != nil {
		return err
	}
	if expressions != nil {
		request.Expression = &expressions
	}
	return nil
}

// decodeExpressions は --expressions の JSON 配列をデコードします。@path によるファイル指定にも対応します。
func decodeExpressions(inputs []string) ([]*iaas.PacketFilterExpression, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if len(inputs) != 1 {
		return nil, fmt.Errorf("--expressions は1回だけ指定できます")
	}
	data, err := requestData(inputs[0])
	if err != nil {
		return nil, fmt.Errorf("--expressions: %w", err)
	}
	var expressions []*iaas.PacketFilterExpression
	if err := json.Unmarshal(data, &expressions); err != nil {
		return nil, fmt.Errorf("--expressions: decode request JSON: %w", err)
	}
	if expressions == nil {
		return nil, fmt.Errorf("--expressions は JSON 配列で指定してください")
	}
	return expressions, nil
}

// requestData は @path 形式のファイル指定とインライン JSON の両方を読み込みます。
func requestData(input string) ([]byte, error) {
	if strings.HasPrefix(input, "@") {
		data, err := os.ReadFile(strings.TrimPrefix(input, "@"))
		if err != nil {
			return nil, fmt.Errorf("read request file: %w", err)
		}
		return data, nil
	}
	return []byte(input), nil
}
