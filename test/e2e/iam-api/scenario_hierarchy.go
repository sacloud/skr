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
	"strconv"
)

type hierarchyFolder struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	ParentID *int64 `json:"parent_id"`
}

type hierarchyProject struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	ParentFolderID *int64 `json:"parent_folder_id"`
}

// hierarchyScenario moves the child folder into the main folder and the
// project into the child folder. Both moves are verified by reading the
// parent back from the API, and the child folder is deleted by the cleanup
// before the main folder.
func (s *scenario) hierarchyScenario(ctx context.Context) error {
	s.childFolder.attempted = true
	data, err := s.call(ctx, "create-child-folder", "iam-api", "folder", "create",
		"--name", s.childFolder.name, "--description", s.childFolder.description)
	if err != nil {
		return err
	}
	created, err := decodeItem("create-child-folder", data)
	if err != nil {
		return err
	}
	if err := checkIdentity(created, s.childFolder); err != nil {
		return err
	}
	s.childFolder.id = created.id()

	if _, err := s.call(ctx, "move-child-folder", "iam-api", "folder", "move",
		"--ids", s.childFolder.id, "--parent-id", s.folder.id); err != nil {
		return err
	}
	if err := s.checkChildFolderParent(ctx); err != nil {
		return err
	}
	if _, err := s.call(ctx, "move-project", "iam-api", "project", "move",
		"--ids", s.project.id, "--parent-folder-id", s.childFolder.id); err != nil {
		return err
	}
	return s.checkProjectParent(ctx)
}

// checkChildFolderParent reads the child folder and verifies that its parent
// is the main folder of this run.
func (s *scenario) checkChildFolderParent(ctx context.Context) error {
	data, err := s.call(ctx, "read-child-folder", "iam-api", "folder", "read", s.childFolder.id,
		"--query", "{id: .id, name: .name, parent_id: .parent_id}")
	if err != nil {
		return err
	}
	var folder hierarchyFolder
	if err := json.Unmarshal(data, &folder); err != nil {
		return fmt.Errorf("read-child-folder: decode folder: %w", err)
	}
	folderID, err := strconv.ParseInt(s.folder.id, 10, 64)
	if err != nil {
		return fmt.Errorf("read-child-folder: parse folder ID %q: %w", s.folder.id, err)
	}
	if folder.ParentID == nil || *folder.ParentID != folderID {
		return fmt.Errorf("read-child-folder: parent_id = %s, want the folder %s", hierarchyIDString(folder.ParentID), s.folder.id)
	}
	return nil
}

// checkProjectParent reads the project and verifies that its parent folder is
// the child folder of this run.
func (s *scenario) checkProjectParent(ctx context.Context) error {
	data, err := s.call(ctx, "read-project-moved", "iam-api", "project", "read", s.project.id,
		"--query", "{id: .id, name: .name, parent_folder_id: .parent_folder_id}")
	if err != nil {
		return err
	}
	var project hierarchyProject
	if err := json.Unmarshal(data, &project); err != nil {
		return fmt.Errorf("read-project-moved: decode project: %w", err)
	}
	childID, err := strconv.ParseInt(s.childFolder.id, 10, 64)
	if err != nil {
		return fmt.Errorf("read-project-moved: parse folder ID %q: %w", s.childFolder.id, err)
	}
	if project.ParentFolderID == nil || *project.ParentFolderID != childID {
		return fmt.Errorf("read-project-moved: parent_folder_id = %s, want the folder %s", hierarchyIDString(project.ParentFolderID), s.childFolder.id)
	}
	return nil
}

func hierarchyIDString(value *int64) string {
	if value == nil {
		return "null"
	}
	return strconv.FormatInt(*value, 10)
}
