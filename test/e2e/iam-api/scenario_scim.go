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
)

type scimConfiguration struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
}

type secretTokenLength struct {
	TokenLength int `json:"token_length"`
}

// scimScenario は user provisioning configuration を作成、読み込み、更新、一覧表示、
// 秘密トークンの再生成、削除を行います。
// configuration は organization 全体のものであるため、シナリオ内で削除されます。
// cleanup は前のステップが失敗した場合のみ削除します。
func (s *scenario) scimScenario(ctx context.Context) error {
	s.scim.attempted = true
	s.scim.currentName = s.scim.name
	data, err := s.call(ctx, "create-scim", "iam-api", "scim", "create",
		"--name", s.scim.name, "--query", "{id: .id, name: .name}")
	if err != nil {
		return err
	}
	id, name, err := decodeNamedResource("create-scim", data)
	if err != nil {
		return err
	}
	if name != s.scim.name {
		return fmt.Errorf("create-scim: name = %q, want %q", name, s.scim.name)
	}
	s.scim.id = id

	data, err = s.call(ctx, "read-scim", "iam-api", "scim", "read", s.scim.id,
		"--query", "{id: .id, name: .name, base_url: .base_url}")
	if err != nil {
		return err
	}
	read, err := decodeScimConfiguration("read-scim", data)
	if err != nil {
		return err
	}
	if read.ID != s.scim.id || read.Name != s.scim.currentName {
		return fmt.Errorf("read-scim: got %q/%q, want %q/%q", read.ID, read.Name, s.scim.id, s.scim.currentName)
	}
	if read.BaseURL == "" {
		return fmt.Errorf("read-scim: base_url is empty")
	}

	renamed := s.scim.name + "-renamed"
	data, err = s.call(ctx, "update-scim", "iam-api", "scim", "update", s.scim.id,
		"--name", renamed, "--query", "{id: .id, name: .name}")
	if err != nil {
		return err
	}
	if _, name, err = decodeNamedResource("update-scim", data); err != nil {
		return err
	}
	if name != renamed {
		return fmt.Errorf("update-scim: name = %q, want %q", name, renamed)
	}
	s.scim.currentName = renamed

	data, err = s.call(ctx, "list-scim", "iam-api", "scim", "list", "--query", managedListQuery)
	if err != nil {
		return err
	}
	found, err := checkManagedListed("list-scim", data, s.scim.id, s.scim.currentName)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("list-scim: configuration %s is missing from the list", s.scim.id)
	}

	// 応答には秘密トークンが含まれるため、
	// query はその長さのみを投影し、evidence にはトークン自体が保存されない。
	data, err = s.call(ctx, "regenerate-scim-token", "iam-api", "scim", "regenerate-token", s.scim.id,
		"--query", "{token_length: (.secret_token | length)}")
	if err != nil {
		return err
	}
	var token secretTokenLength
	if err := json.Unmarshal(data, &token); err != nil {
		return fmt.Errorf("regenerate-scim-token: decode token length: %w", err)
	}
	if token.TokenLength <= 0 {
		return fmt.Errorf("regenerate-scim-token: token_length = %d, want a positive value", token.TokenLength)
	}

	if _, err := s.call(ctx, "delete-scim", "iam-api", "scim", "delete", s.scim.id); err != nil {
		return err
	}
	s.scim.id = ""
	return nil
}

func decodeScimConfiguration(step string, data []byte) (scimConfiguration, error) {
	var configuration scimConfiguration
	if err := json.Unmarshal(data, &configuration); err != nil {
		return scimConfiguration{}, fmt.Errorf("%s: decode configuration: %w", step, err)
	}
	if configuration.ID == "" {
		return scimConfiguration{}, fmt.Errorf("%s: configuration ID is missing", step)
	}
	return configuration, nil
}
