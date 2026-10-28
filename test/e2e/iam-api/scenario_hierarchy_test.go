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

// このファイルは iam-api E2E のフォルダ階層シナリオです。
// child folder と project の移動と、親の確認を確認します。
package main

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
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

// hierarchyScenario は child folder を main folder に移動し、
// project を child folder に移動します。両方の移動は API から親を読み込んで確認し、
// cleanup は main folder の前に child folder を削除します。
func (s *iamRun) hierarchyScenario(t *testing.T) {
	t.Helper()
	s.childFolder.attempted = true
	data := s.call(t, "テスト用の子folderを作成する", "iam-api", "folder", "create",
		"--name", s.childFolder.name, "--description", s.childFolder.description)
	created := requireItem(t, data, "create-child-folder")
	requireIdentity(t, created, s.childFolder)
	s.childFolder.id = created.id()

	s.call(t, "子folderをmain folderに移動する", "iam-api", "folder", "move",
		"--ids", s.childFolder.id, "--parent-id", s.folder.id)
	s.checkChildFolderParent(t)

	s.call(t, "projectを子folderに移動する", "iam-api", "project", "move",
		"--ids", s.project.id, "--parent-folder-id", s.childFolder.id)
	s.checkProjectParent(t)
}

// checkChildFolderParent は child folder を読み込み、
// その親がこの実行の main folder であることを確認します。
func (s *iamRun) checkChildFolderParent(t *testing.T) {
	t.Helper()
	data := s.call(t, "移動後の子folderの親を確認する", "iam-api", "folder", "read", s.childFolder.id,
		"--query", "{id: .id, name: .name, parent_id: .parent_id}")
	var folder hierarchyFolder
	require.NoError(t, json.Unmarshal(data, &folder), "read-child-folder: folderのデコードに失敗しました")
	folderID, err := strconv.ParseInt(s.folder.id, 10, 64)
	require.NoError(t, err, "read-child-folder: folder ID %qを解析できること", s.folder.id)
	require.NotNil(t, folder.ParentID, "read-child-folder: 親folderが設定されていること")
	require.Equal(t, folderID, *folder.ParentID, "read-child-folder: 親がmain folder %sであること", s.folder.id)
}

// checkProjectParent は project を読み込み、
// その親 folder がこの実行の child folder であることを確認します。
func (s *iamRun) checkProjectParent(t *testing.T) {
	t.Helper()
	data := s.call(t, "移動後のprojectの親を確認する", "iam-api", "project", "read", s.project.id,
		"--query", "{id: .id, name: .name, parent_folder_id: .parent_folder_id}")
	var project hierarchyProject
	require.NoError(t, json.Unmarshal(data, &project), "read-project-moved: projectのデコードに失敗しました")
	childID, err := strconv.ParseInt(s.childFolder.id, 10, 64)
	require.NoError(t, err, "read-project-moved: folder ID %qを解析できること", s.childFolder.id)
	require.NotNil(t, project.ParentFolderID, "read-project-moved: 親folderが設定されていること")
	require.Equal(t, childID, *project.ParentFolderID, "read-project-moved: 親folderが子folder %sであること", s.childFolder.id)
}
