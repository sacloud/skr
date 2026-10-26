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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/e2e"
)

// このシナリオは以下の流れでコンテナレジストリとそのユーザーのライフサイクルを検証します。
//
// 1. プロファイルの選択確認
// 2. 同名レジストリが存在しないことを確認
// 3. レジストリを作成
// 4. 作成されたレジストリが一覧に含まれることを確認
// 5. レジストリの説明を更新
// 6. 更新後のレジストリを参照
// 7. レジストリユーザーにアクセス権を追加
// 8. ユーザーの権限を更新
// 9. ユーザーを削除
// 10. 最後に作成したレジストリとユーザーを削除
//
// 途中で失敗した場合は、作成が試行されたレジストリとそのユーザーだけをクリーンアップします。

const (
	registryPrefix    = "skr-e2e-cr-"
	descriptionPrefix = "skr-e2e-container-registry/"
	userName          = "skr-e2e"
	queryRegistries   = "map({ID,Name,SubDomainLabel,Description})"
	queryUsers        = "map({UserName,Permission})"
)

// cli はシナリオが使用する CLI 呼び出しの抽象です。テストで Runner をモック可能にするため定義します。
// Call はトップレベルコマンド、CallSubcommand は container-registry-api 配下のサブコマンドに対応します。
type cli interface {
	Call(ctx context.Context, step, description string, args ...string) ([]byte, error)
	CallSubcommand(ctx context.Context, step, description string, args ...string) ([]byte, error)
}

type registryItem struct {
	ID           json.RawMessage `json:"ID"`
	Name         string          `json:"Name"`
	RegistryName string          `json:"SubDomainLabel"`
	Description  string          `json:"Description"`
}

func (item registryItem) id() (string, error) {
	if len(item.ID) == 0 {
		return "", errors.New("container registry ID is missing")
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(item.ID))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("decode container registry ID: %w", err)
	}
	switch value := value.(type) {
	case string:
		if value == "" {
			return "", errors.New("container registry ID is empty")
		}
		return value, nil
	case json.Number:
		if value == "0" {
			return "", errors.New("container registry ID is empty")
		}
		return value.String(), nil
	default:
		return "", fmt.Errorf("container registry ID has unsupported JSON type %T", value)
	}
}

type userItem struct {
	UserName   string `json:"UserName"`
	Permission string `json:"Permission"`
}

type scenario struct {
	client       cli
	requestDir   string
	passwordFile string
}

func (s scenario) run(ctx context.Context, name, description string) (result error) {
	if err := s.checkProfile(ctx); err != nil {
		return err
	}

	findRequest, err := e2e.WriteRequest(s.requestDir, "find", map[string]any{"Names": []string{name}})
	if err != nil {
		return err
	}
	before, err := s.find(ctx, "e2e-preflight-list", "作成前に同名レジストリが存在しないことを確認する", findRequest, name)
	if err != nil {
		return err
	}
	if len(before) != 0 {
		return fmt.Errorf("registry name %q already exists; refusing to modify it", name)
	}

	updatedDescription := description + "/updated"
	creationAttempted := false
	var id string
	defer func() {
		if !creationAttempted {
			return
		}
		if cleanupErr := s.cleanup(name, description, updatedDescription, id); cleanupErr != nil {
			result = errors.Join(result, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
	}()

	createRequest, err := e2e.WriteRequest(s.requestDir, "create", map[string]any{
		"Name":        name,
		"Description": description,
	})
	if err != nil {
		return err
	}
	creationAttempted = true
	data, err := s.client.CallSubcommand(ctx, "e2e-create", "コンテナレジストリを作成する",
		"registry", "create", "--request", "@"+createRequest)
	if err != nil {
		return err
	}
	var created registryItem
	if err := json.Unmarshal(data, &created); err != nil {
		return fmt.Errorf("decode created container registry: %w", err)
	}
	id, err = created.id()
	if err != nil {
		return err
	}
	if created.Name != name || created.RegistryName != name || created.Description != description {
		return errors.New("created container registry does not match the requested identity")
	}

	listed, err := s.find(ctx, "e2e-list-created", "作成したレジストリが一覧に含まれることを確認する", findRequest, name)
	if err != nil {
		return err
	}
	if len(listed) != 1 {
		return fmt.Errorf("registry list returned %d matches for %q; want one", len(listed), name)
	}
	if err := checkRegistry(listed[0], id, name, description, updatedDescription); err != nil {
		return err
	}

	updateRequest, err := e2e.WriteRequest(s.requestDir, "update", map[string]any{"Description": updatedDescription})
	if err != nil {
		return err
	}
	data, err = s.client.CallSubcommand(ctx, "e2e-update", "レジストリの説明を更新する",
		"registry", "update", id, "--request", "@"+updateRequest)
	if err != nil {
		return err
	}
	var updated registryItem
	if err := json.Unmarshal(data, &updated); err != nil {
		return fmt.Errorf("decode updated container registry: %w", err)
	}
	if err := checkRegistry(updated, id, name, description, updatedDescription); err != nil {
		return err
	}
	if updated.Description != updatedDescription {
		return errors.New("container registry description was not updated")
	}

	current, err := s.read(ctx, "e2e-read-updated", "更新後のレジストリ詳細を確認する", id)
	if err != nil {
		return err
	}
	if err := checkRegistry(current, id, name, updatedDescription, updatedDescription); err != nil {
		return err
	}

	if _, err := s.client.CallSubcommand(ctx, "e2e-add-user", "レジストリにユーザーを追加する",
		"registry", "user", "add", id,
		"--user-name", userName, "--password-file", s.passwordFile, "--permission", "readwrite"); err != nil {
		return err
	}
	users, err := s.listUsers(ctx, "e2e-list-user", "追加したユーザーが一覧に含まれることを確認する", id)
	if err != nil {
		return err
	}
	if len(users) != 1 || users[0].UserName != userName || users[0].Permission != "readwrite" {
		return errors.New("user list does not match the user created by this run")
	}

	if _, err := s.client.CallSubcommand(ctx, "e2e-update-user", "ユーザーの権限を変更する",
		"registry", "user", "update", id, userName, "--permission", "readonly"); err != nil {
		return err
	}
	users, err = s.listUsers(ctx, "e2e-list-updated-user", "権限変更がユーザーに反映されていることを確認する", id)
	if err != nil {
		return err
	}
	if len(users) != 1 || users[0].UserName != userName || users[0].Permission != "readonly" {
		return errors.New("user list does not reflect the permission update")
	}

	if _, err := s.client.CallSubcommand(ctx, "e2e-delete-user", "ユーザーを削除する",
		"registry", "user", "delete", id, userName); err != nil {
		return err
	}
	users, err = s.listUsers(ctx, "e2e-list-deleted-user", "ユーザー削除後にユーザーが残っていないことを確認する", id)
	if err != nil {
		return err
	}
	if len(users) != 0 {
		return errors.New("user remains after deletion")
	}
	return nil
}

func (s scenario) checkProfile(ctx context.Context) error {
	profile, err := s.client.Call(ctx, "e2e-profile-current", "選択されている SDK プロファイルを確認する", "config", "current")
	if err != nil {
		return fmt.Errorf("a selected SDK profile is required: %w", err)
	}
	if strings.TrimSpace(string(profile)) == "" {
		return errors.New("selected SDK profile name is empty")
	}
	fmt.Printf("Selected SDK profile: %s", profile)
	if !strings.HasSuffix(string(profile), "\n") {
		fmt.Println()
	}
	return nil
}

func (s scenario) find(ctx context.Context, step, description, requestPath, name string) ([]registryItem, error) {
	items, err := s.list(ctx, step, description, requestPath)
	if err != nil {
		return nil, err
	}
	matches := make([]registryItem, 0, len(items))
	for _, item := range items {
		if item.Name == name {
			matches = append(matches, item)
		}
	}
	return matches, nil
}

func (s scenario) list(ctx context.Context, step, description, requestPath string) ([]registryItem, error) {
	data, err := s.client.CallSubcommand(ctx, step, description,
		"registry", "list", "--request", "@"+requestPath, "--query", queryRegistries)
	if err != nil {
		return nil, err
	}
	var items []registryItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("%s: decode projected registries: %w", step, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s: expected a JSON registry array", step)
	}
	return items, nil
}

func (s scenario) read(ctx context.Context, step, description, id string) (registryItem, error) {
	data, err := s.client.CallSubcommand(ctx, step, description, "registry", "read", id)
	if err != nil {
		return registryItem{}, err
	}
	var item registryItem
	if err := json.Unmarshal(data, &item); err != nil {
		return registryItem{}, fmt.Errorf("%s: decode container registry: %w", step, err)
	}
	if _, err := item.id(); err != nil {
		return registryItem{}, fmt.Errorf("%s: %w", step, err)
	}
	return item, nil
}

func (s scenario) listUsers(ctx context.Context, step, description, registryID string) ([]userItem, error) {
	data, err := s.client.CallSubcommand(ctx, step, description,
		"registry", "user", "list", registryID, "--query", queryUsers)
	if err != nil {
		return nil, err
	}
	var users []userItem
	if err := json.Unmarshal(data, &users); err != nil {
		return nil, fmt.Errorf("%s: decode projected users: %w", step, err)
	}
	if users == nil {
		return nil, fmt.Errorf("%s: expected a JSON user array", step)
	}
	return users, nil
}

func checkRegistry(item registryItem, id, name, originalDescription, updatedDescription string) error {
	itemID, err := item.id()
	if err != nil {
		return err
	}
	if itemID != id || item.Name != name || item.RegistryName != name ||
		(item.Description != originalDescription && item.Description != updatedDescription) {
		return fmt.Errorf("container registry identity mismatch for ID %s; refusing cleanup", id)
	}
	return nil
}

func (s scenario) cleanup(name, originalDescription, updatedDescription, knownID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	findRequest, err := e2e.WriteRequest(s.requestDir, "cleanup-find", map[string]any{"Names": []string{name}})
	if err != nil {
		return err
	}
	matches, err := s.find(ctx, "e2e-cleanup-list", "クリーンアップ対象のレジストリを特定する", findRequest, name)
	if err != nil {
		return fmt.Errorf("cannot locate possibly created container registry: %w", err)
	}
	if len(matches) == 0 {
		return nil
	}
	if len(matches) != 1 {
		return errors.New("cannot safely identify E2E container registry; manual cleanup required")
	}
	id, err := matches[0].id()
	if err != nil {
		return fmt.Errorf("cannot safely identify E2E container registry: %w", err)
	}
	if knownID != "" && knownID != id {
		return errors.New("container registry ID differs from the ID created by this run; refusing cleanup")
	}
	if err := checkRegistry(matches[0], id, name, originalDescription, updatedDescription); err != nil {
		return err
	}

	current, err := s.read(ctx, "e2e-cleanup-read", "削除前にレジストリの identity を再確認する", id)
	if err != nil {
		return fmt.Errorf("cannot verify container registry before cleanup: %w", err)
	}
	if err := checkRegistry(current, id, name, originalDescription, updatedDescription); err != nil {
		return err
	}

	users, err := s.listUsers(ctx, "e2e-cleanup-users", "削除前にレジストリのユーザー一覧を確認する", id)
	if err != nil {
		return fmt.Errorf("cannot verify users before cleanup: %w", err)
	}
	if len(users) > 1 || (len(users) == 1 && users[0].UserName != userName) {
		return errors.New("container registry contains an unexpected user; refusing cleanup")
	}
	if len(users) == 1 {
		if _, err := s.client.CallSubcommand(ctx, "e2e-cleanup-delete-user", "テストで作成したユーザーを削除する",
			"registry", "user", "delete", id, userName); err != nil {
			return fmt.Errorf("delete E2E container registry user: %w", err)
		}
		users, err = s.listUsers(ctx, "e2e-cleanup-users-after", "ユーザー削除後のユーザー一覧を確認する", id)
		if err != nil {
			return fmt.Errorf("cannot verify user deletion: %w", err)
		}
		if len(users) != 0 {
			return errors.New("E2E container registry user remains after deletion")
		}
	}

	current, err = s.read(ctx, "e2e-cleanup-read-before-delete", "レジストリ削除前に最終確認する", id)
	if err != nil {
		return fmt.Errorf("cannot recheck container registry before deletion: %w", err)
	}
	if err := checkRegistry(current, id, name, originalDescription, updatedDescription); err != nil {
		return err
	}
	if _, err := s.client.CallSubcommand(ctx, "e2e-cleanup-delete", "テストで作成したレジストリを削除する",
		"registry", "delete", id); err != nil {
		return fmt.Errorf("delete E2E container registry: %w", err)
	}
	remaining, err := s.find(ctx, "e2e-cleanup-list-after", "削除後にレジストリが一覧に残っていないことを確認する", findRequest, name)
	if err != nil {
		return fmt.Errorf("container registry absence could not be verified: %w", err)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("E2E container registry %s remains after deletion", id)
	}
	return nil
}
