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

// Package apprundedicated は、さくらのクラウド AppRun 専有型のアプリケーションを
// JSON の定義ファイルからデプロイする高レベルライブラリです。
//
// 低レベルの application・version・cluster 操作を組み合わせた deploy、rollback、
// deactivate、delete のフローと、status・versions・containers の参照、既存リソース
// からの定義復元 (Init)、定義の正規化 (Render、Diff) を提供します。
//
// 使い方の概要は次のとおりです。
//
//	def, err := apprundedicated.ParseDefinition(data)
//	if err != nil {
//		return err
//	}
//	result, err := service.Deploy(ctx, def, nil)
//	if err != nil {
//		return err
//	}
package apprundedicated

import (
	"context"
	"errors"
	"fmt"

	application "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/application"
	cluster "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/cluster"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	version "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/version"
)

// listPageSize は一覧取得のページサイズです。テストでページングを検証するために var にしています。
var listPageSize = int64(100)

// Service は AppRun 専有型のアプリケーションに対するデプロイフローを提供します。
//
// 各メソッドは SDK の低レベル API (application・version・cluster 操作) を組み合わせ、
// クラスタとアプリケーションの名前検索、registryPasswordAction の決定、変更前の
// 再照合、反映待ちのポーリングを行います。変更 API を呼び出す前に context の
// キャンセルを再確認します。読み取りと変更の間に他の利用者による変更がないことは
// 検証しません。
//
// 零値の Service は利用できません。New で構築してください。
type Service struct {
	applicationAPI application.ApplicationAPI
	clusterAPI     cluster.ClusterAPI
	// newVersionAPI はアプリケーション ID に紐づくバージョン操作を構築します。
	// バージョン操作はアプリケーションが確定した後に必要になるため、構築を遅らせます。
	newVersionAPI func(v1.ApplicationID) version.VersionAPI
	clock         clock
}

// New は AppRun Dedicated API クライアント (sacloud-sdk-go の api/apprun-dedicated
// パッケージ) から Service を構築します。
//
//	var client saclient.Client
//	if err := client.SetEnviron(os.Environ()); err != nil {
//		return err
//	}
//	v1Client, err := apprundedsdk.NewClient(&client)
//	if err != nil {
//		return err
//	}
//	service := apprundedicated.New(v1Client)
func New(client *v1.Client) *Service {
	return &Service{
		applicationAPI: application.NewApplicationOp(client),
		clusterAPI:     cluster.NewClusterOp(client),
		newVersionAPI: func(id v1.ApplicationID) version.VersionAPI {
			return version.NewVersionOp(client, id)
		},
		clock: realClock{},
	}
}

// ApplicationNotFoundError は、指定されたクラスタに同名のアプリケーションが
// 存在しないことを表します。errors.As での判定に使えます。
type ApplicationNotFoundError struct {
	// Cluster は検索したクラスタの名前です。
	Cluster string
	// Name は検索したアプリケーションの名前です。
	Name string
}

func (e *ApplicationNotFoundError) Error() string {
	return fmt.Sprintf("クラスタ %q にアプリケーション %q が見つかりません", e.Cluster, e.Name)
}

// findCluster はクラスタ名でクラスタを検索します。見つからない場合はエラーを返します。
func (s *Service) findCluster(ctx context.Context, name string) (*cluster.ClusterDetail, error) {
	var cursor *v1.ClusterID
	for {
		list, next, err := s.clusterAPI.List(ctx, listPageSize, cursor)
		if err != nil {
			return nil, fmt.Errorf("クラスタの一覧取得に失敗しました: %w", err)
		}
		for i := range list {
			if list[i].Name == name {
				return &list[i], nil
			}
		}
		if next == nil {
			return nil, fmt.Errorf("クラスタ %q が見つかりません", name)
		}
		cursor = next
	}
}

// findApplication は定義の cluster と name でアプリケーションを検索します。
//
// アプリケーション名はクラスタ内で一意であるため、一覧の名前一致に対して Read で
// ClusterName を照合し、クラスタが一致するものだけを対象にします。見つからない場合は
// *ApplicationNotFoundError を返します。
func (s *Service) findApplication(ctx context.Context, def *ApplicationDefinition) (*application.ApplicationDetail, error) {
	var cursor *v1.ApplicationID
	for {
		list, next, err := s.applicationAPI.List(ctx, listPageSize, cursor)
		if err != nil {
			return nil, fmt.Errorf("アプリケーションの一覧取得に失敗しました: %w", err)
		}
		for i := range list {
			if list[i].Name != def.Name {
				continue
			}
			detail, err := s.applicationAPI.Read(ctx, list[i].ApplicationID)
			if err != nil {
				return nil, fmt.Errorf("アプリケーション %q の情報取得に失敗しました: %w", def.Name, err)
			}
			if detail.ClusterName == def.Cluster {
				return detail, nil
			}
		}
		if next == nil {
			return nil, &ApplicationNotFoundError{Cluster: def.Cluster, Name: def.Name}
		}
		cursor = next
	}
}

// findOrCreateApplication はアプリケーションを検索し、見つからない場合は作成します。
// 2 番目の戻り値は作成を行ったかどうかです。
func (s *Service) findOrCreateApplication(ctx context.Context, def *ApplicationDefinition, clusterID v1.ClusterID) (*application.ApplicationDetail, bool, error) {
	detail, err := s.findApplication(ctx, def)
	if err == nil {
		return detail, false, nil
	}
	var notFound *ApplicationNotFoundError
	if !errors.As(err, &notFound) {
		return nil, false, err
	}
	if err := ensureActive(ctx); err != nil {
		return nil, false, err
	}
	ident, err := s.applicationAPI.Create(ctx, def.Name, clusterID)
	if err != nil {
		return nil, false, fmt.Errorf("アプリケーション %q の作成に失敗しました: %w", def.Name, err)
	}
	detail, err = s.applicationAPI.Read(ctx, ident.ApplicationID)
	if err != nil {
		return nil, false, fmt.Errorf("作成したアプリケーション %q の情報取得に失敗しました: %w", def.Name, err)
	}
	return detail, true, nil
}

// listAllVersions はアプリケーションの全バージョンを取得します。
func (s *Service) listAllVersions(ctx context.Context, id v1.ApplicationID) ([]v1.ApplicationVersionSummary, error) {
	var (
		cursor *v1.ApplicationVersionNumber
		all    []v1.ApplicationVersionSummary
	)
	for {
		list, next, err := s.newVersionAPI(id).List(ctx, listPageSize, cursor)
		if err != nil {
			return nil, fmt.Errorf("バージョンの一覧取得に失敗しました: %w", err)
		}
		all = append(all, list...)
		if next == nil {
			return all, nil
		}
		cursor = next
	}
}

// ensureActive は context がキャンセルされていないことを確認します。
// 読み取りと変更の間にキャンセルされた場合は変更 API を呼び出しません。
func ensureActive(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("操作がキャンセルされたため API を呼び出しません: %w", err)
	}
	return nil
}

// toInt32Ptr はバージョン番号を Application.Update の引数型へ変換します。
func toInt32Ptr(v v1.ApplicationVersionNumber) *int32 {
	n := int32(v)
	return &n
}
