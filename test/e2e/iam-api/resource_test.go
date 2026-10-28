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

// このファイルは iam-api E2E の共有ヘルパーとシナリオ本体です。
// リソースの作成・照合・更新・後片付けを helper でビルド済みの skr で実行します。
package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// resourceNamePrefix はシナリオが作成する一時リソース名の接頭辞です。
	resourceNamePrefix = "skr-e2e-iam-"
	// resourceDescription は一時リソースの説明の接頭辞です。実行固有のサフィックスを含めます。
	resourceDescription = "skr-e2e-iam/"
	// userGroupPageSize は user と group の一覧で要求するページサイズです。
	userGroupPageSize = 1000
)

// item は IAM リソースの一覧と読み取りの応答から参照する項目です。
type item struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Email       string `json:"email"`
	ProjectID   int    `json:"project_id"`
}

func (i item) id() string {
	return strconv.Itoa(i.ID)
}

// markedForThisRun はリソースがこの E2E 実行によって作成されたものかどうかを報告します。
// Description には実行固有のサフィックスが含まれて変更されないため、
// 名前が変更されても有効なままです。
func (i item) markedForThisRun(suffix string) bool {
	return i.Description == resourceDescription+suffix
}

// projectedPage は一覧の投影結果の1ページです。
type projectedPage struct {
	Items     []item `json:"items"`
	PageCount int    `json:"page_count"`
	HasNext   *bool  `json:"has_next,omitempty"`
}

// resource はシナリオが作成・照合する IAM リソースです。
// id は作成応答から設定され、pendingName は名前変更後の名前を追跡します。
type resource struct {
	kind        string
	name        string
	code        string
	description string
	projectID   int
	id          string
	attempted   bool
	pendingName string
}

// managedResource はシナリオ自身が削除する一時リソースを追跡します。
// 現在の名前は名前変更ステップに追従し、失敗で残されたリソースを後片付けが識別できるようにします。
type managedResource struct {
	kind        string
	name        string
	currentName string
	id          string
	attempted   bool
}

// iamRun はシナリオ全体の状態と実行対象のリソースを保持します。
type iamRun struct {
	h                *helper.Helper
	password         string
	suffix           string
	user             resource
	group            resource
	folder           resource
	childFolder      resource
	project          resource
	servicePrincipal resource
	scim             managedResource
	ssoProfile       managedResource
	projectAPIKey    managedResource
	email            string
	renamed          string
	principalID      int64
	limitedProjectID int64
}

// callWithErr はビルド済みの skr を実行し、stdout・stderr・エラーを返します。
// 後片付けのように、コマンドの失敗を蓄積して処理を続ける経路で使います。
func (s *iamRun) callWithErr(t *testing.T, description string, args ...string) ([]byte, string, error) {
	t.Helper()
	return s.h.CallAllowingFailure(t, description, args...)
}

// call はビルド済みの skr を実行します。コマンドが失敗した場合はテストを停止します。
func (s *iamRun) call(t *testing.T, description string, args ...string) []byte {
	t.Helper()
	data, _, err := s.callWithErr(t, description, args...)
	require.NoError(t, err, "%s: コマンドが失敗しました", description)
	return data
}

// decodePage は一覧の投影結果をデコードします。
// items は JSON 配列であることを確認し、page_count が負でないことを確認します。
func decodePage(data []byte, label string) (projectedPage, error) {
	var page projectedPage
	if err := json.Unmarshal(data, &page); err != nil {
		return projectedPage{}, fmt.Errorf("%s: decode projected list: %w", label, err)
	}
	if page.Items == nil {
		return projectedPage{}, fmt.Errorf("%s: projected items must be a JSON array", label)
	}
	if page.PageCount < 0 {
		return projectedPage{}, fmt.Errorf("%s: page count is negative", label)
	}
	return page, nil
}

// decodeItem はリソースの応答をデコードします。ID が存在することを確認します。
func decodeItem(data []byte, label string) (item, error) {
	var result item
	if err := json.Unmarshal(data, &result); err != nil {
		return item{}, fmt.Errorf("%s: decode resource: %w", label, err)
	}
	if result.ID == 0 {
		return item{}, fmt.Errorf("%s: resource ID is missing", label)
	}
	return result, nil
}

// paginationDone は一覧の読み込みを打ち切るかどうかを判定します。
// folder、project、service-principal の応答は has_next を含みます。
// user と group の応答には has_next がないため、
// page_count と要求したページサイズから打ち切りを判断します。
func paginationDone(page projectedPage, kind string) bool {
	if page.HasNext != nil {
		return !*page.HasNext
	}
	if page.PageCount == 0 {
		return true
	}
	if kind != "user" && kind != "group" {
		return true
	}
	return page.PageCount < userGroupPageSize
}

func listQuery(kind string, targetNames ...string) string {
	fields := "{id,name,code,description}"
	pageItems := "map(" + fields + ")"
	pageCount := "length"
	hasNext := ""
	if kind == "folder" || kind == "project" || kind == "service-principal" {
		fields = "{id,name,code,description,project_id}"
		pageItems = ".items | map(" + fields + ")"
		pageCount = "(.items | length)"
		hasNext = ", has_next: (.next != null)"
	}
	if len(targetNames) == 1 {
		selection := "map(select(.name == " + strconv.Quote(targetNames[0]) + ") | " + fields + ")"
		if kind == "folder" || kind == "project" || kind == "service-principal" {
			pageItems = ".items | " + selection
		} else {
			pageItems = selection
		}
	}
	return "{items: (" + pageItems + "), page_count: " + pageCount + hasNext + "}"
}

// listWithErr は指定した種別の一覧を全ページ読み込みます。
// targetNames は最大1要素で、名前が一致する項目だけを投影します。
func (s *iamRun) listWithErr(t *testing.T, description, kind string, targetNames ...string) ([]item, error) {
	t.Helper()
	switch kind {
	case "user", "group", "folder", "project", "service-principal":
	default:
		return nil, fmt.Errorf("%s: unknown resource kind %q", description, kind)
	}
	if len(targetNames) > 1 {
		return nil, fmt.Errorf("%s: at most one resource name can be matched", description)
	}
	query := listQuery(kind, targetNames...)
	var all []item
	for pageNumber := 1; ; pageNumber++ {
		args := []string{"iam-api", kind, "list", "--page", strconv.Itoa(pageNumber), "--query", query}
		if kind == "user" || kind == "group" {
			args = append(args, "--per-page", strconv.Itoa(userGroupPageSize))
		}
		data, _, err := s.callWithErr(t, fmt.Sprintf("%s (%d ページ目)", description, pageNumber), args...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", description, err)
		}
		page, err := decodePage(data, description)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", description, err)
		}
		all = append(all, page.Items...)
		if paginationDone(page, kind) {
			return all, nil
		}
	}
}

// list は指定した種別の一覧を全ページ読み込みます。読み込みに失敗した場合はテストを停止します。
func (s *iamRun) list(t *testing.T, description, kind string, targetNames ...string) []item {
	t.Helper()
	items, err := s.listWithErr(t, description, kind, targetNames...)
	require.NoError(t, err, "%s: 一覧の読み込みに失敗しました", description)
	return items
}

// findByNameWithErr は指定したリソースを名前で検索します。
// 見つからない場合は false を返し、複数見つかった場合や
// この E2E 実行で作成されたものでない場合はエラーを返します。
func (s *iamRun) findByNameWithErr(t *testing.T, description string, target resource) (item, bool, error) {
	t.Helper()
	items, err := s.listWithErr(t, description, target.kind, target.name)
	if err != nil {
		return item{}, false, err
	}
	var matches []item
	for _, candidate := range items {
		if candidate.Name == target.name {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return item{}, false, nil
	}
	if len(matches) != 1 || !matches[0].markedForThisRun(s.suffix) {
		return item{}, false, fmt.Errorf("%s: refusing to use %q: resource identity is not unique and marked for this E2E", description, target.name)
	}
	if target.code != "" && matches[0].Code != target.code {
		return item{}, false, fmt.Errorf("%s: refusing to use %q: resource code does not match this E2E", description, target.name)
	}
	if target.projectID != 0 && matches[0].ProjectID != target.projectID {
		return item{}, false, fmt.Errorf("%s: refusing to use %q: project ID does not match this E2E", description, target.name)
	}
	return matches[0], true, nil
}

// checkIdentity はリソースの識別情報が期待値と一致することを確認します。
// allowedNames は名前変更後の移行期間に許容する名前です。
func checkIdentity(candidate item, target resource, allowedNames ...string) error {
	if target.id != "" && candidate.id() != target.id {
		return fmt.Errorf("%s ID mismatch: got %s, want %s", target.kind, candidate.id(), target.id)
	}
	if candidate.Name != target.name {
		allowed := false
		for _, name := range allowedNames {
			if name != "" && candidate.Name == name {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("%s name mismatch: got %q, want %q", target.kind, candidate.Name, target.name)
		}
	}
	if candidate.Description != target.description || !strings.HasPrefix(candidate.Description, resourceDescription) {
		return fmt.Errorf("%s description mismatch for %q", target.kind, target.name)
	}
	if target.code != "" && candidate.Code != target.code {
		return fmt.Errorf("%s code mismatch for %q", target.kind, target.name)
	}
	if target.projectID != 0 && candidate.ProjectID != target.projectID {
		return fmt.Errorf("%s project ID mismatch for %q", target.kind, target.name)
	}
	return nil
}

// requireItem はリソースの応答をデコードします。デコードに失敗した場合はテストを停止します。
func requireItem(t *testing.T, data []byte, label string) item {
	t.Helper()
	created, err := decodeItem(data, label)
	require.NoError(t, err, "%s: 応答のデコードに失敗しました", label)
	return created
}

// requireIdentity はリソースの識別情報が期待値と一致することを確認します。
// 一致しない場合はテストを停止します。
func requireIdentity(t *testing.T, candidate item, target resource, allowedNames ...string) {
	t.Helper()
	require.NoError(t, checkIdentity(candidate, target, allowedNames...), "%s: 識別情報が一致しません", target.kind)
}

// preflight は作成前に同名リソースが存在しないことを確認します。
// 同名リソースが存在する場合はテストを失敗させ、既存リソースを変更しません。
func (s *iamRun) preflight(t *testing.T) {
	t.Helper()
	for _, target := range []resource{s.user, s.group, s.folder, s.childFolder, s.project, s.servicePrincipal} {
		items := s.list(t, "作成前に同名の"+target.kind+"が存在しないことを確認する", target.kind, target.name)
		for _, candidate := range items {
			require.NotEqual(t, target.name, candidate.Name,
				"%s name %q already exists; refusing to modify it", target.kind, target.name)
		}
	}
}

// readWithErr はリソースを読み込みます。後片付けのように失敗を蓄積する経路で使います。
func (s *iamRun) readWithErr(t *testing.T, description string, target resource) (item, error) {
	t.Helper()
	data, _, err := s.callWithErr(t, description, "iam-api", target.kind, "read", target.id)
	if err != nil {
		return item{}, fmt.Errorf("%s: %w", description, err)
	}
	result, err := decodeItem(data, description)
	if err != nil {
		return item{}, fmt.Errorf("%s: %w", description, err)
	}
	return result, nil
}

// read はリソースを読み込みます。読み込みに失敗した場合はテストを停止します。
func (s *iamRun) read(t *testing.T, description string, target resource) item {
	t.Helper()
	result, err := s.readWithErr(t, description, target)
	require.NoError(t, err, "%s: リソースの読み込みに失敗しました", description)
	return result
}

func (s *iamRun) createFolder(t *testing.T) {
	t.Helper()
	s.folder.attempted = true
	data := s.call(t, "テスト用のfolderを作成する",
		"iam-api", "folder", "create", "--name", s.folder.name, "--description", s.folder.description)
	created := requireItem(t, data, "create-folder")
	requireIdentity(t, created, s.folder)
	s.folder.id = created.id()
}

func (s *iamRun) createProject(t *testing.T) {
	t.Helper()
	s.project.attempted = true
	data := s.call(t, "テスト用のprojectを作成する",
		"iam-api", "project", "create",
		"--code", s.project.code, "--name", s.project.name, "--description", s.project.description,
		"--parent-folder-id", s.folder.id)
	created := requireItem(t, data, "create-project")
	requireIdentity(t, created, s.project)
	s.project.id = created.id()
	s.servicePrincipal.projectID = created.ID
}

func (s *iamRun) createUser(t *testing.T) {
	t.Helper()
	s.user.attempted = true
	// パスワードはコマンド引数に含めず、テスト専用ディレクトリの保護したファイルから読み込ませます。
	passwordFile := filepath.Join(t.TempDir(), "password")
	require.NoError(t, os.WriteFile(passwordFile, []byte(s.password), 0o600), "パスワードファイルの書き込みに失敗しました")
	data := s.call(t, "テスト用のuserを作成する",
		"iam-api", "user", "create",
		"--name", s.user.name, "--code", s.user.code, "--description", s.user.description,
		"--password-file", passwordFile)
	created := requireItem(t, data, "create-user")
	requireIdentity(t, created, s.user)
	s.user.id = created.id()
}

func (s *iamRun) createServicePrincipal(t *testing.T) {
	t.Helper()
	s.servicePrincipal.attempted = true
	data := s.call(t, "テスト用のservice principalを作成する",
		"iam-api", "service-principal", "create",
		"--project-id", s.project.id, "--name", s.servicePrincipal.name,
		"--description", s.servicePrincipal.description)
	created := requireItem(t, data, "create-service-principal")
	requireIdentity(t, created, s.servicePrincipal)
	require.Equal(t, s.project.id, strconv.Itoa(created.ProjectID),
		"作成したservice principalがテスト用のprojectに属すること")
	s.servicePrincipal.id = created.id()
}

func (s *iamRun) verifyUser(t *testing.T) {
	t.Helper()
	items := s.list(t, "作成したuserが一覧に含まれることを確認する", "user", s.user.name)
	found := false
	for _, candidate := range items {
		if candidate.id() == s.user.id {
			found = true
			break
		}
	}
	require.True(t, found, "作成したuser %sが一覧に存在すること", s.user.id)
}

func (s *iamRun) updateUser(t *testing.T) {
	t.Helper()
	s.user.pendingName = s.renamed
	data := s.call(t, "userの名前を更新する",
		"iam-api", "user", "update", s.user.id, "--name", s.renamed, "--description", s.user.description)
	updated := requireItem(t, data, "update-user")
	require.Equal(t, s.renamed, updated.Name, "更新後のuserの名前がリクエストと一致すること")
	expected := s.user
	expected.name = s.renamed
	requireIdentity(t, updated, expected)
	// 名前変更された user は code と description を保持するため、実行の identity は有効なままです。
	s.user.name = s.renamed
	s.user.pendingName = ""
}

func (s *iamRun) updateFolder(t *testing.T) {
	t.Helper()
	renamed := s.folder.name + "-renamed"
	s.folder.pendingName = renamed
	data := s.call(t, "folderの名前を更新する",
		"iam-api", "folder", "update", s.folder.id, "--name", renamed, "--description", s.folder.description)
	updated := requireItem(t, data, "update-folder")
	require.Equal(t, renamed, updated.Name, "更新後のfolderの名前がリクエストと一致すること")
	expected := s.folder
	expected.name = renamed
	requireIdentity(t, updated, expected)
	s.folder.name = renamed
	s.folder.pendingName = ""
}

func (s *iamRun) updateProject(t *testing.T) {
	t.Helper()
	renamed := s.project.name + "-renamed"
	s.project.pendingName = renamed
	data := s.call(t, "projectの名前を更新する",
		"iam-api", "project", "update", s.project.id, "--name", renamed, "--description", s.project.description)
	updated := requireItem(t, data, "update-project")
	require.Equal(t, renamed, updated.Name, "更新後のprojectの名前がリクエストと一致すること")
	expected := s.project
	expected.name = renamed
	requireIdentity(t, updated, expected)
	s.project.name = renamed
	s.project.pendingName = ""
}

func (s *iamRun) registerEmail(t *testing.T) {
	t.Helper()
	s.call(t, "userにメールアドレスを一時的に登録する", "iam-api", "user", "register-email", s.user.id, "--email", s.email)
	read := s.read(t, "登録したメールアドレスを確認する", s.user)
	requireIdentity(t, read, s.user)
	require.Equal(t, s.email, read.Email, "登録したメールアドレスがリクエストと一致すること")
}

func (s *iamRun) unregisterEmail(t *testing.T) {
	t.Helper()
	s.call(t, "userのメールアドレスを解除する", "iam-api", "user", "unregister-email", s.user.id)
	read := s.read(t, "メールアドレスが解除されたことを確認する", s.user)
	requireIdentity(t, read, s.user)
	require.Empty(t, read.Email, "メールアドレスが解除されていること")
}

func (s *iamRun) createGroup(t *testing.T) {
	t.Helper()
	s.group.attempted = true
	data := s.call(t, "テスト用のgroupを作成する",
		"iam-api", "group", "create", "--name", s.group.name, "--description", s.group.description)
	created := requireItem(t, data, "create-group")
	requireIdentity(t, created, s.group)
	s.group.id = created.id()
}

// updateGroup は group の名前と説明を更新し、元の名前に戻します。
// group update は指定しない項目を空で上書きするため、
// 説明も毎回指定して元の値を保持します。
func (s *iamRun) updateGroup(t *testing.T) {
	t.Helper()
	s.updateGroupName(t, "groupの名前と説明を更新する", s.group.name+"-renamed")
	s.updateGroupName(t, "groupの名前と説明を元に戻す", s.group.name)
}

func (s *iamRun) updateGroupName(t *testing.T, description, name string) {
	t.Helper()
	data := s.call(t, description, "iam-api", "group", "update", s.group.id,
		"--name", name, "--description", s.group.description)
	updated := requireItem(t, data, "update-group")
	require.Equal(t, name, updated.Name, "更新後のgroupの名前がリクエストと一致すること")
	expected := s.group
	expected.name = name
	requireIdentity(t, updated, expected)
}

func (s *iamRun) updateMemberships(t *testing.T) {
	t.Helper()
	data := s.call(t, "groupの所属をuserだけに更新する",
		"iam-api", "group", "update-memberships", s.group.id, "--request", "["+s.user.id+"]")
	requireMemberships(t, data, "update-memberships", s.user.id)
}

func (s *iamRun) readMemberships(t *testing.T) {
	t.Helper()
	data := s.call(t, "groupの所属を確認する", "iam-api", "group", "read-memberships", s.group.id)
	requireMemberships(t, data, "read-memberships", s.user.id)
}

// requireMemberships は group の所属が指定した user だけであることを確認します。
func requireMemberships(t *testing.T, data []byte, label, userID string) {
	t.Helper()
	var memberships []struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.Unmarshal(data, &memberships), "%s: 所属のデコードに失敗しました", label)
	require.Len(t, memberships, 1, "%s: 所属がテスト用のuser %sだけであること", label, userID)
	require.Equal(t, userID, strconv.Itoa(memberships[0].ID), "%s: 所属がテスト用のuser %sであること", label, userID)
}

// cleanupResource は、後片付けとしてリソースの識別情報を再確認してから削除します。
// 削除後は名前が残っていないことを確認します。失敗は呼び出し元に返されます。
func (s *iamRun) cleanupResource(t *testing.T, target *resource) error {
	t.Helper()
	if !target.attempted {
		return nil
	}
	id := target.id
	if id == "" {
		found, ok, err := s.findByNameWithErr(t, "後片付けとして作成を試みた"+target.kind+"を検索する", *target)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		id = found.id()
	}
	verified := *target
	verified.id = id
	current, err := s.readWithErr(t, "後片付けの前に"+target.kind+"の識別情報を確認する", verified)
	if err != nil {
		return fmt.Errorf("cannot verify %s before cleanup: %w", target.kind, err)
	}
	if err := checkIdentity(current, verified, target.pendingName); err != nil {
		return fmt.Errorf("cleanup refused: %w", err)
	}
	if _, _, err := s.callWithErr(t, "後片付けとして"+target.kind+"を削除する", "iam-api", target.kind, "delete", id); err != nil {
		return fmt.Errorf("delete E2E %s: %w", target.kind, err)
	}
	names := []string{target.name}
	if target.pendingName != "" && target.pendingName != target.name {
		names = append(names, target.pendingName)
	}
	for _, name := range names {
		verifyTarget := *target
		verifyTarget.name = name
		found, ok, err := s.findByNameWithErr(t, "後片付けの後に"+target.kind+"が残っていないことを確認する", verifyTarget)
		if err != nil {
			return fmt.Errorf("verify E2E %s deletion: %w", target.kind, err)
		}
		if ok {
			return fmt.Errorf("E2E %s %s remains after delete", target.kind, found.id())
		}
	}
	return nil
}

// cleanup はシナリオが作成した一時リソースを削除します。
// managed resource は project や folder 自体より先に削除されます。
// 途中の失敗でも残りの後片付けを試み、その失敗を結果に含めます。
func (s *iamRun) cleanup(t *testing.T) error {
	t.Helper()
	var result error
	for _, target := range []*managedResource{&s.scim, &s.ssoProfile, &s.projectAPIKey} {
		result = errors.Join(result, s.cleanupManaged(t, target))
	}
	for _, target := range []*resource{&s.servicePrincipal, &s.group, &s.user, &s.project, &s.childFolder, &s.folder} {
		result = errors.Join(result, s.cleanupResource(t, target))
	}
	return result
}

// run はシナリオ全体を実行します。作成した一時リソースは失敗時にも後片付けで削除します。
func (s *iamRun) run(t *testing.T) {
	t.Helper()
	s.preflight(t)
	defer func() {
		require.NoError(t, s.cleanup(t), "後片付けに失敗しました")
	}()
	s.readOnlyScenario(t)
	s.createFolder(t)
	s.createProject(t)
	s.readResourcePolicies(t)
	s.policyScenario(t)
	s.hierarchyScenario(t)
	s.createUser(t)
	s.createServicePrincipal(t)
	s.servicePrincipalScenario(t)
	s.projectAPIKeyScenario(t)
	s.scimScenario(t)
	s.ssoScenario(t)
	s.verifyUser(t)
	s.read(t, "作成したuserを読み込む", s.user)
	s.updateUser(t)
	s.registerEmail(t)
	s.unregisterEmail(t)
	s.updateFolder(t)
	s.updateProject(t)
	s.createGroup(t)
	s.updateMemberships(t)
	s.readMemberships(t)
	s.updateGroup(t)
	s.user2faScenario(t)
	s.accountScenario(t)
}

const passwordChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-."

// randomPassword は IAM のパスワード要件（文字と数字）を満たす24文字のパスワードを生成します。
func randomPassword(t *testing.T) string {
	t.Helper()
	// パスワード要件（文字と数字）を満たすため、先頭が文字と数字である24文字とします。
	const prefix = "Sk9"
	value := make([]byte, 21)
	for index := range value {
		position, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordChars))))
		require.NoError(t, err, "パスワードの生成に失敗しました")
		value[index] = passwordChars[position.Int64()]
	}
	return prefix + string(value)
}
