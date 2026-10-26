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
	"os"
	"path/filepath"
	"strconv"
)

const projectAPIKeyRole = "resource-creator"

// projectAPIKeyScenario creates an API key in the project of this run, reads,
// lists, updates, and deletes it. Create and read responses contain the
// access token, so every output is projected with --query and the evidence
// never stores the token itself. The key is deleted inside the scenario; the
// cleanup only removes it when a previous step failed.
func (s *scenario) projectAPIKeyScenario(ctx context.Context) error {
	s.projectAPIKey.attempted = true
	s.projectAPIKey.currentName = s.projectAPIKey.name
	createFile, err := s.writeProjectAPIKeyRequest("pak-create", s.projectAPIKey.name, true)
	if err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(createFile); err != nil && !os.IsNotExist(err) {
			fmt.Printf("remove private %s request file: %v\n", "project API key", err)
		}
	}()
	data, err := s.call(ctx, "create-project-api-key", "iam-api", "project-api-key", "create",
		"--request", "@"+createFile, "--query", "{id: .id, name: .name, iam_roles: .iam_roles}")
	if err != nil {
		return err
	}
	id, name, roles, err := decodeAPIKeyProjection("create-project-api-key", data)
	if err != nil {
		return err
	}
	if name != s.projectAPIKey.name {
		return fmt.Errorf("create-project-api-key: name = %q, want %q", name, s.projectAPIKey.name)
	}
	if err := checkAPIKeyRoles(roles); err != nil {
		return err
	}
	s.projectAPIKey.id = id

	data, err = s.call(ctx, "read-project-api-key", "iam-api", "project-api-key", "read", s.projectAPIKey.id,
		"--query", "{id: .id, name: .name, iam_roles: .iam_roles}")
	if err != nil {
		return err
	}
	if id, name, roles, err = decodeAPIKeyProjection("read-project-api-key", data); err != nil {
		return err
	}
	if id != s.projectAPIKey.id || name != s.projectAPIKey.currentName {
		return fmt.Errorf("read-project-api-key: got %q/%q, want %q/%q", id, name, s.projectAPIKey.id, s.projectAPIKey.currentName)
	}
	if err := checkAPIKeyRoles(roles); err != nil {
		return err
	}

	data, err = s.call(ctx, "list-project-api-keys", "iam-api", "project-api-key", "list", "--query", managedListQuery)
	if err != nil {
		return err
	}
	found, err := checkManagedListed("list-project-api-keys", data, s.projectAPIKey.id, s.projectAPIKey.currentName)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("list-project-api-keys: key %s is missing from the list", s.projectAPIKey.id)
	}

	renamed := s.projectAPIKey.name + "-renamed"
	updateFile, err := s.writeProjectAPIKeyRequest("pak-update", renamed, false)
	if err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(updateFile); err != nil && !os.IsNotExist(err) {
			fmt.Printf("remove private %s request file: %v\n", "project API key", err)
		}
	}()
	data, err = s.call(ctx, "update-project-api-key", "iam-api", "project-api-key", "update", s.projectAPIKey.id,
		"--request", "@"+updateFile, "--query", "{id: .id, name: .name}")
	if err != nil {
		return err
	}
	if _, name, _, err = decodeAPIKeyProjection("update-project-api-key", data); err != nil {
		return err
	}
	if name != renamed {
		return fmt.Errorf("update-project-api-key: name = %q, want %q", name, renamed)
	}
	s.projectAPIKey.currentName = renamed

	if _, err := s.call(ctx, "delete-project-api-key", "iam-api", "project-api-key", "delete", s.projectAPIKey.id); err != nil {
		return err
	}
	s.projectAPIKey.id = ""
	return nil
}

// writeProjectAPIKeyRequest writes the create or update request JSON. The
// create request pins the key to the project of this run, or to the project
// of the authentication context when the principal is limited to a specific
// project.
func (s *scenario) writeProjectAPIKeyRequest(purpose, name string, create bool) (string, error) {
	request := map[string]any{
		"name":        name,
		"description": s.projectAPIKey.name,
		"iam_roles":   []string{projectAPIKeyRole},
	}
	if create {
		pinned := s.projectAPIKeyProjectID()
		projectID, err := strconv.ParseInt(pinned, 10, 64)
		if err != nil {
			return "", fmt.Errorf("parse project ID %q: %w", pinned, err)
		}
		request["project_id"] = projectID
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode project API key %s request: %w", purpose, err)
	}
	path := filepath.Join(os.TempDir(), "skr-e2e-iam-"+purpose+"-"+s.suffix+".json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return "", fmt.Errorf("write project API key %s request file: %w", purpose, err)
	}
	return path, nil
}

// projectAPIKeyProjectID returns the project to pin a new API key to. A
// principal limited to a specific project can only create keys there, so the
// authentication context's project wins over the project of this run.
func (s *scenario) projectAPIKeyProjectID() string {
	if s.limitedProjectID > 0 {
		return strconv.FormatInt(s.limitedProjectID, 10)
	}
	return s.project.id
}

// decodeAPIKeyProjection decodes the projected {id, name, iam_roles} output.
func decodeAPIKeyProjection(step string, data []byte) (id string, name string, roles []string, err error) {
	var result struct {
		ID       any      `json:"id"`
		Name     string   `json:"name"`
		IAMRoles []string `json:"iam_roles"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", "", nil, fmt.Errorf("%s: decode key: %w", step, err)
	}
	switch value := result.ID.(type) {
	case string:
		id = value
	case float64:
		id = strconv.FormatInt(int64(value), 10)
	default:
		return "", "", nil, fmt.Errorf("%s: key ID is missing", step)
	}
	if id == "" {
		return "", "", nil, fmt.Errorf("%s: key ID is missing", step)
	}
	return id, result.Name, result.IAMRoles, nil
}

func checkAPIKeyRoles(roles []string) error {
	if len(roles) != 1 || roles[0] != projectAPIKeyRole {
		return fmt.Errorf("iam_roles = %q, want [%q]", roles, projectAPIKeyRole)
	}
	return nil
}
