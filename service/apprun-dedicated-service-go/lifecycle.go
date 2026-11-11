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

	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	"github.com/sacloud/sacloud-sdk-go/common/saclient"
)

// WaitOptions は Deactivate と Delete の待機に関するオプションです。
type WaitOptions struct {
	// NoWait が true の場合、変更後の停止待ちを行いません。削除自体と削除後の
	// 不在確認は行います。
	NoWait bool
	// Timeout は停止待ちと削除のタイムアウトです。零値の場合は DefaultWaitTimeout
	// を使います。
	Timeout time.Duration
}

// DeactivateResult は deactivate の結果です。
type DeactivateResult struct {
	// Cluster はクラスタの名前です。
	Cluster string `json:"cluster"`
	// Application はアプリケーションの名前です。
	Application string `json:"application"`
	// ApplicationID はアプリケーションの ID です。
	ApplicationID v1.ApplicationID `json:"applicationID"`
	// DeactivatedVersion は解除したアクティブバージョンの番号です。
	// すでに非活性だった場合は変更しないため null になります。
	DeactivatedVersion *int32 `json:"deactivatedVersion"`
}

// DeleteResult は delete の結果です。
type DeleteResult struct {
	// Cluster はクラスタの名前です。
	Cluster string `json:"cluster"`
	// Application はアプリケーションの名前です。
	Application string `json:"application"`
	// ApplicationID は削除したアプリケーションの ID です。
	ApplicationID v1.ApplicationID `json:"applicationID"`
}

// Deactivate はアプリケーションのアクティブバージョンを解除して停止します。
//
// アプリケーションとバージョンは残ります。すでに非活性のときは変更せず、成功を返します
// (DeactivatedVersion が null になります)。変更の直前に定義の cluster、name と対象の
// ClusterName、Name が一致することを再照合し、一致しない場合は変更 API を呼び出しません。
// NoWait が true の場合、解除後の停止待ちを行いません。
func (s *Service) Deactivate(ctx context.Context, def *ApplicationDefinition, opts *WaitOptions) (*DeactivateResult, error) {
	if def == nil {
		return nil, errors.New("定義が nil です")
	}
	if err := def.Validate(); err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &WaitOptions{}
	}

	// 変更直前の再照合を兼ねます。
	detail, err := s.findApplication(ctx, def)
	if err != nil {
		return nil, err
	}
	result := &DeactivateResult{
		Cluster:            def.Cluster,
		Application:        def.Name,
		ApplicationID:      detail.ApplicationID,
		DeactivatedVersion: detail.ActiveVersion,
	}

	if detail.ActiveVersion != nil {
		if err := ensureActive(ctx); err != nil {
			return nil, err
		}
		if err := s.applicationAPI.Update(ctx, detail.ApplicationID, nil); err != nil {
			return nil, fmt.Errorf("アクティブバージョンの解除に失敗しました: %w", err)
		}
	}

	if !opts.NoWait {
		if err := s.waitStopped(ctx, detail.ApplicationID, opts.Timeout); err != nil {
			return nil, fmt.Errorf("アプリケーションの停止待ちに失敗しました (アクティブバージョンの解除は完了しています): %w", err)
		}
	}
	return result, nil
}

// Delete はアプリケーションとすべてのバージョンを削除します。
//
// アクティブバージョンがあれば先に解除し、停止を待ってから削除します。API が削除条件
// (アクティブバージョンなし、実行中コンテナなし) を満たすまで即座に失敗する場合は、
// 期限付きで再試行します。削除の成否は削除後の Read が見つからないこと (不在) で
// 確認します。変更の直前の再照合は Deactivate と同じです。
func (s *Service) Delete(ctx context.Context, def *ApplicationDefinition, opts *WaitOptions) (*DeleteResult, error) {
	if def == nil {
		return nil, errors.New("定義が nil です")
	}
	if err := def.Validate(); err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &WaitOptions{}
	}

	// 変更直前の再照合を兼ねます。
	detail, err := s.findApplication(ctx, def)
	if err != nil {
		return nil, err
	}
	result := &DeleteResult{
		Cluster:       def.Cluster,
		Application:   def.Name,
		ApplicationID: detail.ApplicationID,
	}

	if detail.ActiveVersion != nil {
		if err := ensureActive(ctx); err != nil {
			return nil, err
		}
		if err := s.applicationAPI.Update(ctx, detail.ApplicationID, nil); err != nil {
			return nil, fmt.Errorf("アクティブバージョンの解除に失敗しました: %w", err)
		}
	}

	if !opts.NoWait {
		if err := s.waitStopped(ctx, detail.ApplicationID, opts.Timeout); err != nil {
			return nil, fmt.Errorf("アプリケーションの停止待ちに失敗しました: %w", err)
		}
	}

	if err := s.deleteWithRetry(ctx, detail.ApplicationID, opts.Timeout); err != nil {
		return nil, fmt.Errorf("アプリケーションの削除に失敗しました: %w", err)
	}
	return result, nil
}

// waitStopped はアプリケーションの DesiredCount が 0 または null になるまで待機します。
func (s *Service) waitStopped(ctx context.Context, id v1.ApplicationID, timeout time.Duration) error {
	return s.poll(ctx, timeout, func(ctx context.Context) (bool, error) {
		detail, err := s.applicationAPI.Read(ctx, id)
		if err != nil {
			// 参照エラーは打ち切らず、期限まで再試行します。
			return false, nil
		}
		// DesiredCount はアクティブバージョンの解除後に null を返すことがあるため、
		// null も停止完了として扱います。
		return detail.DesiredCount == nil || *detail.DesiredCount == 0, nil
	})
}

// deleteWithRetry は削除を期限付きで再試行し、削除後の不在を確認します。
func (s *Service) deleteWithRetry(ctx context.Context, id v1.ApplicationID, timeout time.Duration) error {
	return s.poll(ctx, timeout, func(ctx context.Context) (bool, error) {
		if err := ensureActive(ctx); err != nil {
			return false, err
		}
		if err := s.applicationAPI.Delete(ctx, id); err != nil {
			// 削除 API は削除条件を満たすまで即座に失敗することがあるため、期限まで
			// 再試行します。ただし、削除が完了した後も参照が遅延で存在を返すと、次の
			// 削除は既に削除済みを示すエラーで失敗するため、削除の成否は不在確認の
			// 結果で判定します。
			return s.confirmDeleted(ctx, id)
		}
		// 削除 API が成功した場合も、参照が遅延で存在を返すことがあるため、不在を
		// 確認してから完了とします。
		return s.confirmDeleted(ctx, id)
	})
}

// confirmDeleted は対象の不在を確認します。NotFound なら削除済みとして true を返し、
// 存在する場合と参照エラーの場合は、期限まで再試行するため false を返します。
func (s *Service) confirmDeleted(ctx context.Context, id v1.ApplicationID) (bool, error) {
	if _, err := s.applicationAPI.Read(ctx, id); err != nil {
		if saclient.IsNotFoundError(err) {
			return true, nil
		}
		// 不在確認の参照エラーも期限まで再試行します。
		return false, nil
	}
	// まだ存在するため、期限まで再試行します。
	return false, nil
}
