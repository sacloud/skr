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
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// このシナリオは、一時的なプロファイルディレクトリで以下の流れを検証します。
// 既存のプロファイルは参照・変更しません。
//
// 1. 新しいプロファイルを作成し、同時に current に設定する
// 2. 作成直後の current プロファイル名が期待値であることを確認する
// 3. 2 つ目のプロファイルを作成する
// 4. 2 つ目のプロファイルを current に切り替える
// 5. use 後の current プロファイル名が切り替わっていることを確認する
// 6. 作成した 2 つのプロファイルが一覧に含まれることを確認する
// 7. show 出力に作成時の属性が含まれ、秘密情報がマスクされていることを確認する
// 8. ServicePrincipalID を編集する
// 9. 編集後の属性が show 出力に反映されていることを確認する

const (
	profileName       = "skr-e2e-config"
	secondProfileName = "skr-e2e-config-second"
)

// cli はシナリオが使用する CLI 呼び出しの抽象です。テストで Runner をモック可能にするため定義します。
type cli interface {
	Call(ctx context.Context, step, description string, args ...string) ([]byte, error)
}

type scenario struct {
	client  cli
	keyPath string
}

func (s scenario) run(ctx context.Context) error {
	if _, err := s.client.Call(ctx, "c0nfig-create", "新しいプロファイルを作成し、同時に current に設定する",
		"config", "create", profileName,
		"--service-principal-id", "spid-e2e",
		"--service-principal-key-id", "kid-e2e",
		"--private-key-file", s.keyPath,
		"--use",
	); err != nil {
		return fmt.Errorf("create profile: %w", err)
	}

	current, err := s.client.Call(ctx, "c0nfig-current", "作成直後の current プロファイル名が期待値であることを確認する", "config", "current")
	if err != nil {
		return fmt.Errorf("get current profile: %w", err)
	}
	if strings.TrimSpace(string(current)) != profileName {
		return fmt.Errorf("current profile = %q, want %q", string(current), profileName)
	}

	if _, err := s.client.Call(ctx, "c0nfig-create-second", "2 つ目のプロファイルを作成する",
		"config", "create", secondProfileName,
		"--service-principal-id", "spid-e2e",
		"--service-principal-key-id", "kid-e2e",
		"--private-key-file", s.keyPath,
	); err != nil {
		return fmt.Errorf("create second profile: %w", err)
	}

	if _, err := s.client.Call(ctx, "c0nfig-use", "2 つ目のプロファイルを current に切り替える",
		"config", "use", secondProfileName,
	); err != nil {
		return fmt.Errorf("use profile: %w", err)
	}

	current, err = s.client.Call(ctx, "c0nfig-current-after-use", "use 後の current プロファイル名が切り替わっていることを確認する", "config", "current")
	if err != nil {
		return fmt.Errorf("get current profile after use: %w", err)
	}
	if strings.TrimSpace(string(current)) != secondProfileName {
		return fmt.Errorf("current profile after use = %q, want %q", string(current), secondProfileName)
	}

	list, err := s.client.Call(ctx, "c0nfig-list", "作成した 2 つのプロファイルが一覧に含まれることを確認する", "config", "list")
	if err != nil {
		return fmt.Errorf("list profiles: %w", err)
	}
	for _, name := range []string{profileName, secondProfileName} {
		if !strings.Contains(string(list), name) {
			return fmt.Errorf("profile list does not contain %q: %q", name, string(list))
		}
	}

	show, err := s.client.Call(ctx, "c0nfig-show", "作成時の属性が show 出力に正しく含まれることを確認する", "config", "show", profileName)
	if err != nil {
		return fmt.Errorf("show profile: %w", err)
	}
	attrs, err := parseProfileAttributes(show)
	if err != nil {
		return fmt.Errorf("decode show output: %w", err)
	}
	if err := assertProfileAttribute(attrs, "ServicePrincipalID", "spid-e2e"); err != nil {
		return fmt.Errorf("after create: %w", err)
	}
	if err := assertProfileAttribute(attrs, "ServicePrincipalKeyID", "kid-e2e"); err != nil {
		return fmt.Errorf("after create: %w", err)
	}
	if err := assertProfileAttribute(attrs, "PrivateKeyPEMPath", "(masked)"); err != nil {
		return fmt.Errorf("after create: %w", err)
	}

	if _, err := s.client.Call(ctx, "c0nfig-edit", "ServicePrincipalID を変更する",
		"config", "edit", profileName,
		"--service-principal-id", "spid-edited",
	); err != nil {
		return fmt.Errorf("edit profile: %w", err)
	}

	showAfterEdit, err := s.client.Call(ctx, "c0nfig-show-after-edit", "編集後の属性が show 出力に反映されていることを確認する", "config", "show", profileName)
	if err != nil {
		return fmt.Errorf("show edited profile: %w", err)
	}
	attrsAfterEdit, err := parseProfileAttributes(showAfterEdit)
	if err != nil {
		return fmt.Errorf("decode edited show output: %w", err)
	}
	if err := assertProfileAttribute(attrsAfterEdit, "ServicePrincipalID", "spid-edited"); err != nil {
		return fmt.Errorf("after edit: %w", err)
	}
	if err := assertProfileAttribute(attrsAfterEdit, "ServicePrincipalKeyID", "kid-e2e"); err != nil {
		return fmt.Errorf("after edit: %w", err)
	}
	if err := assertProfileAttribute(attrsAfterEdit, "PrivateKeyPEMPath", "(masked)"); err != nil {
		return fmt.Errorf("after edit: %w", err)
	}
	return nil
}

func parseProfileAttributes(data []byte) (map[string]any, error) {
	var attrs map[string]any
	if err := json.Unmarshal(data, &attrs); err != nil {
		return nil, err
	}
	return attrs, nil
}

func assertProfileAttribute(attrs map[string]any, key, want string) error {
	got, ok := attrs[key].(string)
	if !ok {
		return fmt.Errorf("attribute %q is not a string", key)
	}
	if got != want {
		return fmt.Errorf("attribute %q = %q, want %q", key, got, want)
	}
	return nil
}
