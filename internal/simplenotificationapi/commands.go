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

package simplenotificationapi

import (
	"context"
	"fmt"

	"github.com/alecthomas/kong"
	simplenotification "github.com/sacloud/sacloud-sdk-go/api/simple-notification"
	v1 "github.com/sacloud/sacloud-sdk-go/api/simple-notification/apis/v1"
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

// Commands は Simple Notification API のコマンド群です。CommonServiceItem 操作は
// Kong の embed で展開して、1 階層のフラットなコマンド構造にしています。
type Commands struct {
	CommonServiceItem CommonServiceItemCommands `embed:""`
	History           HistoryCommands           `embed:""`
}

// NewCommonServiceItemAPI は --provider-class で指定された種別に対応する API 操作を返します。
// 種別ごとの SDK 操作は、一覧操作の Provider.Class 絞り込みと、作成・更新時の
// リクエストへの Provider.Class 設定を行います。読み取りと削除は ID で直接操作するため
// 種別に依存しません。
func NewCommonServiceItemAPI(trace bool, providerClass string) (CommonServiceItemAPI, error) {
	class, err := parseProviderClass(providerClass)
	if err != nil {
		return nil, err
	}
	client, err := NewClient(trace)
	if err != nil {
		return nil, err
	}
	return selectCommonServiceItemOp(class, client), nil
}

// NewHistoryAPI は通知履歴の API 操作を構築します。
func NewHistoryAPI(trace bool) (HistoryAPI, error) {
	client, err := NewClient(trace)
	if err != nil {
		return nil, err
	}
	return simplenotification.NewHistoryOp(client), nil
}

// parseProviderClass は --provider-class の値を ProviderClass に変換し、
// 未対応の値をエラーにします。
func parseProviderClass(value string) (v1.CommonServiceItemProviderClass, error) {
	switch v1.CommonServiceItemProviderClass(value) {
	case v1.CommonServiceItemProviderClassSaknoticedestination,
		v1.CommonServiceItemProviderClassSaknoticegroup,
		v1.CommonServiceItemProviderClassSaknoticerouting:
		return v1.CommonServiceItemProviderClass(value), nil
	default:
		return "", fmt.Errorf("--provider-class には saknoticedestination、saknoticegroup、saknoticerouting のいずれかを指定してください")
	}
}

// selectCommonServiceItemOp は種別に対応する SDK 操作を選択します。通知先は
// メールアドレスや Webhook URL が含まれ得るエラーを保護する adapter で包みます。
func selectCommonServiceItemOp(class v1.CommonServiceItemProviderClass, client *v1.Client) CommonServiceItemAPI {
	switch class {
	case v1.CommonServiceItemProviderClassSaknoticedestination:
		return destinationAdapter{simplenotification.NewDestinationOp(client)}
	case v1.CommonServiceItemProviderClassSaknoticegroup:
		return simplenotification.NewGroupOp(client)
	default:
		return simplenotification.NewRoutingOp(client)
	}
}

// commonServiceItemStatusReader は通知先のステータス確認をサポートする API 操作が
// 実装するメソッドです。ステータス確認は通知先のみに存在するため、共通の API
// interface には含まれません。
type commonServiceItemStatusReader interface {
	ReadStatus(context.Context, string) (*v1.CheckDestinationStatusResponse, error)
}

// notificationMessageSender はメッセージ送信をサポートする API 操作が実装するメソッドです。
// メッセージ送信は通知先グループのみに存在するため、共通の API interface には含まれません。
type notificationMessageSender interface {
	SendMessage(context.Context, string, v1.SendNotificationRequest) (*v1.SendNotificationResponse, error)
}

// routingReorderer はルーティング順序の変更をサポートする API 操作が実装するメソッドです。
// 順序変更はルーティング設定のみに存在するため、共通の API interface には含まれません。
type routingReorderer interface {
	Reorder(context.Context, v1.ReorderNotificationRoutingRequest) (*v1.ReorderNotificationRoutingAccepted, error)
}

// notificationSourceLister は通知元の一覧取得をサポートする API 操作が実装するメソッドです。
// 通知元の一覧はルーティング設定のみに存在するため、共通の API interface には含まれません。
type notificationSourceLister interface {
	ListSource(context.Context) (*v1.ListNotificationSourcesResponse, error)
}

// CommonServiceItemReadStatusCommand は通知先の送信可否ステータスを取得します。
// ステータス確認は通知先専用の操作のため --provider-class を付けず、
// 種別を saknoticedestination に固定します。
type CommonServiceItemReadStatusCommand struct {
	ID      string `arg:"" help:"ステータスを確認する通知先 ID。"`
	factory CommonServiceItemAPIFactory
	runtime CommonServiceItemRuntime
}

func (c *CommonServiceItemReadStatusCommand) Run(ctx *kong.Context) error {
	if c.runtime.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	if err := c.runtime.ValidateOutput(ctx); err != nil {
		return err
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory(string(v1.CommonServiceItemProviderClassSaknoticedestination))
	if err != nil {
		return err
	}
	statusOp, ok := op.(commonServiceItemStatusReader)
	if !ok {
		return fmt.Errorf("ステータスの確認は通知先 (saknoticedestination) に対してのみ実行できます")
	}
	result, err := statusOp.ReadStatus(context.Background(), c.ID)
	if err != nil {
		return err
	}
	if c.runtime.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return c.runtime.WriteOutput(ctx, result)
}

// CommonServiceItemSendMessageCommand は通知先グループにメッセージを送信します。
// メッセージ送信は通知先グループ専用の操作のため --provider-class を付けず、
// 種別を saknoticegroup に固定します。
type CommonServiceItemSendMessageCommand struct {
	ID      string  `arg:"" help:"送信先の通知先グループ ID。通知先 ID ではありません。"`
	Request *string `help:"Message を含む JSON を直接または @path.json で指定するか、--message を指定します。--request と個別フラグは併用できません。例: send-notification GROUP-ID --message 'テスト通知'"`
	Message *string `name:"message" help:"必須: 通知するメッセージ。API の Message フィールドに対応します。"`
	factory CommonServiceItemAPIFactory
	runtime CommonServiceItemRuntime
}

func (c *CommonServiceItemSendMessageCommand) Run(ctx *kong.Context) error {
	flagsSet := false
	flagsSet = flagsSet || c.Message != nil
	if c.Request != nil && flagsSet {
		return fmt.Errorf("--request と個別フラグは併用できません")
	}
	request := &v1.SendNotificationRequest{}
	if c.Request != nil {
		if c.runtime.DecodeRequest == nil {
			return fmt.Errorf("API リクエスト処理が設定されていません")
		}
		if err := c.runtime.DecodeRequest(*c.Request, request); err != nil {
			return err
		}
	} else {
		if !flagsSet {
			return fmt.Errorf("--request または個別フラグが必要です")
		}
		if c.Message == nil {
			return fmt.Errorf("--message が必要です")
		}
		request.Message = *c.Message
	}
	if c.runtime.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	if err := c.runtime.ValidateOutput(ctx); err != nil {
		return err
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory(string(v1.CommonServiceItemProviderClassSaknoticegroup))
	if err != nil {
		return err
	}
	sendOp, ok := op.(notificationMessageSender)
	if !ok {
		return fmt.Errorf("メッセージの送信は通知先グループ (saknoticegroup) に対してのみ実行できます")
	}
	result, err := sendOp.SendMessage(context.Background(), c.ID, *request)
	if err != nil {
		return err
	}
	if c.runtime.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return c.runtime.WriteOutput(ctx, result)
}

// CommonServiceItemReorderCommand はルーティング設定の順序を変更します。
// 順序変更はルーティング設定専用の操作のため --provider-class を付けず、
// 種別を saknoticerouting に固定します。
type CommonServiceItemReorderCommand struct {
	Request *string `help:"ルーティング順序の変更に使う JSON を直接または @path.json で指定します。"`
	factory CommonServiceItemAPIFactory
	runtime CommonServiceItemRuntime
}

func (c *CommonServiceItemReorderCommand) Run(ctx *kong.Context) error {
	if c.Request == nil {
		return fmt.Errorf("--request が必要です")
	}
	if c.runtime.DecodeRequest == nil {
		return fmt.Errorf("API リクエスト処理が設定されていません")
	}
	request := &v1.ReorderNotificationRoutingRequest{}
	if err := c.runtime.DecodeRequest(*c.Request, request); err != nil {
		return err
	}
	if c.runtime.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	if err := c.runtime.ValidateOutput(ctx); err != nil {
		return err
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory(string(v1.CommonServiceItemProviderClassSaknoticerouting))
	if err != nil {
		return err
	}
	reorderOp, ok := op.(routingReorderer)
	if !ok {
		return fmt.Errorf("順序の変更はルーティング設定 (saknoticerouting) に対してのみ実行できます")
	}
	result, err := reorderOp.Reorder(context.Background(), *request)
	if err != nil {
		return err
	}
	if c.runtime.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return c.runtime.WriteOutput(ctx, result)
}

// CommonServiceItemListSourceCommand はルーティング設定に利用できる通知元を一覧表示します。
// 通知元の一覧はルーティング設定専用の操作のため --provider-class を付けず、
// 種別を saknoticerouting に固定します。
type CommonServiceItemListSourceCommand struct {
	factory CommonServiceItemAPIFactory
	runtime CommonServiceItemRuntime
}

func (c *CommonServiceItemListSourceCommand) Run(ctx *kong.Context) error {
	if c.runtime.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	if err := c.runtime.ValidateOutput(ctx); err != nil {
		return err
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory(string(v1.CommonServiceItemProviderClassSaknoticerouting))
	if err != nil {
		return err
	}
	listOp, ok := op.(notificationSourceLister)
	if !ok {
		return fmt.Errorf("通知元の一覧はルーティング設定 (saknoticerouting) に対してのみ実行できます")
	}
	result, err := listOp.ListSource(context.Background())
	if err != nil {
		return err
	}
	if c.runtime.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return c.runtime.WriteOutput(ctx, result)
}

func NewClient(trace bool) (*v1.Client, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	return simplenotification.NewClient(client)
}
