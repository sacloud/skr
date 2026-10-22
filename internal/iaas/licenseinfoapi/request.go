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

package licenseinfoapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
)

type FindRequest struct {
	Count   int
	From    int
	Sort    []string
	Filter  map[string]any
	Include []string
	Exclude []string
}

func (request *FindRequest) UnmarshalJSON(data []byte) error {
	type findRequestAlias FindRequest
	var decoded struct {
		*findRequestAlias
		Filter json.RawMessage `json:"Filter"`
	}
	decoded.findRequestAlias = (*findRequestAlias)(request)
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if len(decoded.Filter) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded.Filter))
	decoder.UseNumber()
	if err := decoder.Decode(&request.Filter); err != nil {
		return fmt.Errorf("decode Filter: %w", err)
	}
	return nil
}

func (request *FindRequest) findCondition() *iaas.FindCondition {
	condition := &iaas.FindCondition{
		Count:   request.Count,
		From:    request.From,
		Include: request.Include,
		Exclude: request.Exclude,
	}
	if len(request.Filter) > 0 {
		condition.Filter = make(search.Filter, len(request.Filter))
		for key, value := range request.Filter {
			filter := filterKey(key)
			condition.Filter[filter] = filterValue(filter, value)
		}
	}
	if len(request.Sort) > 0 {
		condition.Sort = make(search.SortKeys, 0, len(request.Sort))
		for _, key := range request.Sort {
			if strings.HasPrefix(key, "-") {
				condition.Sort = append(condition.Sort, search.SortKeyDesc(strings.TrimPrefix(key, "-")))
			} else {
				condition.Sort = append(condition.Sort, search.SortKeyAsc(key))
			}
		}
	}
	return condition
}

func filterKey(value string) search.FilterKey {
	for _, operator := range []search.ComparisonOperator{
		search.OpGreaterEqual,
		search.OpLessEqual,
		search.OpGreaterThan,
		search.OpLessThan,
	} {
		if strings.HasSuffix(value, string(operator)) {
			return search.KeyWithOp(strings.TrimSuffix(value, string(operator)), operator)
		}
	}
	return search.Key(value)
}

func filterValue(key search.FilterKey, value any) any {
	if key.Op != search.OpEqual {
		return normalizeFilterNumbers(value)
	}
	switch value := value.(type) {
	case string:
		return search.AndEqual(value)
	case []any:
		values := make([]any, len(value))
		for i, item := range value {
			values[i] = normalizeFilterNumbers(item)
		}
		return search.OrEqual(values...)
	case []string:
		values := make([]any, len(value))
		for i, item := range value {
			values[i] = item
		}
		return search.OrEqual(values...)
	default:
		return search.OrEqual(normalizeFilterNumbers(value))
	}
}

func normalizeFilterNumbers(value any) any {
	switch value := value.(type) {
	case json.Number:
		return value.String()
	case []any:
		values := make([]any, len(value))
		for i, item := range value {
			values[i] = normalizeFilterNumbers(item)
		}
		return values
	case map[string]any:
		values := make(map[string]any, len(value))
		for key, item := range value {
			values[key] = normalizeFilterNumbers(item)
		}
		return values
	default:
		return value
	}
}

type FindResult struct {
	Total       int                 `json:"Total"`
	From        int                 `json:"From"`
	Count       int                 `json:"Count"`
	LicenseInfo []*iaas.LicenseInfo `json:"LicenseInfo"`
}
