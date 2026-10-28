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

// このファイルは iam-api E2E の共有ヘルパーと、user・group・folder・project の
// 作成・更新シナリオです。リソースの作成・照合・更新・後片付けを
// helper でビルド済みの skr で実行します。
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

// iamItem は IAM リソースの一覧と読み取りの応答から参照する項目です。
type iamItem struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Email       string `json:"email"`
	ProjectID   int    `json:"project_id"`
}

func (i iamItem) id() string {
	return strconv.Itoa(i.ID)
}

// projectedPage は一覧の投影結果の1ページです。
type projectedPage struct {
	Items     []iamItem `json:"items"`
	PageCount int       `json:"page_count"`
	HasNext   *bool     `json:"has_next,omitempty"`
}

// iamResource はシナリオが作成・追跡する一時リソースの識別情報です。
// user、group、folder、project、service principal と、organization 全体の
// SCIM 設定、SSO profile、project API key を同じ型で追跡します。
type iamResource struct {
	// kind は CLI のリソース名です。
	kind string
	// name はリソースの現在の名前です。名前変更ステップで更新されます。
	name string
	// code は user と project のコードです。
	code string
	// description はリソースの説明です。
	description string
	// projectID は service principal が属する project の ID です。
	projectID int
	// id は作成応答から保存したリソースの ID です。
	id string
	// attempted は作成 API を呼び出したかどうかです。呼び出しが失敗しても後片付けの対象にします。
	attempted bool
	// deleted はシナリオ内で削除済みかどうかです。削除済みのリソースは後片付けで変更しません。
	deleted bool
	// renamedTo は名前変更ステップの実行中にリソースが改名する可能性のある名前です。
	// ステップが失敗した場合、後片付けはこの名前も残っていないことを確認します。
	renamedTo string
}

// iamRun は1回の IAM API E2E 実行の状態と設定です。
type iamRun struct {
	// password は user の作成に使う一時的なパスワードです。
	password string

	// email は user に一時的に登録するメールアドレスです。
	email string

	// user はテスト用の user です。
	user iamResource

	// group はテスト用の group です。
	group iamResource

	// folder はテスト用の folder です。project の親になります。
	folder iamResource

	// childFolder は階層移動の確認に使う子 folder です。
	childFolder iamResource

	// project はテスト用の project です。service principal と project API key の所属先になります。
	project iamResource

	// servicePrincipal はテスト用の service principal です。
	servicePrincipal iamResource

	// scim は organization 全体の SCIM 設定です。シナリオ内で削除します。
	scim iamResource

	// ssoProfile は organization 全体の SSO profile です。シナリオ内で削除します。
	ssoProfile iamResource

	// projectAPIKey はテスト用の project 内で作成する API key です。シナリオ内で削除します。
	projectAPIKey iamResource

	// principalID は認証コンテキストから読み込んだ principal の ID です。
	principalID int64

	// limitedProjectID は principal が特定の project に限定されている場合の project ID です。
	limitedProjectID int64
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
func decodeItem(data []byte, label string) (iamItem, error) {
	var result iamItem
	if err := json.Unmarshal(data, &result); err != nil {
		return iamItem{}, fmt.Errorf("%s: decode resource: %w", label, err)
	}
	if result.ID == 0 {
		return iamItem{}, fmt.Errorf("%s: resource ID is missing", label)
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

// lookupIAMItems は指定した種別の一覧を全ページ読み込みます。
// targetNames は最大1要素で、名前が一致する項目だけを投影します。
func lookupIAMItems(t *testing.T, h *helper.Helper, description, kind string, targetNames ...string) ([]iamItem, error) {
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
	var all []iamItem
	for pageNumber := 1; ; pageNumber++ {
		args := []string{"iam-api", kind, "list", "--page", strconv.Itoa(pageNumber), "--query", query}
		if kind == "user" || kind == "group" {
			args = append(args, "--per-page", strconv.Itoa(userGroupPageSize))
		}
		data, _, err := h.CallAllowingFailure(t, fmt.Sprintf("%s (%d ページ目)", description, pageNumber), args...)
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

// listIAMItems は指定した種別の一覧を全ページ読み込みます。読み込みに失敗した場合はテストを停止します。
func listIAMItems(t *testing.T, h *helper.Helper, description, kind string, targetNames ...string) []iamItem {
	t.Helper()
	items, err := lookupIAMItems(t, h, description, kind, targetNames...)
	require.NoError(t, err, "%s: 一覧の読み込みに失敗しました", description)
	return items
}

// findIAMItemByName は指定したリソースを名前で検索します。
// 見つからない場合は false を返し、複数見つかった場合や
// この E2E 実行で作成されたものでない場合はエラーを返します。
func findIAMItemByName(t *testing.T, h *helper.Helper, description string, target iamResource) (iamItem, bool, error) {
	t.Helper()
	items, err := lookupIAMItems(t, h, description, target.kind, target.name)
	if err != nil {
		return iamItem{}, false, err
	}
	var matches []iamItem
	for _, candidate := range items {
		if candidate.Name == target.name {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return iamItem{}, false, nil
	}
	if len(matches) != 1 || matches[0].Description != target.description {
		return iamItem{}, false, fmt.Errorf("%s: refusing to use %q: resource identity is not unique and marked for this E2E", description, target.name)
	}
	if target.code != "" && matches[0].Code != target.code {
		return iamItem{}, false, fmt.Errorf("%s: refusing to use %q: resource code does not match this E2E", description, target.name)
	}
	if target.projectID != 0 && matches[0].ProjectID != target.projectID {
		return iamItem{}, false, fmt.Errorf("%s: refusing to use %q: project ID does not match this E2E", description, target.name)
	}
	return matches[0], true, nil
}

// checkIdentity はリソースの識別情報が期待値と一致することを確認します。
// 名前は名前変更ステップの実行中に改名される可能性のある renamedTo も許容します。
func checkIdentity(candidate iamItem, target iamResource) error {
	if target.id != "" && candidate.id() != target.id {
		return fmt.Errorf("%s ID mismatch: got %s, want %s", target.kind, candidate.id(), target.id)
	}
	if candidate.Name != target.name && candidate.Name != target.renamedTo {
		return fmt.Errorf("%s name mismatch: got %q, want %q", target.kind, candidate.Name, target.name)
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
func requireItem(t *testing.T, data []byte, label string) iamItem {
	t.Helper()
	created, err := decodeItem(data, label)
	require.NoError(t, err, "%s: 応答のデコードに失敗しました", label)
	return created
}

// requireIdentity はリソースの識別情報が期待値と一致することを確認します。
// 一致しない場合はテストを停止します。
func requireIdentity(t *testing.T, candidate iamItem, target iamResource) {
	t.Helper()
	require.NoError(t, checkIdentity(candidate, target), "%s: 識別情報が一致しません", target.kind)
}

// requireNoNameCollision は作成前に同名リソースが存在しないことを確認します。
// 同名リソースが存在する場合はテストを失敗させ、既存リソースを変更しません。
func requireNoNameCollision(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	for _, target := range []iamResource{run.user, run.group, run.folder, run.childFolder, run.project, run.servicePrincipal} {
		items := listIAMItems(t, h, "作成前に同名の"+target.kind+"が存在しないことを確認する", target.kind, target.name)
		for _, candidate := range items {
			require.NotEqual(t, target.name, candidate.Name,
				"%s name %q already exists; refusing to modify it", target.kind, target.name)
		}
	}
}

// lookupIAMItem はリソースを読み込みます。後片付けのように失敗を蓄積する経路で使います。
func lookupIAMItem(t *testing.T, h *helper.Helper, description string, target iamResource) (iamItem, error) {
	t.Helper()
	data, _, err := h.CallAllowingFailure(t, description, "iam-api", target.kind, "read", target.id)
	if err != nil {
		return iamItem{}, fmt.Errorf("%s: %w", description, err)
	}
	result, err := decodeItem(data, description)
	if err != nil {
		return iamItem{}, fmt.Errorf("%s: %w", description, err)
	}
	return result, nil
}

// readIAMItem はリソースを読み込みます。読み込みに失敗した場合はテストを停止します。
func readIAMItem(t *testing.T, h *helper.Helper, description string, target iamResource) iamItem {
	t.Helper()
	result, err := lookupIAMItem(t, h, description, target)
	require.NoError(t, err, "%s: リソースの読み込みに失敗しました", description)
	return result
}

func createFolder(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	run.folder.attempted = true
	data := h.Call(t, "テスト用のfolderを作成する",
		"iam-api", "folder", "create", "--name", run.folder.name, "--description", run.folder.description)
	created := requireItem(t, data, "create-folder")
	requireIdentity(t, created, run.folder)
	run.folder.id = created.id()
}

func createProject(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	run.project.attempted = true
	data := h.Call(t, "テスト用のprojectを作成する",
		"iam-api", "project", "create",
		"--code", run.project.code, "--name", run.project.name, "--description", run.project.description,
		"--parent-folder-id", run.folder.id)
	created := requireItem(t, data, "create-project")
	requireIdentity(t, created, run.project)
	run.project.id = created.id()
	run.servicePrincipal.projectID = created.ID
}

func createUser(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	run.user.attempted = true
	// パスワードはコマンド引数に含めず、テスト専用ディレクトリの保護したファイルから読み込ませます。
	passwordFile := filepath.Join(t.TempDir(), "password")
	require.NoError(t, os.WriteFile(passwordFile, []byte(run.password), 0o600), "パスワードファイルの書き込みに失敗しました")
	data := h.Call(t, "テスト用のuserを作成する",
		"iam-api", "user", "create",
		"--name", run.user.name, "--code", run.user.code, "--description", run.user.description,
		"--password-file", passwordFile)
	created := requireItem(t, data, "create-user")
	requireIdentity(t, created, run.user)
	run.user.id = created.id()
}

func createServicePrincipal(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	run.servicePrincipal.attempted = true
	data := h.Call(t, "テスト用のservice principalを作成する",
		"iam-api", "service-principal", "create",
		"--project-id", run.project.id, "--name", run.servicePrincipal.name,
		"--description", run.servicePrincipal.description)
	created := requireItem(t, data, "create-service-principal")
	requireIdentity(t, created, run.servicePrincipal)
	require.Equal(t, run.project.id, strconv.Itoa(created.ProjectID),
		"作成したservice principalがテスト用のprojectに属すること")
	run.servicePrincipal.id = created.id()
}

func verifyUser(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	items := listIAMItems(t, h, "作成したuserが一覧に含まれることを確認する", "user", run.user.name)
	found := false
	for _, candidate := range items {
		if candidate.id() == run.user.id {
			found = true
			break
		}
	}
	require.True(t, found, "作成したuser %sが一覧に存在すること", run.user.id)
}

func updateUser(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	renamed := run.user.name + "-renamed"
	run.user.renamedTo = renamed
	data := h.Call(t, "userの名前を更新する",
		"iam-api", "user", "update", run.user.id, "--name", renamed, "--description", run.user.description)
	updated := requireItem(t, data, "update-user")
	require.Equal(t, renamed, updated.Name, "更新後のuserの名前がリクエストと一致すること")
	requireIdentity(t, updated, run.user)
	// 名前変更された user は code と description を保持するため、実行の identity は有効なままです。
	run.user.name = renamed
	run.user.renamedTo = ""
}

func updateFolder(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	renamed := run.folder.name + "-renamed"
	run.folder.renamedTo = renamed
	data := h.Call(t, "folderの名前を更新する",
		"iam-api", "folder", "update", run.folder.id, "--name", renamed, "--description", run.folder.description)
	updated := requireItem(t, data, "update-folder")
	require.Equal(t, renamed, updated.Name, "更新後のfolderの名前がリクエストと一致すること")
	requireIdentity(t, updated, run.folder)
	run.folder.name = renamed
	run.folder.renamedTo = ""
}

func updateProject(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	renamed := run.project.name + "-renamed"
	run.project.renamedTo = renamed
	data := h.Call(t, "projectの名前を更新する",
		"iam-api", "project", "update", run.project.id, "--name", renamed, "--description", run.project.description)
	updated := requireItem(t, data, "update-project")
	require.Equal(t, renamed, updated.Name, "更新後のprojectの名前がリクエストと一致すること")
	requireIdentity(t, updated, run.project)
	run.project.name = renamed
	run.project.renamedTo = ""
}

func registerEmail(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	h.Call(t, "userにメールアドレスを一時的に登録する", "iam-api", "user", "register-email", run.user.id, "--email", run.email)
	read := readIAMItem(t, h, "登録したメールアドレスを確認する", run.user)
	requireIdentity(t, read, run.user)
	require.Equal(t, run.email, read.Email, "登録したメールアドレスがリクエストと一致すること")
}

func unregisterEmail(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	h.Call(t, "userのメールアドレスを解除する", "iam-api", "user", "unregister-email", run.user.id)
	read := readIAMItem(t, h, "メールアドレスが解除されたことを確認する", run.user)
	requireIdentity(t, read, run.user)
	require.Empty(t, read.Email, "メールアドレスが解除されていること")
}

func createGroup(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	run.group.attempted = true
	data := h.Call(t, "テスト用のgroupを作成する",
		"iam-api", "group", "create", "--name", run.group.name, "--description", run.group.description)
	created := requireItem(t, data, "create-group")
	requireIdentity(t, created, run.group)
	run.group.id = created.id()
}

// updateGroup は group の名前と説明を更新し、元の名前に戻します。
// group update は指定しない項目を空で上書きするため、
// 説明も毎回指定して元の値を保持します。
func updateGroup(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	updateGroupName(t, h, run, "groupの名前と説明を更新する", run.group.name+"-renamed")
	updateGroupName(t, h, run, "groupの名前と説明を元に戻す", run.group.name)
}

func updateGroupName(t *testing.T, h *helper.Helper, run *iamRun, description, name string) {
	t.Helper()
	data := h.Call(t, description, "iam-api", "group", "update", run.group.id,
		"--name", name, "--description", run.group.description)
	updated := requireItem(t, data, "update-group")
	require.Equal(t, name, updated.Name, "更新後のgroupの名前がリクエストと一致すること")
	expected := run.group
	expected.name = name
	requireIdentity(t, updated, expected)
}

func updateMemberships(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	data := h.Call(t, "groupの所属をuserだけに更新する",
		"iam-api", "group", "update-memberships", run.group.id, "--request", "["+run.user.id+"]")
	requireMemberships(t, data, "update-memberships", run.user.id)
}

func readMemberships(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	data := h.Call(t, "groupの所属を確認する", "iam-api", "group", "read-memberships", run.group.id)
	requireMemberships(t, data, "read-memberships", run.user.id)
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

// createFolderAndProject はテスト用の folder と project を作成します。
func createFolderAndProject(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	createFolder(t, h, run)
	createProject(t, h, run)
}

// createUserAndServicePrincipal はテスト用の user と service principal を作成します。
func createUserAndServicePrincipal(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	createUser(t, h, run)
	createServicePrincipal(t, h, run)
}

// runUserLifecycleScenario は作成した user を確認し、名前とメールアドレスを更新します。
func runUserLifecycleScenario(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	verifyUser(t, h, run)
	readIAMItem(t, h, "作成したuserを読み込む", run.user)
	updateUser(t, h, run)
	registerEmail(t, h, run)
	unregisterEmail(t, h, run)
}

// renameFolderAndProject は folder と project の名前を更新します。
// 更新後の名前は後片付けの照合に使われます。
func renameFolderAndProject(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	updateFolder(t, h, run)
	updateProject(t, h, run)
}

// runGroupScenario は group を作成し、所属を user だけに更新して確認します。
// group update は名前と説明の更新と元に戻しも確認します。
func runGroupScenario(t *testing.T, h *helper.Helper, run *iamRun) {
	t.Helper()
	createGroup(t, h, run)
	updateMemberships(t, h, run)
	readMemberships(t, h, run)
	updateGroup(t, h, run)
}

// cleanupIAMResource は、後片付けとしてリソースの識別情報を再確認してから削除します。
// 削除後は名前が残っていないことを確認します。失敗は呼び出し元に返されます。
func cleanupIAMResource(t *testing.T, h *helper.Helper, target *iamResource) error {
	t.Helper()
	if !target.attempted || target.deleted {
		return nil
	}
	id := target.id
	if id == "" {
		found, ok, err := findIAMItemByName(t, h, "後片付けとして作成を試みた"+target.kind+"を検索する", *target)
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
	current, err := lookupIAMItem(t, h, "後片付けの前に"+target.kind+"の識別情報を確認する", verified)
	if err != nil {
		return fmt.Errorf("cannot verify %s before cleanup: %w", target.kind, err)
	}
	if err := checkIdentity(current, verified); err != nil {
		return fmt.Errorf("cleanup refused: %w", err)
	}
	if _, _, err := h.CallAllowingFailure(t, "後片付けとして"+target.kind+"を削除する", "iam-api", target.kind, "delete", id); err != nil {
		return fmt.Errorf("delete E2E %s: %w", target.kind, err)
	}
	names := []string{target.name}
	if target.renamedTo != "" && target.renamedTo != target.name {
		names = append(names, target.renamedTo)
	}
	for _, name := range names {
		verifyTarget := *target
		verifyTarget.name = name
		found, ok, err := findIAMItemByName(t, h, "後片付けの後に"+target.kind+"が残っていないことを確認する", verifyTarget)
		if err != nil {
			return fmt.Errorf("verify E2E %s deletion: %w", target.kind, err)
		}
		if ok {
			return fmt.Errorf("E2E %s %s remains after delete", target.kind, found.id())
		}
	}
	return nil
}

// cleanupIAMResources はシナリオが作成した一時リソースを削除します。
// organization 全体の設定は project や folder 自体より先に削除されます。
// 途中の失敗でも残りの後片付けを試み、その失敗を結果に含めます。
func cleanupIAMResources(t *testing.T, h *helper.Helper, run *iamRun) error {
	t.Helper()
	var result error
	for _, target := range []*iamResource{&run.scim, &run.ssoProfile, &run.projectAPIKey} {
		result = errors.Join(result, cleanupManagedResource(t, h, target))
	}
	for _, target := range []*iamResource{&run.servicePrincipal, &run.group, &run.user, &run.project, &run.childFolder, &run.folder} {
		result = errors.Join(result, cleanupIAMResource(t, h, target))
	}
	return result
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
