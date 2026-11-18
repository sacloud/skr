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

// Package workflowsapi は Workflows API (skr workflows-api) のコマンドを実装します。
// ワークフロー定義、Runbook リビジョン、実行、サブスクリプションの 4 グループ 21 操作を提供します。
// コマンド名は SDK の operationId から機械派生しています (ADR 0074)。
package workflowsapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	v1 "github.com/sacloud/sacloud-sdk-go/api/workflows/apis/v1"
)

// Commands は Workflows のコマンド集合です。リソース別のグループ階層を Kong の embed で
// 展開して 1 階層のフラット構造にしています (ADR 0074)。
type Commands struct {
	Workflow     WorkflowCommands             `embed:""`
	Revision     WorkflowRevisionCommands     `embed:""`
	Execution    WorkflowExecutionCommands    `embed:""`
	Subscription WorkflowSubscriptionCommands `embed:""`
}

// --- Runtime の出力処理ラッパー ---

func (r WorkflowRuntime) validateOutput(ctx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(ctx)
}

func (r WorkflowRuntime) writeOutput(ctx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(ctx, value)
}

func (r WorkflowRevisionRuntime) validateOutput(ctx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(ctx)
}

func (r WorkflowRevisionRuntime) writeOutput(ctx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(ctx, value)
}

func (r WorkflowExecutionRuntime) validateOutput(ctx *kong.Context) error {
	if r.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.ValidateOutput(ctx)
}

func (r WorkflowExecutionRuntime) writeOutput(ctx *kong.Context, value any) error {
	if r.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return r.WriteOutput(ctx, value)
}

// --- workflow: create-workflow ---

type WorkflowCreateCommand struct {
	Request         *string  `help:"個別フラグと併用不可。CreateWorkflowReq JSON を直接または @path.json、- (標準入力) で指定します。ServicePrincipalId は JSON でのみ指定できます。"`
	Name            *string  `name:"name" help:"必須: ワークフロー名。例: daily-backup"`
	RunbookFile     *string  `name:"runbook-file" help:"必須: Runbook (YAML) のファイル。- の場合は標準入力から読み込みます。"`
	Description     *string  `name:"description" help:"任意: ワークフローの説明。"`
	Publish         *bool    `name:"publish" help:"任意: 作成後に公開します。未指定時は非公開で作成します。"`
	Logging         *bool    `name:"logging" help:"任意: 実行ログの記録を有効にします。"`
	Tags            []string `name:"tags" sep:"none" help:"任意: 設定するタグ。--tags 名前 の形式で繰り返し指定します。"`
	RevisionAlias   *string  `name:"revision-alias" help:"任意: 作成時の Runbook リビジョンに付けるエイリアス。"`
	ConcurrencyMode *string  `name:"concurrency-mode" help:"任意: 同時実行の制御方法。parallel、lock、queue のいずれか。"`
	factory         WorkflowAPIFactory
	runtime         WorkflowRuntime
}

func (c *WorkflowCreateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	flags := c.Name != nil || c.RunbookFile != nil || c.Description != nil ||
		c.Publish != nil || c.Logging != nil || c.ConcurrencyMode != nil ||
		c.RevisionAlias != nil || len(c.Tags) != 0
	if c.Request != nil && flags {
		return fmt.Errorf("--request と個別フラグは併用できません")
	}

	var request v1.CreateWorkflowReq
	if c.Request != nil {
		if c.runtime.DecodeRequest == nil {
			return fmt.Errorf("API リクエスト処理が設定されていません")
		}
		if err := c.runtime.DecodeRequest(*c.Request, &request); err != nil {
			return err
		}
	} else {
		if c.Name == nil || *c.Name == "" {
			return fmt.Errorf("--name が必要です (--request JSON も指定できます)")
		}
		if c.RunbookFile == nil || *c.RunbookFile == "" {
			return fmt.Errorf("--runbook-file が必要です (--request JSON も指定できます)")
		}
		runbook, err := readRunbookFile(*c.RunbookFile)
		if err != nil {
			return err
		}
		request = v1.CreateWorkflowReq{
			Name:    *c.Name,
			Runbook: runbook,
			Publish: c.Publish != nil && *c.Publish,
			Logging: c.Logging != nil && *c.Logging,
		}
		if c.Description != nil {
			request.Description = v1.NewOptString(*c.Description)
		}
		if c.RevisionAlias != nil {
			request.RevisionAlias = v1.NewOptString(*c.RevisionAlias)
		}
		if c.ConcurrencyMode != nil {
			if err := validateConcurrencyModeValue(*c.ConcurrencyMode); err != nil {
				return err
			}
			request.ConcurrencyMode = NewOptWorkflowConcurrencyMode(*c.ConcurrencyMode)
		}
		if len(c.Tags) != 0 {
			tags := make([]v1.CreateWorkflowReqTagsItem, 0, len(c.Tags))
			for _, name := range c.Tags {
				tags = append(tags, v1.CreateWorkflowReqTagsItem{Name: name})
			}
			request.Tags = tags
		}
	}

	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	workflow, err := op.Create(context.Background(), request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, workflow)
}

// validateConcurrencyModeValue は create-workflow の --concurrency-mode の値を検証します。
func validateConcurrencyModeValue(value string) error {
	switch v1.CreateWorkflowReqConcurrencyMode(value) {
	case v1.CreateWorkflowReqConcurrencyModeParallel, v1.CreateWorkflowReqConcurrencyModeLock, v1.CreateWorkflowReqConcurrencyModeQueue:
		return nil
	default:
		return fmt.Errorf("--concurrency-mode には parallel、lock、queue のいずれかを指定してください")
	}
}

// readRunbookFile は --runbook-file の内容を読み込みます。- で標準入力から読み込めます。
func readRunbookFile(path string) (string, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read runbook from standard input: %w", err)
		}
		return string(data), nil
	}
	//nolint:gosec // パスは --runbook-file でユーザーが明示的に指定したものです。
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read runbook file: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return "", fmt.Errorf("runbook file is empty")
	}
	return string(data), nil
}

// --- revision: create-revision ---

type WorkflowRevisionCreateCommand struct {
	WorkflowID    string  `arg:"" name:"workflow-id" help:"リビジョンを登録するワークフロー ID。"`
	Request       *string `help:"個別フラグと併用不可。CreateRevisionReq JSON を直接または @path.json、- (標準入力) で指定します。Runbook には Runbook (YAML) 全文を指定します。"`
	RunbookFile   *string `name:"runbook-file" help:"必須: Runbook (YAML) のファイル。- の場合は標準入力から読み込みます。"`
	RevisionAlias *string `name:"revision-alias" help:"任意: 登録するリビジョンに付けるエイリアス。"`
	factory       WorkflowRevisionAPIFactory
	runtime       WorkflowRevisionRuntime
}

func (c *WorkflowRevisionCreateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	flags := c.RunbookFile != nil || c.RevisionAlias != nil
	if c.Request != nil && flags {
		return fmt.Errorf("--request と個別フラグは併用できません")
	}

	var request v1.CreateRevisionReq
	if c.Request != nil {
		if c.runtime.DecodeRequest == nil {
			return fmt.Errorf("API リクエスト処理が設定されていません")
		}
		if err := c.runtime.DecodeRequest(*c.Request, &request); err != nil {
			return err
		}
	} else {
		if c.RunbookFile == nil || *c.RunbookFile == "" {
			return fmt.Errorf("--runbook-file が必要です (--request JSON も指定できます)")
		}
		runbook, err := readRunbookFile(*c.RunbookFile)
		if err != nil {
			return err
		}
		request = v1.CreateRevisionReq{Runbook: runbook}
		if c.RevisionAlias != nil {
			request.RevisionAlias = v1.NewOptString(*c.RevisionAlias)
		}
	}

	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	revision, err := op.Create(context.Background(), c.WorkflowID, request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, revision)
}

// --- revision: list-revisions ---

type WorkflowRevisionListCommand struct {
	WorkflowID string  `arg:"" name:"workflow-id" help:"リビジョンを一覧表示するワークフロー ID。"`
	Page       *int    `name:"page" help:"任意: ページ番号。"`
	PageLimit  *int    `name:"page-limit" help:"任意: 1 ページあたりの取得件数。"`
	SortBy     *string `name:"sort-by" help:"任意: ソートに使うプロパティ。createdAt、updatedAt、id のいずれか。"`
	Order      *string `name:"order" help:"任意: ソートの順序。asc または desc。"`
	Published  *bool   `name:"published" help:"任意: 公開状態で絞り込みます。"`
	factory    WorkflowRevisionAPIFactory
	runtime    WorkflowRevisionRuntime
}

func (c *WorkflowRevisionListCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	request := v1.ListRevisionsParams{ID: c.WorkflowID}
	if c.Page != nil {
		request.Page = v1.NewOptInt(*c.Page)
	}
	if c.PageLimit != nil {
		request.PageLimit = v1.NewOptInt(*c.PageLimit)
	}
	if c.SortBy != nil {
		request.SortBy = NewOptRevisionSortBy(*c.SortBy)
	}
	if c.Order != nil {
		request.Order = NewOptRevisionOrder(*c.Order)
	}
	if c.Published != nil {
		request.Published = v1.NewOptBool(*c.Published)
	}
	if err := validateRevisionListRequest(&request); err != nil {
		return err
	}

	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	result, err := op.List(context.Background(), request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

// validateRevisionListRequest は list-revisions の列挙値フラグを検証します。
func validateRevisionListRequest(request *v1.ListRevisionsParams) error {
	if request.SortBy.Set {
		switch request.SortBy.Value {
		case v1.ListRevisionsSortByCreatedAt, v1.ListRevisionsSortByUpdatedAt, v1.ListRevisionsSortByID:
		default:
			return fmt.Errorf("--sort-by には createdAt、updatedAt、id のいずれかを指定してください")
		}
	}
	if request.Order.Set {
		switch request.Order.Value {
		case v1.ListRevisionsOrderAsc, v1.ListRevisionsOrderDesc:
		default:
			return fmt.Errorf("--order には asc または desc を指定してください")
		}
	}
	return nil
}

// --- execution: create-execution ---

type WorkflowExecutionCreateCommand struct {
	WorkflowID    string  `arg:"" name:"workflow-id" help:"実行するワークフロー ID。"`
	Request       *string `help:"個別フラグと併用不可。CreateExecutionReq JSON を直接または @path.json、- (標準入力) で指定します。RevisionId、RevisionAlias、Args、Name を指定できます。"`
	RevisionID    *int    `name:"revision-id" help:"任意: 実行するリビジョン番号。--revision-alias と同時に指定できません。"`
	RevisionAlias *string `name:"revision-alias" help:"任意: 実行するリビジョンのエイリアス。--revision-id と同時に指定できません。"`
	Args          *string `name:"args" help:"任意: 実行時に Runbook へ渡す JSON。JSON 文字列を直接指定するか、@path.json、- (標準入力) で読み込みます。"`
	Name          *string `name:"name" help:"任意: 実行の名前。"`
	factory       WorkflowExecutionAPIFactory
	runtime       WorkflowExecutionRuntime
}

func (c *WorkflowExecutionCreateCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	flags := c.RevisionID != nil || c.RevisionAlias != nil || c.Args != nil || c.Name != nil
	if c.Request != nil && flags {
		return fmt.Errorf("--request と個別フラグは併用できません")
	}

	var request v1.CreateExecutionReq
	if c.Request != nil {
		if c.runtime.DecodeRequest == nil {
			return fmt.Errorf("API リクエスト処理が設定されていません")
		}
		if err := c.runtime.DecodeRequest(*c.Request, &request); err != nil {
			return err
		}
	} else {
		if c.RevisionID != nil && c.RevisionAlias != nil {
			return fmt.Errorf("--revision-id と --revision-alias は同時に指定できません")
		}
		if c.Args != nil {
			args, err := readArgsValue(*c.Args)
			if err != nil {
				return err
			}
			request.Args = v1.NewOptString(args)
		}
		if c.RevisionID != nil {
			request.RevisionId = v1.NewOptInt(*c.RevisionID)
		}
		if c.RevisionAlias != nil {
			request.RevisionAlias = v1.NewOptString(*c.RevisionAlias)
		}
		if c.Name != nil {
			request.Name = v1.NewOptString(*c.Name)
		}
	}

	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	execution, err := op.Create(context.Background(), c.WorkflowID, v1.NewOptCreateExecutionReq(request))
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, execution)
}

// readArgsValue は --args の値を受け取ります。@path と - (標準入力) でファイルから読み込み、
// 内容が JSON として妥当か検証します。Args は API に JSON 文字列として送信されます。
func readArgsValue(input string) (string, error) {
	var data []byte
	switch {
	case input == "-":
		read, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read args from standard input: %w", err)
		}
		data = read
	case strings.HasPrefix(input, "@"):
		path := strings.TrimPrefix(input, "@")
		if path == "" {
			return "", fmt.Errorf("--args のファイルパスが空です")
		}
		//nolint:gosec // パスは --args でユーザーが明示的に指定したものです。
		read, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read args file: %w", err)
		}
		data = read
	default:
		data = []byte(input)
	}
	if !json.Valid(data) {
		return "", fmt.Errorf("--args には JSON として妥当な文字列を指定してください")
	}
	return string(data), nil
}

// --- execution: list-executions ---

type WorkflowExecutionListCommand struct {
	WorkflowID string  `arg:"" name:"workflow-id" help:"実行を一覧表示するワークフロー ID。"`
	Page       *int    `name:"page" help:"任意: ページ番号。"`
	PageLimit  *int    `name:"page-limit" help:"任意: 1 ページあたりの取得件数。"`
	Order      *string `name:"order" help:"任意: ソートの順序。asc または desc。"`
	factory    WorkflowExecutionAPIFactory
	runtime    WorkflowExecutionRuntime
}

func (c *WorkflowExecutionListCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	request := v1.ListExecutionsParams{ID: c.WorkflowID}
	if c.Page != nil {
		request.Page = v1.NewOptInt(*c.Page)
	}
	if c.PageLimit != nil {
		request.PageLimit = v1.NewOptInt(*c.PageLimit)
	}
	if c.Order != nil {
		request.Order = NewOptExecutionOrder(*c.Order)
	}
	if request.Order.Set {
		switch request.Order.Value {
		case v1.ListExecutionsOrderAsc, v1.ListExecutionsOrderDesc:
		default:
			return fmt.Errorf("--order には asc または desc を指定してください")
		}
	}

	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	result, err := op.List(context.Background(), request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}

// --- execution: list-execution-histories ---

type WorkflowExecutionListHistoryCommand struct {
	WorkflowID  string  `arg:"" name:"workflow-id" help:"実行が属するワークフロー ID。"`
	ExecutionID string  `arg:"" name:"execution-id" help:"履歴を見る実行 ID。create-execution の出力にある ExecutionId。"`
	Page        *int    `name:"page" help:"任意: ページ番号。"`
	PageLimit   *int    `name:"page-limit" help:"任意: 1 ページあたりの取得件数。"`
	SortOrder   *string `name:"sort-order" help:"任意: ソートの順序。asc または desc。"`
	factory     WorkflowExecutionAPIFactory
	runtime     WorkflowExecutionRuntime
}

func (c *WorkflowExecutionListHistoryCommand) Run(ctx *kong.Context) error {
	if err := c.runtime.validateOutput(ctx); err != nil {
		return err
	}
	request := v1.ListExecutionHistoriesParams{ID: c.WorkflowID, ExecutionId: c.ExecutionID}
	if c.Page != nil {
		request.Page = v1.NewOptInt(*c.Page)
	}
	if c.PageLimit != nil {
		request.PageLimit = v1.NewOptInt(*c.PageLimit)
	}
	if c.SortOrder != nil {
		request.SortOrder = NewOptExecutionHistorySortOrder(*c.SortOrder)
	}
	if request.SortOrder.Set {
		switch request.SortOrder.Value {
		case v1.ListExecutionHistoriesSortOrderAsc, v1.ListExecutionHistoriesSortOrderDesc:
		default:
			return fmt.Errorf("--sort-order には asc または desc を指定してください")
		}
	}

	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	result, err := op.ListHistory(context.Background(), request)
	if err != nil {
		return err
	}
	return c.runtime.writeOutput(ctx, result)
}
