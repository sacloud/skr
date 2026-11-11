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

package eventbus

import (
	"fmt"

	"github.com/sacloud/skr/internal/textjson"
	eventbusapi "github.com/sacloud/skr/service/eventbus-service-go"
)

// ParseConditions は反復可能な --condition の値をライブラリの TriggerCondition へ変換します。
// 各値は 1 条件を表し、skr 全体で統一された text → JSON 簡易表記 (ADR 0078) の
// カンマ区切りの key=value リストとして解析します。対応キーは key と op (各1回) と
// value (1回以上) です。解析後の意味検証 (key の pattern、eq/in と値数の整合) は
// service 層が行います。
func ParseConditions(inputs []string) ([]eventbusapi.TriggerCondition, error) {
	if inputs == nil {
		return nil, nil
	}
	result := make([]eventbusapi.TriggerCondition, 0, len(inputs))
	for _, input := range inputs {
		fields, err := textjson.Parse(input, []textjson.Field{
			{Name: "key", Required: true},
			{Name: "op", Required: true},
			{Name: "value", Required: true, Repeated: true},
		})
		if err != nil {
			return nil, fmt.Errorf("--condition: %w", err)
		}
		result = append(result, eventbusapi.TriggerCondition{
			Key:    fields.Get("key"),
			Op:     fields.Get("op"),
			Values: fields.GetValues("value"),
		})
	}
	return result, nil
}
