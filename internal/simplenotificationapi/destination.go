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

	v1 "github.com/sacloud/sacloud-sdk-go/api/simple-notification/apis/v1"
)

type destinationError struct {
	message string
	cause   error
}

func (e *destinationError) Error() string { return e.message }
func (e *destinationError) Unwrap() error { return e.cause }

func protectDestinationError(message string, err error) error {
	if err == nil {
		return nil
	}
	return &destinationError{message: message, cause: err}
}

type destinationAdapter struct {
	DestinationAPI
}

func (a destinationAdapter) List(ctx context.Context) (*v1.ListCommonServiceItemsResponse, error) {
	result, err := a.DestinationAPI.List(ctx)
	return result, protectDestinationError("通知先一覧の取得に失敗しました", err)
}

func (a destinationAdapter) Read(ctx context.Context, id string) (*v1.GetCommonServiceItemOK, error) {
	result, err := a.DestinationAPI.Read(ctx, id)
	return result, protectDestinationError("通知先の取得に失敗しました", err)
}

func (a destinationAdapter) Create(ctx context.Context, request v1.PostCommonServiceItemRequest) (*v1.CreateCommonServiceItemCreated, error) {
	result, err := a.DestinationAPI.Create(ctx, request)
	return result, protectDestinationError("通知先の作成に失敗しました。リクエストの設定と API の権限を確認してください", err)
}

func (a destinationAdapter) Update(ctx context.Context, id string, request v1.PutCommonServiceItemRequest) (*v1.UpdateCommonServiceItemOK, error) {
	result, err := a.DestinationAPI.Update(ctx, id, request)
	return result, protectDestinationError("通知先の更新に失敗しました。リクエストの設定と API の権限を確認してください", err)
}

func (a destinationAdapter) Delete(ctx context.Context, id string) error {
	return protectDestinationError("通知先の削除に失敗しました", a.DestinationAPI.Delete(ctx, id))
}

func (a destinationAdapter) ReadStatus(ctx context.Context, id string) (*v1.GetCommonServiceItemStatusResponse, error) {
	result, err := a.DestinationAPI.ReadStatus(ctx, id)
	return result, protectDestinationError("通知先ステータスの取得に失敗しました", err)
}
