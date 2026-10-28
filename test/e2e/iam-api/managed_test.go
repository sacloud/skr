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

// このファイルは iam-api E2E の managed resource ヘルパーです。
// SCIM、SSO、project API key の識別と後片付けを担当します。
package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
)

// managedListQuery は一覧応答をリソース ID と name に投影します。
// scim と SSO の識別子は UUID だが project API key の識別子は整数のため、
// ID は文字列に変換されます。
const managedListQuery = "{items: [.items[] | {id: (.id | tostring), name: .name}]}"

type nameID struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type nameIDList struct {
	Items []nameID `json:"items"`
}

// decodeNamedResource は {id, name} の投影をデコードします。
// ID はリソース種別によって UUID 文字列または整数の場合があります。
func decodeNamedResource(step string, data []byte) (id string, name string, err error) {
	var result struct {
		ID   any    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", "", fmt.Errorf("%s: decode resource: %w", step, err)
	}
	switch value := result.ID.(type) {
	case string:
		id = value
	case float64:
		id = strconv.FormatInt(int64(value), 10)
	default:
		return "", "", fmt.Errorf("%s: resource ID is missing", step)
	}
	if id == "" {
		return "", "", fmt.Errorf("%s: resource ID is missing", step)
	}
	return id, result.Name, nil
}

// findManaged はリソース種別を一覧表示し、指定した name の項目を返します。
func (s *iamRun) findManaged(t *testing.T, description, kind, name string) (nameID, bool, error) {
	t.Helper()
	data, _, err := s.callWithErr(t, description, "iam-api", kind, "list", "--query", managedListQuery)
	if err != nil {
		return nameID{}, false, fmt.Errorf("%s: %w", description, err)
	}
	var list nameIDList
	if err := json.Unmarshal(data, &list); err != nil {
		return nameID{}, false, fmt.Errorf("%s: decode list: %w", description, err)
	}
	for _, candidate := range list.Items {
		if candidate.Name == name {
			return candidate, true, nil
		}
	}
	return nameID{}, false, nil
}

// checkManagedListed は一覧に指定した ID と name のリソースが含まれているかどうかを報告します。
func checkManagedListed(step string, data []byte, id, name string) (bool, error) {
	var list nameIDList
	if err := json.Unmarshal(data, &list); err != nil {
		return false, fmt.Errorf("%s: decode list: %w", step, err)
	}
	for _, candidate := range list.Items {
		if candidate.ID == id {
			if candidate.Name != name {
				return false, fmt.Errorf("%s: listed name = %q, want %q", step, candidate.Name, name)
			}
			return true, nil
		}
	}
	return false, nil
}

// cleanupManaged は、managed シナリオが削除前に失敗して作成されたリソースを削除します。
// リソースの identity は削除前に読み込んで確認し、
// 削除は一覧チェックで確認します。
func (s *iamRun) cleanupManaged(t *testing.T, target *managedResource) error {
	t.Helper()
	if !target.attempted {
		return nil
	}
	id := target.id
	if id == "" {
		found, ok, err := s.findManaged(t, "後片付けとして作成を試みた"+target.kind+"を検索する", target.kind, target.currentName)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		id = found.ID
	}
	data, _, err := s.callWithErr(t, "後片付けの前に"+target.kind+"の識別情報を確認する",
		"iam-api", target.kind, "read", id, "--query", "{id: (.id | tostring), name: .name}")
	if err != nil {
		return fmt.Errorf("cannot verify %s before cleanup: %w", target.kind, err)
	}
	currentID, currentName, err := decodeNamedResource("cleanup-read-"+target.kind, data)
	if err != nil {
		return err
	}
	if currentID != id || currentName != target.currentName {
		return fmt.Errorf("cleanup refused: %s %s does not match %q", target.kind, currentID, target.currentName)
	}
	if _, _, err := s.callWithErr(t, "後片付けとして"+target.kind+"を削除する", "iam-api", target.kind, "delete", id); err != nil {
		return fmt.Errorf("delete E2E %s: %w", target.kind, err)
	}
	data, _, err = s.callWithErr(t, "後片付けの後に"+target.kind+"が残っていないことを確認する",
		"iam-api", target.kind, "list", "--query", managedListQuery)
	if err != nil {
		return fmt.Errorf("verify E2E %s deletion: %w", target.kind, err)
	}
	remaining, err := checkManagedListed("cleanup-verify-"+target.kind, data, id, target.currentName)
	if err != nil {
		return err
	}
	if remaining {
		return fmt.Errorf("E2E %s %s remains after delete", target.kind, id)
	}
	return nil
}
