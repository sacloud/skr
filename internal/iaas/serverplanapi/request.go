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

package serverplanapi

import (
	"fmt"
	"strings"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

type FindRequest struct {
	Zone    string
	Count   int
	From    int
	Sort    []string
	Filter  map[string]any
	Include []string
	Exclude []string
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
			filterKey := filterKey(key)
			condition.Filter[filterKey] = filterValue(filterKey, value)
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
		return value
	}
	switch value := value.(type) {
	case string:
		return search.AndEqual(value)
	case []any:
		return search.OrEqual(value...)
	case []string:
		values := make([]any, len(value))
		for i, item := range value {
			values[i] = item
		}
		return search.OrEqual(values...)
	default:
		return search.OrEqual(value)
	}
}

type ReadRequest struct {
	Zone string
	ID   types.ID
}

func (request *ReadRequest) Validate() error {
	if request.ID <= 0 {
		return fmt.Errorf("ID is required")
	}
	return nil
}
