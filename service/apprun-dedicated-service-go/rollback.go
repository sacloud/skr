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

package apprundedicated

import (
	"context"
	"errors"
	"fmt"
	"time"

	application "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/application"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
)

// RollbackOptions は Rollback のオプションです。
type RollbackOptions struct {
	// Target は有効化するバージョン番号です。nil の場合はアプリケーションの状態から
	// 既定の対象を選択します (Rollback を参照)。
	Target *v1.ApplicationVersionNumber
	// NoWait が true の場合、有効化後の反映待ちを行いません。
	NoWait bool
	// Timeout は反映待ちのタイムアウトです。零値の場合は DefaultWaitTimeout を使います。
	Timeout time.Duration
}

// Rollback は既存バージョンを有効化して、有効バージョンを切り替えます。
//
// 新しいバージョンは作成しません。opts.Target が nil の場合、アプリケーションが活性
// (アクティブバージョンあり) ならアクティブバージョンより前のバージョンのうち最大の
// ものを、非活性なら最新のバージョンを選択します。対象バージョンがすでにアクティブな
// 場合や存在しない場合は、変更 API を呼び出さずにエラーを返します。
func (s *Service) Rollback(ctx context.Context, def *ApplicationDefinition, opts *RollbackOptions) (*DeployResult, error) {
	if def == nil {
		return nil, errors.New("定義が nil です")
	}
	if err := def.Validate(); err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &RollbackOptions{}
	}

	// 変更直前の再照合を兼ねます。findApplication は定義の cluster、name と対象の
	// ClusterName、Name が一致するものだけを返します。
	detail, err := s.findApplication(ctx, def)
	if err != nil {
		return nil, err
	}

	target, err := s.selectRollbackVersion(ctx, detail, opts.Target)
	if err != nil {
		return nil, err
	}

	if err := ensureActive(ctx); err != nil {
		return nil, err
	}
	if err := s.applicationAPI.Update(ctx, detail.ApplicationID, toInt32Ptr(target)); err != nil {
		return nil, fmt.Errorf("バージョン %d の有効化に失敗しました: %w", target, err)
	}

	if !opts.NoWait {
		if err := s.waitRunning(ctx, detail.ApplicationID, target, opts.Timeout); err != nil {
			return nil, fmt.Errorf("バージョン %d の反映待ちに失敗しました (バージョンの有効化は完了しています): %w", target, err)
		}
	}
	return &DeployResult{
		Cluster:       def.Cluster,
		Application:   def.Name,
		ApplicationID: detail.ApplicationID,
		Version:       target,
	}, nil
}

// selectRollbackVersion は rollback の対象バージョンを選択します。
//
// バージョンの存在は Version.List のページングで確認します。
func (s *Service) selectRollbackVersion(ctx context.Context, detail *application.ApplicationDetail, target *v1.ApplicationVersionNumber) (v1.ApplicationVersionNumber, error) {
	versions, err := s.listAllVersions(ctx, detail.ApplicationID)
	if err != nil {
		return 0, err
	}
	if len(versions) == 0 {
		return 0, errors.New("バージョンが 1 つもないため rollback できません")
	}

	if target != nil {
		found := false
		for _, v := range versions {
			if v.Version == *target {
				found = true
				break
			}
		}
		if !found {
			return 0, fmt.Errorf("バージョン %d が見つかりません", *target)
		}
		if detail.ActiveVersion != nil && *detail.ActiveVersion == int32(*target) {
			return 0, fmt.Errorf("バージョン %d はすでにアクティブです", *target)
		}
		return *target, nil
	}

	if detail.ActiveVersion != nil {
		active := v1.ApplicationVersionNumber(*detail.ActiveVersion)
		var prev *v1.ApplicationVersionNumber
		for _, v := range versions {
			if v.Version < active && (prev == nil || v.Version > *prev) {
				copied := v.Version
				prev = &copied
			}
		}
		if prev == nil {
			return 0, fmt.Errorf("アクティブバージョン %d より前のバージョンがありません", active)
		}
		return *prev, nil
	}

	// 非活性のアプリケーションの再開には最新のバージョンを使います。
	latest := versions[0].Version
	for _, v := range versions[1:] {
		if v.Version > latest {
			latest = v.Version
		}
	}
	return latest, nil
}
