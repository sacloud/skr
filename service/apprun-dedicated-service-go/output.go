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

// Status は status の出力です。射影から registryPassword を除外します。
type Status struct {
	// Cluster はクラスタの名前です。
	Cluster string `json:"cluster"`
	// Application はアプリケーションの状態です。
	Application ApplicationStatus `json:"application"`
	// ActiveVersionDetail はアクティブバージョンの内容です。
	// アクティブバージョンがなければ null になります。
	ActiveVersionDetail *ActiveVersionSummary `json:"activeVersionDetail"`
}

// ApplicationStatus はアプリケーションの状態です。
type ApplicationStatus struct {
	// ID はアプリケーションの ID です。
	ID v1.ApplicationID `json:"applicationID"`
	// Name はアプリケーションの名前です。
	Name string `json:"name"`
	// ActiveVersion はアクティブバージョンの番号です。非活性の場合は null になります。
	ActiveVersion *int32 `json:"activeVersion"`
	// DesiredCount はデプロイされているべきコンテナ数です。
	DesiredCount *int32 `json:"desiredCount"`
}

// ActiveVersionSummary はアクティブバージョンの内容です。
type ActiveVersionSummary struct {
	// Version はバージョン番号です。
	Version v1.ApplicationVersionNumber `json:"version"`
	// Image はコンテナイメージです。
	Image string `json:"image"`
	// CPU は割り当てる CPU です (単位: mCPU)。
	CPU int64 `json:"cpu"`
	// Memory は割り当てるメモリです (単位: MB)。
	Memory int64 `json:"memory"`
	// ScalingMode はスケーリング方式です。
	ScalingMode v1.ScalingMode `json:"scalingMode"`
	// ActiveNodeCount は現在動いているノードの数です。
	ActiveNodeCount int64 `json:"activeNodeCount"`
}

// VersionEntry は versions の出力の要素です。
type VersionEntry struct {
	// Version はバージョン番号です。
	Version v1.ApplicationVersionNumber `json:"version"`
	// Image はコンテナイメージです。
	Image string `json:"image"`
	// ActiveNodeCount は現在動いているノードの数です。
	ActiveNodeCount int64 `json:"activeNodeCount"`
	// Active はアプリケーションのアクティブバージョンと一致するかどうかです。
	Active bool `json:"active"`
	// Created はバージョンの作成日時です (UTC)。
	Created time.Time `json:"created"`
}

// Status はアプリケーションの状態を返します。
//
// アクティブバージョンがあればその内容を取得して ActiveVersionDetail に格納します。
func (s *Service) Status(ctx context.Context, def *ApplicationDefinition) (*Status, error) {
	if def == nil {
		return nil, errors.New("定義が nil です")
	}
	if err := def.Validate(); err != nil {
		return nil, err
	}
	detail, err := s.findApplication(ctx, def)
	if err != nil {
		return nil, err
	}
	status := &Status{
		Cluster: def.Cluster,
		Application: ApplicationStatus{
			ID:            detail.ApplicationID,
			Name:          detail.Name,
			ActiveVersion: detail.ActiveVersion,
			DesiredCount:  detail.DesiredCount,
		},
	}
	if detail.ActiveVersion == nil {
		return status, nil
	}
	versionDetail, err := s.newVersionAPI(detail.ApplicationID).Read(ctx, v1.ApplicationVersionNumber(*detail.ActiveVersion))
	if err != nil {
		return nil, fmt.Errorf("アクティブバージョン %d の情報取得に失敗しました: %w", *detail.ActiveVersion, err)
	}
	status.ActiveVersionDetail = &ActiveVersionSummary{
		Version:         versionDetail.Version,
		Image:           versionDetail.Image,
		CPU:             versionDetail.CPU,
		Memory:          versionDetail.Memory,
		ScalingMode:     versionDetail.ScalingMode,
		ActiveNodeCount: versionDetail.ActiveNodeCount,
	}
	return status, nil
}

// Versions はアプリケーションのバージョンの配列を返します。
func (s *Service) Versions(ctx context.Context, def *ApplicationDefinition) ([]VersionEntry, error) {
	if def == nil {
		return nil, errors.New("定義が nil です")
	}
	if err := def.Validate(); err != nil {
		return nil, err
	}
	detail, err := s.findApplication(ctx, def)
	if err != nil {
		return nil, err
	}
	versions, err := s.listAllVersions(ctx, detail.ApplicationID)
	if err != nil {
		return nil, err
	}
	entries := make([]VersionEntry, 0, len(versions))
	for _, v := range versions {
		entries = append(entries, VersionEntry{
			Version:         v.Version,
			Image:           v.Image,
			ActiveNodeCount: v.ActiveNodeCount,
			Active:          detail.ActiveVersion != nil && v.Version == v1.ApplicationVersionNumber(*detail.ActiveVersion),
			Created:         time.Unix(int64(v.Created), 0).UTC(),
		})
	}
	return entries, nil
}

// Containers はコンテナの配置を返します。SDK の応答モデルをそのまま返します。
func (s *Service) Containers(ctx context.Context, def *ApplicationDefinition) ([]application.Placement, error) {
	if def == nil {
		return nil, errors.New("定義が nil です")
	}
	if err := def.Validate(); err != nil {
		return nil, err
	}
	detail, err := s.findApplication(ctx, def)
	if err != nil {
		return nil, err
	}
	placements, err := s.applicationAPI.Containers(ctx, detail.ApplicationID)
	if err != nil {
		return nil, fmt.Errorf("コンテナの配置取得に失敗しました: %w", err)
	}
	return placements, nil
}
