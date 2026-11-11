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
	"encoding/json"
	"errors"
	"fmt"

	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	version "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/version"
)

// DiffResult は diff の比較対象です。両面とも正規化した定義の JSON オブジェクト
// (map[string]any) です。unified diff の文字列化は CLI 層が行います。
type DiffResult struct {
	// Local は正規化したローカルの定義です。
	Local map[string]any `json:"local"`
	// Remote は正規化したデプロイ済みの現状です。アプリケーションが存在しないか
	// 非活性の場合は、識別子 (cluster、name) のみの定義になります。
	Remote map[string]any `json:"remote"`
}

// Render は定義を検証して正規形 (cluster、name を含む定義の正規化結果) を返します。
//
// 差分のローカル側と同じ形式です。registryPassword と secret=true の環境変数の値は
// 出力に含めません (Diff の比較から除外するのと同じ処理です)。
func (s *Service) Render(def *ApplicationDefinition) (map[string]any, error) {
	if def == nil {
		return nil, errors.New("定義が nil です")
	}
	if err := def.ValidateForDeploy(); err != nil {
		return nil, err
	}
	return normalizedMap(def), nil
}

// Diff はローカルの定義と、デプロイ済みの現状 (アクティブバージョン) を正規化して
// 返します。
//
// 現状は Version.Read の結果を定義と同じ形式に正規化します。アプリケーションが存在
// しないか非活性の場合は、識別子のみの定義を現状とします (初回デプロイで作成される
// 内容の確認)。registryPassword、secret=true の環境変数の値、registryPasswordAction
// は比較から除外します。応答ではマスクされるか、定義に現れないフィールドのためです。
// unified diff の文字列化は CLI 層が行います。
func (s *Service) Diff(ctx context.Context, def *ApplicationDefinition) (*DiffResult, error) {
	if def == nil {
		return nil, errors.New("定義が nil です")
	}
	if err := def.ValidateForDeploy(); err != nil {
		return nil, err
	}
	local := normalizedMap(def)

	detail, err := s.findApplication(ctx, def)
	if err != nil {
		var notFound *ApplicationNotFoundError
		if errors.As(err, &notFound) {
			return &DiffResult{Local: local, Remote: identifierMap(def)}, nil
		}
		return nil, err
	}
	if detail.ActiveVersion == nil {
		return &DiffResult{Local: local, Remote: identifierMap(def)}, nil
	}
	versionDetail, err := s.newVersionAPI(detail.ApplicationID).Read(ctx, v1.ApplicationVersionNumber(*detail.ActiveVersion))
	if err != nil {
		return nil, fmt.Errorf("アクティブバージョン %d の情報取得に失敗しました: %w", *detail.ActiveVersion, err)
	}
	remote := normalizedMap(definitionFromVersionDetail(versionDetail, def.Cluster, def.Name))
	return &DiffResult{Local: local, Remote: remote}, nil
}

// identifierMap は識別子のみの定義を正規化形式で返します。
func identifierMap(def *ApplicationDefinition) map[string]any {
	return normalizedMap(&ApplicationDefinition{Cluster: def.Cluster, Name: def.Name})
}

// normalizedMap は定義を JSON オブジェクトへ正規化します。
//
// registryPassword と secret=true の環境変数の値を除外してから map へ変換します。
// 変換元の定義を変更しません。JSON 経由で変換するため、両面でキーの並びが揃います。
func normalizedMap(def *ApplicationDefinition) map[string]any {
	data, err := json.Marshal(maskSecrets(def))
	if err != nil {
		// 定義は JSON に変換可能な型のみで構成されるため、ここには到達しません。
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]any{}
	}
	return m
}

// maskSecrets は定義のコピーから秘密情報を除きます。
//
// registryPassword と secret=true の環境変数の値が対象です。応答ではマスクされるか
// null になるため、比較と出力から除外します。
func maskSecrets(def *ApplicationDefinition) *ApplicationDefinition {
	masked := *def
	masked.RegistryPassword = nil
	if len(def.Env) > 0 {
		env := make([]version.EnvironmentVariable, len(def.Env))
		copy(env, def.Env)
		for i := range env {
			if env[i].Secret {
				env[i].Value = nil
			}
		}
		masked.Env = env
	}
	return &masked
}
