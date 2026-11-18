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

package workflowsapi

import (
	"fmt"

	v1 "github.com/sacloud/sacloud-sdk-go/api/workflows/apis/v1"
)

// このファイルには生成コマンドから呼び出される入力変換と検証を実装します。
// 列挙値の検証は API に送る前に CLI で行い、利用者が誤りに気づきやすくします。

func NewOptWorkflowSortBy(value string) v1.OptListWorkflowsSortBy {
	return v1.NewOptListWorkflowsSortBy(v1.ListWorkflowsSortBy(value))
}

func NewOptWorkflowOrder(value string) v1.OptListWorkflowsOrder {
	return v1.NewOptListWorkflowsOrder(v1.ListWorkflowsOrder(value))
}

func NewOptWorkflowNameMatchType(value string) v1.OptListWorkflowsNameMatchType {
	return v1.NewOptListWorkflowsNameMatchType(v1.ListWorkflowsNameMatchType(value))
}

func NewOptSuggestionSortBy(value string) v1.OptListSuggestionsSortBy {
	return v1.NewOptListSuggestionsSortBy(v1.ListSuggestionsSortBy(value))
}

func NewOptSuggestionOrder(value string) v1.OptListSuggestionsOrder {
	return v1.NewOptListSuggestionsOrder(v1.ListSuggestionsOrder(value))
}

func NewOptRevisionSortBy(value string) v1.OptListRevisionsSortBy {
	return v1.NewOptListRevisionsSortBy(v1.ListRevisionsSortBy(value))
}

func NewOptRevisionOrder(value string) v1.OptListRevisionsOrder {
	return v1.NewOptListRevisionsOrder(v1.ListRevisionsOrder(value))
}

func NewOptExecutionOrder(value string) v1.OptListExecutionsOrder {
	return v1.NewOptListExecutionsOrder(v1.ListExecutionsOrder(value))
}

func NewOptExecutionHistorySortOrder(value string) v1.OptListExecutionHistoriesSortOrder {
	return v1.NewOptListExecutionHistoriesSortOrder(v1.ListExecutionHistoriesSortOrder(value))
}

func NewOptWorkflowConcurrencyMode(value string) v1.OptCreateWorkflowReqConcurrencyMode {
	return v1.NewOptCreateWorkflowReqConcurrencyMode(v1.CreateWorkflowReqConcurrencyMode(value))
}

func NewOptPatchWorkflowConcurrencyMode(value string) v1.OptPatchWorkflowReqConcurrencyMode {
	return v1.NewOptPatchWorkflowReqConcurrencyMode(v1.PatchWorkflowReqConcurrencyMode(value))
}

// ValidateWorkflowListRequest は list-workflows の列挙値フラグを検証します。
func ValidateWorkflowListRequest(request *v1.ListWorkflowsParams) error {
	if request.SortBy.Set {
		switch request.SortBy.Value {
		case v1.ListWorkflowsSortByCreatedAt, v1.ListWorkflowsSortByUpdatedAt, v1.ListWorkflowsSortByID:
		default:
			return fmt.Errorf("--sort-by には createdAt、updatedAt、id のいずれかを指定してください")
		}
	}
	if request.Order.Set {
		switch request.Order.Value {
		case v1.ListWorkflowsOrderAsc, v1.ListWorkflowsOrderDesc:
		default:
			return fmt.Errorf("--order には asc または desc を指定してください")
		}
	}
	if request.NameMatchType.Set {
		switch request.NameMatchType.Value {
		case v1.ListWorkflowsNameMatchTypePartial, v1.ListWorkflowsNameMatchTypePrefix:
		default:
			return fmt.Errorf("--name-match-type には partial または prefix を指定してください")
		}
	}
	return nil
}

// ValidateSuggestionListRequest は list-suggestions の列挙値フラグを検証します。
func ValidateSuggestionListRequest(request *v1.ListSuggestionsParams) error {
	if request.SortBy.Set {
		switch request.SortBy.Value {
		case v1.ListSuggestionsSortByName, v1.ListSuggestionsSortByCount:
		default:
			return fmt.Errorf("--sort-by には name または count を指定してください")
		}
	}
	if request.Order.Set {
		switch request.Order.Value {
		case v1.ListSuggestionsOrderAsc, v1.ListSuggestionsOrderDesc:
		default:
			return fmt.Errorf("--order には asc または desc を指定してください")
		}
	}
	return nil
}

// ValidatePatchWorkflowRequest は patch-workflow の同時実行制御の値を検証します。
func ValidatePatchWorkflowRequest(request *v1.PatchWorkflowReq) error {
	if request.ConcurrencyMode.Set {
		switch request.ConcurrencyMode.Value {
		case v1.PatchWorkflowReqConcurrencyModeParallel, v1.PatchWorkflowReqConcurrencyModeLock, v1.PatchWorkflowReqConcurrencyModeQueue:
		default:
			return fmt.Errorf("--concurrency-mode には parallel、lock、queue のいずれかを指定してください")
		}
	}
	return nil
}

// SetPatchWorkflowTags は --tags の反復入力を API の Tags 配列に変換します。
// 個別フラグ経路では必ず呼ばれますが、--tags を省略した場合は Tags をリクエストに含めません。
func SetPatchWorkflowTags(c *WorkflowUpdateCommand, request *v1.PatchWorkflowReq) error {
	if len(c.Tags) == 0 {
		return nil
	}
	tags := make([]v1.PatchWorkflowReqTagsItem, 0, len(c.Tags))
	for _, name := range c.Tags {
		tags = append(tags, v1.PatchWorkflowReqTagsItem{Name: name})
	}
	request.Tags = tags
	return nil
}
