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

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// eventbusItem は CLI の JSON 出力を共通のリソース項目として解釈するための構造体です。
type eventbusItem struct {
	ID          json.RawMessage `json:"ID"`
	Name        string          `json:"Name"`
	Description string          `json:"Description"`
	Status      struct {
		QueueName string `json:"QueueName"`
	} `json:"Status"`
}

func (i eventbusItem) id() (string, error) {
	return decodeResourceID(i.ID)
}

// eventbusResource は今回の実行が扱うリソースの識別情報と後片付けの状態です。
type eventbusResource struct {
	kind        string
	name        string
	description string
	id          string
	// attempted は作成 API を呼び出したかどうかです。呼び出しが失敗しても後片付けの対象にします。
	attempted bool
	// deleted はシナリオ内で削除済みかどうかです。削除済みのリソースは後片付けで変更しません。
	deleted bool
}

// decodeResourceID は JSON 文字列または数値としてエンコードされたリソース ID をデコードします。
// EventBus と SimpleMQ のリソース ID は文字列、IaaS のリソース ID は数値です。
func decodeResourceID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("resource ID is missing")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decode resource ID: %w", err)
	}
	switch value := value.(type) {
	case string:
		if value == "" {
			return "", errors.New("resource ID is empty")
		}
		return value, nil
	case json.Number:
		// IaaS の未割り当て ID は 0 として返るため、空の ID として扱う。
		if value == "0" {
			return "", errors.New("resource ID is empty")
		}
		return value.String(), nil
	default:
		return "", fmt.Errorf("resource ID has unsupported JSON type %T", value)
	}
}

// decodeEventbusItem は CLI の JSON 出力を1件のリソースとして解釈します。
func decodeEventbusItem(data []byte) (eventbusItem, error) {
	var item eventbusItem
	if err := json.Unmarshal(data, &item); err != nil {
		return eventbusItem{}, fmt.Errorf("decode resource: %w", err)
	}
	if _, err := item.id(); err != nil {
		return eventbusItem{}, err
	}
	return item, nil
}

// decodeEventbusItems は一覧の JSON 出力をリソース項目の配列として解釈します。
func decodeEventbusItems(data []byte) ([]eventbusItem, error) {
	var items []eventbusItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode resource list: %w", err)
	}
	if items == nil {
		return nil, errors.New("expected a JSON array")
	}
	return items, nil
}

// kindName はリソース種別の利用者向けの名称を返します。
func kindName(kind string) string {
	switch kind {
	case "queue":
		return "SimpleMQ キュー"
	case "process-configuration":
		return "プロセス設定"
	case "trigger":
		return "トリガー"
	case "schedule":
		return "スケジュール"
	case "switch":
		return "スイッチ"
	default:
		return kind
	}
}

// listArgs は kind に対応する一覧取得のコマンド引数を返します。
func listArgs(kind string) ([]string, error) {
	switch kind {
	case "queue":
		return []string{"simplemq-api", "queue", "list"}, nil
	case "process-configuration", "trigger", "schedule":
		return []string{"eventbus-api", kind, "list"}, nil
	case "switch":
		return []string{"iaas-api", "switch", "find", "--zone", zone}, nil
	default:
		return nil, fmt.Errorf("unknown resource kind %q", kind)
	}
}

// readArgs は kind に対応する読み取りのコマンド引数を返します。
func readArgs(kind, id string) ([]string, error) {
	switch kind {
	case "queue":
		return []string{"simplemq-api", "queue", "read", id}, nil
	case "process-configuration", "trigger", "schedule":
		return []string{"eventbus-api", kind, "read", id}, nil
	case "switch":
		return []string{"iaas-api", "switch", "read", "--zone", zone, "--id", id}, nil
	default:
		return nil, fmt.Errorf("unknown resource kind %q", kind)
	}
}

// deleteArgs は kind に対応する削除のコマンド引数を返します。
// スイッチの削除は対象が見つからない場合にエラーにします。
func deleteArgs(kind, id string) ([]string, error) {
	switch kind {
	case "queue":
		return []string{"simplemq-api", "queue", "delete", id}, nil
	case "process-configuration", "trigger", "schedule":
		return []string{"eventbus-api", kind, "delete", id}, nil
	case "switch":
		return []string{"iaas-api", "switch", "delete", "--zone", zone, "--id", id, "--fail-if-not-found=true"}, nil
	default:
		return nil, fmt.Errorf("unknown resource kind %q", kind)
	}
}

// parseEventbusItem は CLI の JSON 出力を1件のリソースとして解釈します。失敗した場合はテストを停止します。
func parseEventbusItem(t *testing.T, data []byte) eventbusItem {
	t.Helper()

	item, err := decodeEventbusItem(data)
	require.NoError(t, err, "decode resource: %v", err)
	return item
}

// listEventbusItems は kind に対応するリソースの一覧を取得します。取得に失敗した場合はテストを停止します。
func listEventbusItems(t *testing.T, h *helper.Helper, description, kind string) []eventbusItem {
	t.Helper()

	items, err := lookupEventbusItems(t, h, description, kind)
	require.NoError(t, err, "%s: %v", description, err)
	return items
}

// lookupEventbusItems は kind に対応するリソースの一覧を取得します。呼び出し元がエラーを処理します。
func lookupEventbusItems(t *testing.T, h *helper.Helper, description, kind string) ([]eventbusItem, error) {
	t.Helper()

	args, err := listArgs(kind)
	if err != nil {
		return nil, err
	}
	data, stderr, err := h.CallAllowingFailure(t, description, args...)
	if err != nil {
		return nil, fmt.Errorf("%v: %s", err, stderr)
	}
	return decodeEventbusItems(data)
}

// readTypedItem は ID を指定してリソースを読み取り、Settings を含む型で解釈します。
// 読み取りに失敗した場合はテストを停止します。
func readTypedItem[T typedItem](t *testing.T, h *helper.Helper, description, kind, id string) T {
	t.Helper()

	item, err := lookupTypedItem[T](t, h, description, kind, id)
	require.NoError(t, err, "%s: %v", description, err)
	return item
}

// lookupEventbusItem は ID を指定してリソースを共通の項目型で読み取ります。
// 呼び出し元がエラーを処理します。
func lookupEventbusItem(t *testing.T, h *helper.Helper, description, kind, id string) (eventbusItem, error) {
	t.Helper()

	args, err := readArgs(kind, id)
	if err != nil {
		return eventbusItem{}, err
	}
	data, stderr, err := h.CallAllowingFailure(t, description, args...)
	if err != nil {
		return eventbusItem{}, fmt.Errorf("%v: %s", err, stderr)
	}
	return decodeEventbusItem(data)
}

// typedItem は Settings を含めて解釈するリソース項目の共通の振る舞いです。
type typedItem interface {
	id() (string, error)
}

// lookupTypedItem は ID を指定してリソースを読み取り、Settings を含む型で解釈します。
// 呼び出し元がエラーを処理します。
func lookupTypedItem[T typedItem](t *testing.T, h *helper.Helper, description, kind, id string) (T, error) {
	t.Helper()

	args, err := readArgs(kind, id)
	if err != nil {
		var zero T
		return zero, err
	}
	data, stderr, err := h.CallAllowingFailure(t, description, args...)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("%v: %s", err, stderr)
	}
	var item T
	if err := json.Unmarshal(data, &item); err != nil {
		var zero T
		return zero, fmt.Errorf("decode %s: %w", kind, err)
	}
	if _, err := item.id(); err != nil {
		var zero T
		return zero, err
	}
	return item, nil
}

// itemsNamed は一覧から指定した名前のリソースを抽出します。
func itemsNamed(items []eventbusItem, name string) []eventbusItem {
	matches := make([]eventbusItem, 0)
	for _, candidate := range items {
		if candidate.Name == name {
			matches = append(matches, candidate)
		}
	}
	return matches
}

// findEventbusItemsByName は一覧から、今回の実行が扱う名前のリソースを検索します。
func findEventbusItemsByName(t *testing.T, h *helper.Helper, description string, r *eventbusResource) []eventbusItem {
	t.Helper()

	return itemsNamed(listEventbusItems(t, h, description, r.kind), r.name)
}

// checkIdentity は参照結果の識別情報が、今回の実行で作成したリソースと一致することを確認します。
func checkIdentity(candidate eventbusItem, r *eventbusResource, id string) error {
	candidateID, err := candidate.id()
	if err != nil {
		return err
	}
	if candidateID != id || candidate.Name != r.name || candidate.Description != r.description {
		return fmt.Errorf("%s identity mismatch for ID %s", r.kind, id)
	}
	if r.kind == "queue" && candidate.Status.QueueName != r.name {
		return fmt.Errorf("SimpleMQ queue name mismatch for ID %s", id)
	}
	return nil
}

// requireEventbusIdentity は作成・参照結果が今回の作成物と一致することを確認し、ID を返します。
func requireEventbusIdentity(t *testing.T, candidate eventbusItem, r *eventbusResource) string {
	t.Helper()

	id, err := candidate.id()
	require.NoError(t, err, "decode %s ID: %v", kindName(r.kind), err)
	require.NoError(t, checkIdentity(candidate, r, id), "%s identity check failed for ID %s", kindName(r.kind), id)
	return id
}

// requireNoNameCollision は作成前に同名リソースが存在しないことを確認します。
// 一覧の取得に失敗した場合はテストを停止し、不在として扱いません。
func requireNoNameCollision(t *testing.T, h *helper.Helper, resources ...*eventbusResource) {
	t.Helper()

	for _, r := range resources {
		items := listEventbusItems(t, h,
			fmt.Sprintf("作成前に同名%sが存在しないことを確認する", kindName(r.kind)), r.kind)
		for _, candidate := range items {
			require.NotEqual(t, r.name, candidate.Name,
				"%s name %q already exists; refusing to modify it", kindName(r.kind), r.name)
		}
	}
}

// cleanupEventbusResources は依存関係の逆順でリソースを後片付けします。
// 失敗しても後続のリソースの後片付けを続け、その失敗をテストの結果へ反映します。
func cleanupEventbusResources(t *testing.T, h *helper.Helper, resources ...*eventbusResource) {
	t.Helper()

	for _, r := range resources {
		if !r.attempted || r.deleted {
			continue
		}
		cleanupEventbusResource(t, h, r)
	}
}

// cleanupEventbusResource は1件のリソースを後片付けします。
// 作成 API を呼び出していないリソースは変更しません。ID が確定できない場合は名前で検索し、
// 名前と説明が完全一致する今回の実行の作成物だけを削除します。
// 削除直前に今回の作成物と識別情報を再照合し、一致しない場合は削除しません。
func cleanupEventbusResource(t *testing.T, h *helper.Helper, r *eventbusResource) {
	t.Helper()

	label := kindName(r.kind)
	id := r.id
	if id == "" {
		// 作成 API がエラーを返しても、サーバー側で作成済みの可能性があります。
		items, err := lookupEventbusItems(t, h, fmt.Sprintf("後片付け前に%sを検索する", label), r.kind)
		if err != nil {
			t.Errorf("cleanup %s: %v", label, err)
			return
		}
		matches := itemsNamed(items, r.name)
		if len(matches) == 0 {
			return
		}
		if len(matches) != 1 || matches[0].Description != r.description {
			t.Errorf("cleanup refused: %s %q identity is not unique and marked for this E2E", label, r.name)
			return
		}
		foundID, err := matches[0].id()
		if err != nil {
			t.Errorf("cleanup %s: %v", label, err)
			return
		}
		id = foundID
	}

	// 削除前に今回の作成物と識別情報を再照合する
	current, err := lookupEventbusItem(t, h, fmt.Sprintf("削除前に%sを再照合する", label), r.kind, id)
	if err != nil {
		t.Errorf("cannot verify %s before cleanup: %v", label, err)
		return
	}
	if err := checkIdentity(current, r, id); err != nil {
		t.Errorf("cleanup refused: %v", err)
		return
	}

	if r.kind == "queue" {
		if _, stderr, err := h.CallAllowingFailure(t, "キューのメッセージをすべて削除する",
			"simplemq-api", "queue", "clear-messages", id); err != nil {
			t.Errorf("clear queue messages: %v\n%s", err, stderr)
			return
		}
	}

	args, err := deleteArgs(r.kind, id)
	if err != nil {
		t.Errorf("cleanup %s: %v", label, err)
		return
	}
	if _, stderr, err := h.CallAllowingFailure(t, fmt.Sprintf("%sを削除する", label), args...); err != nil {
		t.Errorf("delete %s: %v\n%s", label, err, stderr)
		return
	}

	// 削除後に一覧から消失したことを確認する
	items, err := lookupEventbusItems(t, h,
		fmt.Sprintf("削除後に%sが一覧に残っていないことを確認する", label), r.kind)
	if err != nil {
		t.Errorf("verify %s deletion: %v", label, err)
		return
	}
	for _, candidate := range itemsNamed(items, r.name) {
		remainingID, idErr := candidate.id()
		if idErr != nil {
			t.Errorf("verify %s deletion: %v", label, idErr)
			return
		}
		t.Errorf("%s %s remains after delete", label, remainingID)
	}
}

// TestDecodeResourceID は JSON 文字列または数値としてエンコードされたリソース ID の解釈を確認する。
func TestDecodeResourceID(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		id, err := decodeResourceID(json.RawMessage(`"test-only-id"`))
		require.NoError(t, err)
		require.Equal(t, "test-only-id", id)
	})
	t.Run("number", func(t *testing.T) {
		id, err := decodeResourceID(json.RawMessage(`123456789012`))
		require.NoError(t, err)
		require.Equal(t, "123456789012", id)
	})
	t.Run("zero number", func(t *testing.T) {
		_, err := decodeResourceID(json.RawMessage(`0`))
		require.Error(t, err, "a zero IaaS ID should be treated as an empty ID")
	})
	t.Run("empty string", func(t *testing.T) {
		_, err := decodeResourceID(json.RawMessage(`""`))
		require.Error(t, err)
	})
	t.Run("missing", func(t *testing.T) {
		_, err := decodeResourceID(nil)
		require.Error(t, err)
	})
	t.Run("unsupported type", func(t *testing.T) {
		_, err := decodeResourceID(json.RawMessage(`{"ID":1}`))
		require.Error(t, err)
	})
}

// TestDecodeEventbusItems は一覧の JSON 出力の解釈と、配列以外の拒否を確認する。
func TestDecodeEventbusItems(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		items, err := decodeEventbusItems([]byte(`[{"ID":"test-only-id","Name":"test-only-name","Description":"test-only-description"}]`))
		require.NoError(t, err)
		require.Len(t, items, 1)
		id, err := items[0].id()
		require.NoError(t, err)
		require.Equal(t, "test-only-id", id)
	})
	t.Run("not an array", func(t *testing.T) {
		_, err := decodeEventbusItems([]byte(`null`))
		require.Error(t, err, "a null list should not be treated as an empty list")
	})
	t.Run("invalid json", func(t *testing.T) {
		_, err := decodeEventbusItems([]byte(`not-json`))
		require.Error(t, err)
	})
}

// TestCheckIdentity は参照結果と今回の作成物の識別情報照合を確認する。
func TestCheckIdentity(t *testing.T) {
	r := &eventbusResource{
		kind:        "queue",
		name:        "test-only-name",
		description: "test-only-description",
		id:          "test-only-id",
	}

	t.Run("ok", func(t *testing.T) {
		var candidate eventbusItem
		require.NoError(t, json.Unmarshal([]byte(`{"ID":"test-only-id","Name":"test-only-name","Description":"test-only-description","Status":{"QueueName":"test-only-name"}}`), &candidate))
		require.NoError(t, checkIdentity(candidate, r, r.id))
	})
	t.Run("ok without status", func(t *testing.T) {
		// SimpleMQ キュー以外は Status の照合を行わない
		other := &eventbusResource{kind: "trigger", name: r.name, description: r.description, id: r.id}
		var candidate eventbusItem
		require.NoError(t, json.Unmarshal([]byte(`{"ID":"test-only-id","Name":"test-only-name","Description":"test-only-description"}`), &candidate))
		require.NoError(t, checkIdentity(candidate, other, r.id))
	})
	t.Run("ID mismatch", func(t *testing.T) {
		var candidate eventbusItem
		require.NoError(t, json.Unmarshal([]byte(`{"ID":"other-id","Name":"test-only-name","Description":"test-only-description"}`), &candidate))
		require.Error(t, checkIdentity(candidate, r, r.id))
	})
	t.Run("name mismatch", func(t *testing.T) {
		var candidate eventbusItem
		require.NoError(t, json.Unmarshal([]byte(`{"ID":"test-only-id","Name":"other-name","Description":"test-only-description"}`), &candidate))
		require.Error(t, checkIdentity(candidate, r, r.id))
	})
	t.Run("description mismatch", func(t *testing.T) {
		var candidate eventbusItem
		require.NoError(t, json.Unmarshal([]byte(`{"ID":"test-only-id","Name":"test-only-name","Description":"other-description"}`), &candidate))
		require.Error(t, checkIdentity(candidate, r, r.id))
	})
	t.Run("queue name mismatch", func(t *testing.T) {
		var candidate eventbusItem
		require.NoError(t, json.Unmarshal([]byte(`{"ID":"test-only-id","Name":"test-only-name","Description":"test-only-description","Status":{"QueueName":"other-queue"}}`), &candidate))
		require.Error(t, checkIdentity(candidate, r, r.id))
	})
}
