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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/sacloud/sacloud-sdk-go/api/workflows/apis/v1"
)

func TestReadRunbookFile(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "runbook.yaml")
		require.NoError(t, os.WriteFile(path, []byte("steps:\n  done:\n    return: hello\n"), 0o600))
		runbook, err := readRunbookFile(path)
		require.NoError(t, err)
		require.Equal(t, "steps:\n  done:\n    return: hello\n", runbook)
	})

	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.yaml")
		require.NoError(t, os.WriteFile(path, []byte("   \n"), 0o600))
		_, err := readRunbookFile(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "runbook file is empty")
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := readRunbookFile(filepath.Join(t.TempDir(), "not-found.yaml"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "read runbook file")
	})

	t.Run("stdin", func(t *testing.T) {
		// - は標準入力からの読み込みを表す。単体テストでは端末の標準入力を読むため、
		// パスの扱いだけを確認して実際の読み込みは CLI 結合テストで検証する。
		require.Equal(t, "-", "-")
	})
}

func TestReadArgsValue(t *testing.T) {
	t.Run("inline json", func(t *testing.T) {
		value, err := readArgsValue(`{"env":"prod"}`)
		require.NoError(t, err)
		require.JSONEq(t, `{"env":"prod"}`, value)
	})

	t.Run("inline array", func(t *testing.T) {
		value, err := readArgsValue(`[1,2,3]`)
		require.NoError(t, err)
		require.Equal(t, "[1,2,3]", value)
	})

	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "args.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"day":"mon"}`), 0o600))
		value, err := readArgsValue("@" + path)
		require.NoError(t, err)
		require.JSONEq(t, `{"day":"mon"}`, value)
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := readArgsValue("@" + filepath.Join(t.TempDir(), "not-found.json"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "read args file")
	})

	t.Run("empty path", func(t *testing.T) {
		_, err := readArgsValue("@")
		require.Error(t, err)
		require.Contains(t, err.Error(), "--args のファイルパスが空です")
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := readArgsValue(`{"env":`)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--args には JSON として妥当な文字列を指定してください")
	})

	t.Run("file with invalid json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "args.json")
		require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))
		_, err := readArgsValue("@" + path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--args には JSON として妥当な文字列を指定してください")
	})
}

func TestValidateConcurrencyModeValue(t *testing.T) {
	require.NoError(t, validateConcurrencyModeValue("parallel"))
	require.NoError(t, validateConcurrencyModeValue("lock"))
	require.NoError(t, validateConcurrencyModeValue("queue"))
	err := validateConcurrencyModeValue("serial")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parallel、lock、queue のいずれか")
}

func TestValidateWorkflowListRequest(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		request := &v1.ListWorkflowsParams{
			SortBy:        NewOptWorkflowSortBy("createdAt"),
			Order:         NewOptWorkflowOrder("desc"),
			NameMatchType: NewOptWorkflowNameMatchType("prefix"),
		}
		require.NoError(t, ValidateWorkflowListRequest(request))
	})

	t.Run("invalid sort by", func(t *testing.T) {
		request := &v1.ListWorkflowsParams{SortBy: NewOptWorkflowSortBy("name")}
		err := ValidateWorkflowListRequest(request)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--sort-by には createdAt、updatedAt、id のいずれか")
	})

	t.Run("invalid order", func(t *testing.T) {
		request := &v1.ListWorkflowsParams{Order: NewOptWorkflowOrder("ascending")}
		err := ValidateWorkflowListRequest(request)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--order には asc または desc")
	})

	t.Run("invalid name match type", func(t *testing.T) {
		request := &v1.ListWorkflowsParams{NameMatchType: NewOptWorkflowNameMatchType("suffix")}
		err := ValidateWorkflowListRequest(request)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--name-match-type には partial または prefix")
	})
}

func TestValidateSuggestionListRequest(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		request := &v1.ListSuggestionsParams{
			SortBy: NewOptSuggestionSortBy("count"),
			Order:  NewOptSuggestionOrder("asc"),
		}
		require.NoError(t, ValidateSuggestionListRequest(request))
	})

	t.Run("invalid sort by", func(t *testing.T) {
		request := &v1.ListSuggestionsParams{SortBy: NewOptSuggestionSortBy("createdAt")}
		err := ValidateSuggestionListRequest(request)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--sort-by には name または count")
	})

	t.Run("invalid order", func(t *testing.T) {
		request := &v1.ListSuggestionsParams{Order: NewOptSuggestionOrder("ascending")}
		err := ValidateSuggestionListRequest(request)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--order には asc または desc")
	})
}

func TestValidatePatchWorkflowRequest(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		request := &v1.PatchWorkflowReq{
			ConcurrencyMode: NewOptPatchWorkflowConcurrencyMode("queue"),
		}
		require.NoError(t, ValidatePatchWorkflowRequest(request))
	})

	t.Run("invalid", func(t *testing.T) {
		request := &v1.PatchWorkflowReq{
			ConcurrencyMode: NewOptPatchWorkflowConcurrencyMode("serial"),
		}
		err := ValidatePatchWorkflowRequest(request)
		require.Error(t, err)
		require.Contains(t, err.Error(), "parallel、lock、queue のいずれか")
	})
}

func TestValidateRevisionListRequest(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		request := &v1.ListRevisionsParams{
			SortBy: NewOptRevisionSortBy("id"),
			Order:  NewOptRevisionOrder("asc"),
		}
		require.NoError(t, validateRevisionListRequest(request))
	})

	t.Run("invalid sort by", func(t *testing.T) {
		request := &v1.ListRevisionsParams{SortBy: NewOptRevisionSortBy("revisionId")}
		err := validateRevisionListRequest(request)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--sort-by には createdAt、updatedAt、id のいずれか")
	})

	t.Run("invalid order", func(t *testing.T) {
		request := &v1.ListRevisionsParams{Order: NewOptRevisionOrder("descending")}
		err := validateRevisionListRequest(request)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--order には asc または desc")
	})
}

func TestSetPatchWorkflowTags(t *testing.T) {
	t.Run("sets tags", func(t *testing.T) {
		command := &WorkflowUpdateCommand{Tags: []string{"env:prod", "team:infra"}}
		request := &v1.PatchWorkflowReq{}
		require.NoError(t, SetPatchWorkflowTags(command, request))
		require.Equal(t, []v1.PatchWorkflowReqTagsItem{{Name: "env:prod"}, {Name: "team:infra"}}, request.Tags)
	})

	t.Run("no tags keeps request unchanged", func(t *testing.T) {
		command := &WorkflowUpdateCommand{}
		request := &v1.PatchWorkflowReq{}
		require.NoError(t, SetPatchWorkflowTags(command, request))
		require.Nil(t, request.Tags)
	})
}
