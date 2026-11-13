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
	"strings"
	"time"

	autoscalinggroup "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/autoscalinggroup"
	cluster "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/cluster"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
)

// CreateAutoScalingGroupOptions は CreateAutoScalingGroup の待機に関するオプションです。
type CreateAutoScalingGroupOptions struct {
	// NoWait が true の場合、作成後のワーカノード healthy 待ちを行いません。
	NoWait bool
	// Timeout は healthy 待ちのタイムアウトです。零値の場合は DefaultWaitTimeout を使います。
	Timeout time.Duration
}

// CreateAutoScalingGroupResult は CreateAutoScalingGroup の結果です。
type CreateAutoScalingGroupResult struct {
	// Cluster はクラスタの名前です。
	Cluster string `json:"cluster"`
	// Name はオートスケーリンググループの名前です。
	Name string `json:"name"`
	// AutoScalingGroupID はオートスケーリンググループの ID です。
	AutoScalingGroupID v1.AutoScalingGroupID `json:"autoScalingGroupID"`
	// Zone はゾーンです。
	Zone string `json:"zone"`
	// WorkerServiceClassPath はワーカサービスクラスのパスです。
	WorkerServiceClassPath string `json:"workerServiceClassPath"`
	// NameServers はネームサーバーの IPv4 アドレスです。
	NameServers []v1.Ipv4 `json:"nameServers"`
	// MinNodes は最小ノード数です。
	MinNodes int32 `json:"minNodes"`
	// MaxNodes は最大ノード数です。
	MaxNodes int32 `json:"maxNodes"`
}

// CreateAutoScalingGroup はクラスタにオートスケーリンググループを作成し、
// ワーカノードが healthy になるまで待ちます。
//
// クラスタ名からクラスタを検索し、同名のグループが既に存在しないことを確認してから
// 作成します。NoWait が true の場合、作成 API 呼び出し後の healthy 待ちを省略します。
func (s *Service) CreateAutoScalingGroup(ctx context.Context, clusterName, groupName, zone, workerServiceClassPath string, nameServers []v1.Ipv4, minNodes, maxNodes int32, opts *CreateAutoScalingGroupOptions) (*CreateAutoScalingGroupResult, error) {
	if opts == nil {
		opts = &CreateAutoScalingGroupOptions{}
	}
	clusterDetail, err := s.findCluster(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	asgAPI := s.newAutoScalingGroupAPI(clusterDetail.ClusterID)
	if _, err := s.findAutoScalingGroup(ctx, asgAPI, clusterName, groupName); err == nil {
		return nil, fmt.Errorf("クラスタ %q にオートスケーリンググループ %q は既に存在します", clusterName, groupName)
	} else {
		var notFound *AutoScalingGroupNotFoundError
		if !errors.As(err, &notFound) {
			return nil, err
		}
	}
	if err := ensureActive(ctx); err != nil {
		return nil, err
	}
	ident, err := asgAPI.Create(ctx, autoscalinggroup.CreateParams{
		Name:                   groupName,
		Zone:                   zone,
		NameServers:            nameServers,
		WorkerServiceClassPath: workerServiceClassPath,
		MinNodes:               minNodes,
		MaxNodes:               maxNodes,
		Interfaces: []autoscalinggroup.NodeInterface{{
			InterfaceIndex: 0,
			Upstream:       "shared",
			ConnectsToLB:   false,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("オートスケーリンググループ %q の作成に失敗しました: %w", groupName, err)
	}

	detail, err := asgAPI.Read(ctx, ident.AutoScalingGroupID)
	if err != nil {
		return nil, fmt.Errorf("作成したオートスケーリンググループ %q の情報取得に失敗しました: %w", groupName, err)
	}
	result := &CreateAutoScalingGroupResult{
		Cluster:                clusterName,
		Name:                   detail.Name,
		AutoScalingGroupID:     detail.AutoScalingGroupID,
		Zone:                   detail.Zone,
		WorkerServiceClassPath: detail.WorkerServiceClassPath,
		NameServers:            detail.NameServers,
		MinNodes:               detail.MinNodes,
		MaxNodes:               detail.MaxNodes,
	}
	if !opts.NoWait {
		if err := s.waitWorkerNodesHealthy(ctx, clusterDetail.ClusterID, detail.AutoScalingGroupID, opts.Timeout); err != nil {
			return nil, fmt.Errorf("ワーカノードの healthy 待ちに失敗しました (オートスケーリンググループ %q の作成は完了しています): %w", groupName, err)
		}
	}
	return result, nil
}

// DeleteAutoScalingGroupOptions は DeleteAutoScalingGroup の待機に関するオプションです。
type DeleteAutoScalingGroupOptions struct {
	// NoWait は削除 API 呼び出し後の待機の扱いです。削除は API 側で非同期に完了するため、
	// 削除後の不在確認 (対象が見つからなくなるまでのポーリング) が削除完了の確認を兼ね、
	// NoWait にかかわらず常に行います。
	NoWait bool
	// Timeout は削除後の不在確認のタイムアウトです。零値の場合は DefaultWaitTimeout を使います。
	Timeout time.Duration
}

// DeleteAutoScalingGroupResult は DeleteAutoScalingGroup の結果です。
type DeleteAutoScalingGroupResult struct {
	// Cluster はクラスタの名前です。
	Cluster string `json:"cluster"`
	// Name は削除したオートスケーリンググループの名前です。
	Name string `json:"name"`
	// AutoScalingGroupID は削除したオートスケーリンググループの ID です。
	AutoScalingGroupID v1.AutoScalingGroupID `json:"autoScalingGroupID"`
}

// DeleteAutoScalingGroup はクラスタのオートスケーリンググループを削除します。
//
// クラスタ名とグループ名で対象を特定し、削除 API を呼び出した後、削除後の不在を
// 確認します。削除後の不在確認は常に行います。
func (s *Service) DeleteAutoScalingGroup(ctx context.Context, clusterName, groupName string, opts *DeleteAutoScalingGroupOptions) (*DeleteAutoScalingGroupResult, error) {
	if opts == nil {
		opts = &DeleteAutoScalingGroupOptions{}
	}
	clusterDetail, err := s.findCluster(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	asgAPI := s.newAutoScalingGroupAPI(clusterDetail.ClusterID)
	asgDetail, err := s.findAutoScalingGroup(ctx, asgAPI, clusterName, groupName)
	if err != nil {
		return nil, err
	}
	result := &DeleteAutoScalingGroupResult{
		Cluster:            clusterName,
		Name:               asgDetail.Name,
		AutoScalingGroupID: asgDetail.AutoScalingGroupID,
	}
	if err := ensureActive(ctx); err != nil {
		return nil, err
	}
	if err := asgAPI.Delete(ctx, asgDetail.AutoScalingGroupID); err != nil {
		if !saclient.IsNotFoundError(err) {
			return nil, fmt.Errorf("オートスケーリンググループ %q の削除に失敗しました: %w", groupName, err)
		}
		return result, nil
	}
	if err := s.confirmAutoScalingGroupDeleted(ctx, asgAPI, asgDetail.AutoScalingGroupID, opts.Timeout); err != nil {
		return nil, fmt.Errorf("オートスケーリンググループ %q の削除後の不在確認に失敗しました: %w", groupName, err)
	}
	return result, nil
}

// DeleteClusterOptions は DeleteCluster の待機に関するオプションです。
type DeleteClusterOptions struct {
	// NoWait は削除 API 呼び出し後の待機の扱いです。依存確認、削除 API の呼び出し、
	// 削除後の不在確認 (対象が見つからなくなるまでのポーリング) は NoWait にかかわらず
	// 常に行います。削除は API 側で非同期に完了するため、不在確認が削除完了の確認を兼ねます。
	NoWait bool
	// Timeout は削除後の不在確認のタイムアウトです。零値の場合は DefaultWaitTimeout を使います。
	Timeout time.Duration
}

// DeleteClusterResult は DeleteCluster の結果です。
type DeleteClusterResult struct {
	// Name は削除したクラスタの名前です。
	Name string `json:"name"`
	// ClusterID は削除したクラスタの ID です。
	ClusterID v1.ClusterID `json:"clusterID"`
}

// DeleteCluster はクラスタを削除します。
//
// クラスタ名で対象を特定し、アプリケーションやオートスケーリンググループなどの
// 関連リソースが残っていれば列挙して中断します。関連リソースがなければ削除 API を
// 呼び出し、削除後の不在を確認します。削除後の不在確認は常に行います。
func (s *Service) DeleteCluster(ctx context.Context, clusterName string, opts *DeleteClusterOptions) (*DeleteClusterResult, error) {
	if opts == nil {
		opts = &DeleteClusterOptions{}
	}
	clusterDetail, err := s.findCluster(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	deps, err := s.listClusterDependencies(ctx, clusterDetail)
	if err != nil {
		return nil, err
	}
	if len(deps) > 0 {
		return nil, fmt.Errorf("クラスタ %q に削除対象外のリソースが残っています: %s", clusterName, formatClusterDependencies(deps))
	}
	if err := ensureActive(ctx); err != nil {
		return nil, err
	}
	result := &DeleteClusterResult{
		Name:      clusterName,
		ClusterID: clusterDetail.ClusterID,
	}
	if err := s.clusterAPI.Delete(ctx, clusterDetail.ClusterID); err != nil {
		if !saclient.IsNotFoundError(err) {
			return nil, fmt.Errorf("クラスタ %q の削除に失敗しました: %w", clusterName, err)
		}
		return result, nil
	}
	if err := s.confirmClusterDeleted(ctx, clusterDetail.ClusterID, opts.Timeout); err != nil {
		return nil, fmt.Errorf("クラスタ %q の削除後の不在確認に失敗しました: %w", clusterName, err)
	}
	return result, nil
}

// AutoScalingGroupNotFoundError は、指定されたクラスタに同名のオートスケーリンググループが
// 存在しないことを表します。errors.As での判定に使えます。
type AutoScalingGroupNotFoundError struct {
	// Cluster は検索したクラスタの名前です。
	Cluster string
	// Name は検索したオートスケーリンググループの名前です。
	Name string
}

func (e *AutoScalingGroupNotFoundError) Error() string {
	return fmt.Sprintf("クラスタ %q にオートスケーリンググループ %q が見つかりません", e.Cluster, e.Name)
}

// findAutoScalingGroup はクラスタ内のオートスケーリンググループを名前で検索します。
func (s *Service) findAutoScalingGroup(ctx context.Context, api autoscalinggroup.AutoScalingGroupAPI, clusterName, name string) (*autoscalinggroup.AutoScalingGroupDetail, error) {
	var cursor *v1.AutoScalingGroupID
	for {
		list, next, err := api.List(ctx, listPageSize, cursor)
		if err != nil {
			return nil, fmt.Errorf("オートスケーリンググループの一覧取得に失敗しました: %w", err)
		}
		for i := range list {
			if list[i].Name != name {
				continue
			}
			detail, err := api.Read(ctx, list[i].AutoScalingGroupID)
			if err != nil {
				return nil, fmt.Errorf("オートスケーリンググループ %q の情報取得に失敗しました: %w", name, err)
			}
			return detail, nil
		}
		if next == nil {
			return nil, &AutoScalingGroupNotFoundError{Cluster: clusterName, Name: name}
		}
		cursor = next
	}
}

// waitWorkerNodesHealthy は指定されたオートスケーリンググループの全ワーカノードが
// healthy になるまで待機します。完了条件は API のワーカノード一覧応答の Status で、
// すべてのノードが healthy 状態のときに完了とします。全ページを走査します。
//
// 一覧応答には healthy フィールドが含まれず (SDK の WorkerNode.List は Summary を
// Detail に変換して Healthy を常にゼロ値にする)、実 API でも healthy は status と
// 乖離して返るため、Healthy ではなく Status を完了条件に使います。
func (s *Service) waitWorkerNodesHealthy(ctx context.Context, clusterID v1.ClusterID, asgID v1.AutoScalingGroupID, timeout time.Duration) error {
	wnAPI := s.newWorkerNodeAPI(clusterID, asgID)
	return s.poll(ctx, timeout, func(ctx context.Context) (bool, error) {
		var cursor *v1.WorkerNodeID
		found := false
		for {
			nodes, next, err := wnAPI.List(ctx, listPageSize, cursor)
			if err != nil {
				return false, nil
			}
			if len(nodes) > 0 {
				found = true
			}
			for _, node := range nodes {
				if node.Status != v1.WorkerNodeStatusHealthy {
					return false, nil
				}
			}
			if next == nil {
				break
			}
			cursor = next
		}
		return found, nil
	})
}

// confirmAutoScalingGroupDeleted は対象の不在を確認します。
func (s *Service) confirmAutoScalingGroupDeleted(ctx context.Context, api autoscalinggroup.AutoScalingGroupAPI, id v1.AutoScalingGroupID, timeout time.Duration) error {
	return s.poll(ctx, timeout, func(ctx context.Context) (bool, error) {
		_, err := api.Read(ctx, id)
		if err != nil {
			if saclient.IsNotFoundError(err) {
				return true, nil
			}
			return false, nil
		}
		return false, nil
	})
}

// confirmClusterDeleted は対象クラスタの不在を確認します。
func (s *Service) confirmClusterDeleted(ctx context.Context, id v1.ClusterID, timeout time.Duration) error {
	return s.poll(ctx, timeout, func(ctx context.Context) (bool, error) {
		_, err := s.clusterAPI.Read(ctx, id)
		if err != nil {
			if saclient.IsNotFoundError(err) {
				return true, nil
			}
			return false, nil
		}
		return false, nil
	})
}

// clusterDependency はクラスタに残っている依存リソースを表します。
type clusterDependency struct {
	kind string
	name string
}

// listClusterDependencies はクラスタに残っている削除対象外のリソースを列挙します。
func (s *Service) listClusterDependencies(ctx context.Context, clusterDetail *cluster.ClusterDetail) ([]clusterDependency, error) {
	var deps []clusterDependency
	var appCursor *v1.ApplicationID
	for {
		apps, next, err := s.applicationAPI.List(ctx, listPageSize, appCursor)
		if err != nil {
			return nil, fmt.Errorf("アプリケーションの一覧取得に失敗しました: %w", err)
		}
		for i := range apps {
			if apps[i].ClusterID != clusterDetail.ClusterID {
				continue
			}
			deps = append(deps, clusterDependency{kind: "application", name: apps[i].Name})
		}
		if next == nil {
			break
		}
		appCursor = next
	}
	asgAPI := s.newAutoScalingGroupAPI(clusterDetail.ClusterID)
	var asgCursor *v1.AutoScalingGroupID
	for {
		asgs, next, err := asgAPI.List(ctx, listPageSize, asgCursor)
		if err != nil {
			return nil, fmt.Errorf("オートスケーリンググループの一覧取得に失敗しました: %w", err)
		}
		for i := range asgs {
			deps = append(deps, clusterDependency{kind: "auto-scaling-group", name: asgs[i].Name})
		}
		if next == nil {
			break
		}
		asgCursor = next
	}
	return deps, nil
}

// formatClusterDependencies は依存リソースを人間が読める文字列に整形します。
func formatClusterDependencies(deps []clusterDependency) string {
	var parts []string
	for _, d := range deps {
		parts = append(parts, fmt.Sprintf("%s %q", d.kind, d.name))
	}
	return strings.Join(parts, ", ")
}
