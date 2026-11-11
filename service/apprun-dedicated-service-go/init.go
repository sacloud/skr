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

	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	version "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/version"
)

// Init は既存アプリケーションから定義を復元します。
//
// アクティブバージョンがあればその内容から、なければ最新のバージョンから定義を
// 復元します。バージョンが 1 つもなければ、識別子だけの定義を返します。
// registryPassword と secret=true の環境変数の値は API から取得できないため、
// 生成される定義には含めません (値は null になります)。ファイルへの書き出しは
// 呼び出し側の責務です。
func (s *Service) Init(ctx context.Context, clusterName, applicationName string) (*ApplicationDefinition, error) {
	if clusterName == "" {
		return nil, errors.New("クラスタ名が空です")
	}
	if applicationName == "" {
		return nil, errors.New("アプリケーション名が空です")
	}
	def := &ApplicationDefinition{Cluster: clusterName, Name: applicationName}

	detail, err := s.findApplication(ctx, def)
	if err != nil {
		return nil, err
	}
	// クラスタ名とアプリケーション名は API が返した値を優先します。
	def.Cluster = detail.ClusterName
	def.Name = detail.Name

	versionAPI := s.newVersionAPI(detail.ApplicationID)
	var versionDetail *version.VersionDetail
	if detail.ActiveVersion != nil {
		versionDetail, err = versionAPI.Read(ctx, v1.ApplicationVersionNumber(*detail.ActiveVersion))
		if err != nil {
			return nil, fmt.Errorf("アクティブバージョン %d の情報取得に失敗しました: %w", *detail.ActiveVersion, err)
		}
	} else {
		versions, err := s.listAllVersions(ctx, detail.ApplicationID)
		if err != nil {
			return nil, err
		}
		if len(versions) == 0 {
			return def, nil
		}
		latest := versions[0].Version
		for _, v := range versions[1:] {
			if v.Version > latest {
				latest = v.Version
			}
		}
		versionDetail, err = versionAPI.Read(ctx, latest)
		if err != nil {
			return nil, fmt.Errorf("バージョン %d の情報取得に失敗しました: %w", latest, err)
		}
	}
	return definitionFromVersionDetail(versionDetail, def.Cluster, def.Name), nil
}
