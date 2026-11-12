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

// DeployOptions は Deploy の待機に関するオプションです。
type DeployOptions struct {
	// NoWait が true の場合、有効化後の反映待ちを行いません。
	NoWait bool
	// Timeout は反映待ちのタイムアウトです。零値の場合は DefaultWaitTimeout を使います。
	Timeout time.Duration
}

// DeployResult は deploy と rollback の結果です。
type DeployResult struct {
	// Cluster はデプロイ先クラスタの名前です。
	Cluster string `json:"cluster"`
	// Application はアプリケーションの名前です。
	Application string `json:"application"`
	// ApplicationID はアプリケーションの ID です。
	ApplicationID v1.ApplicationID `json:"applicationID"`
	// Version は有効化したバージョン番号です。
	Version v1.ApplicationVersionNumber `json:"version"`
	// ApplicationCreated は Deploy でアプリケーションを新規作成したかどうかです。
	// rollback では常に false です。
	ApplicationCreated bool `json:"applicationCreated"`
}

// Deploy は定義の内容をデプロイします。
//
// 定義を ValidateForDeploy で検証し、クラスタ名でクラスタを検索したうえで、
// アプリケーションを検索または作成し、定義からバージョンを作成して有効化します。
// registryPasswordAction は定義に非空の registryPassword があれば new、アクティブ
// バージョンがあれば keep、それ以外 (初回デプロイ) は remove です。
//
// opts が nil の場合は待機あり (タイムアウト DefaultWaitTimeout) で動作します。
// NoWait が true の場合は有効化まで行い、反映を待ちません。反映待ちの完了条件は、
// 要求されたコンテナ数 (DesiredCount) と同じ数のコンテナが作成したバージョンで
// すべて running 状態であることです。ポーリング中の参照エラーは打ち切らず、
// 期限まで再試行します。
func (s *Service) Deploy(ctx context.Context, def *ApplicationDefinition, opts *DeployOptions) (*DeployResult, error) {
	if def == nil {
		return nil, errors.New("定義が nil です")
	}
	if err := def.ValidateForDeploy(); err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &DeployOptions{}
	}

	clusterDetail, err := s.findCluster(ctx, def.Cluster)
	if err != nil {
		return nil, err
	}

	detail, created, err := s.findOrCreateApplication(ctx, def, clusterDetail.ClusterID)
	if err != nil {
		return nil, err
	}

	params := def.createParams(registryPasswordAction(def, detail))
	if err := ensureActive(ctx); err != nil {
		return nil, err
	}
	createdVersion, err := s.newVersionAPI(detail.ApplicationID).Create(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("バージョンの作成に失敗しました: %w", err)
	}
	version := createdVersion.Version

	if err := ensureActive(ctx); err != nil {
		return nil, err
	}
	if err := s.applicationAPI.Update(ctx, detail.ApplicationID, toInt32Ptr(version)); err != nil {
		return nil, fmt.Errorf("バージョン %d の有効化に失敗しました: %w", version, err)
	}

	if !opts.NoWait {
		if err := s.waitRunning(ctx, detail.ApplicationID, version, opts.Timeout); err != nil {
			return nil, fmt.Errorf("バージョン %d の反映待ちに失敗しました (バージョンの作成と有効化は完了しています): %w", version, err)
		}
	}
	return &DeployResult{
		Cluster:            def.Cluster,
		Application:        def.Name,
		ApplicationID:      detail.ApplicationID,
		Version:            version,
		ApplicationCreated: created,
	}, nil
}

// registryPasswordAction は deploy の文脈から registryPasswordAction を決定します。
//
// 誤るとパスワードが消去 (remove) されるため、決定は service 層に集めてテストで
// 固定します。registryPassword はリクエスト・ログ・出力に含めません。
func registryPasswordAction(def *ApplicationDefinition, detail *application.ApplicationDetail) v1.RegistryPasswordAction {
	if def.RegistryPassword != nil && *def.RegistryPassword != "" {
		return v1.RegistryPasswordActionNew
	}
	if detail.ActiveVersion != nil {
		return v1.RegistryPasswordActionKeep
	}
	return v1.RegistryPasswordActionRemove
}

// waitRunning は、要求されたコンテナ数 (DesiredCount) と同じ数のコンテナが
// 対象バージョンで running になるまで待機します。
//
// 1つのワーカノードには同一アプリケーションのコンテナが最大1つ配置されるため
// (https://manual.sakura.ad.jp/cloud/apprun-dedicated/glossary.html「技術概要」の
// 「コンテナのスケジューリング」)、要求されたコンテナ数を配置できるワーカノードが
// ない場合、コンテナは要求した数まで現れません。その場合も反映完了とは判定せず、
// 期限まで待機してタイムアウトします。
func (s *Service) waitRunning(ctx context.Context, id v1.ApplicationID, version v1.ApplicationVersionNumber, timeout time.Duration) error {
	return s.poll(ctx, timeout, func() (bool, error) {
		detail, err := s.applicationAPI.Read(ctx, id)
		if err != nil {
			// 参照エラーは打ち切らず、期限まで再試行します。
			return false, nil
		}
		if detail.DesiredCount == nil || *detail.DesiredCount <= 0 {
			// 要求されたコンテナ数が確定していない間は完了と判定しません。
			return false, nil
		}
		placements, err := s.applicationAPI.Containers(ctx, id)
		if err != nil {
			// 参照エラーは打ち切らず、期限まで再試行します。
			return false, nil
		}
		running, total := containersRunning(placements, version)
		return total == int(*detail.DesiredCount) && running == total, nil
	})
}

// containersRunning は対象バージョンのコンテナ数 (total) と、そのうち running 状態の
// 数 (running) を返します。
func containersRunning(placements []application.Placement, version v1.ApplicationVersionNumber) (running, total int) {
	for _, p := range placements {
		for _, c := range p.ContainersStats.Containers {
			if c.ApplicationVersion == version {
				total++
				if c.State == "running" {
					running++
				}
			}
		}
	}
	return running, total
}
