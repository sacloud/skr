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

package cli

import (
	eventbusv1 "github.com/sacloud/sacloud-sdk-go/api/eventbus/apis/v1"
)

// apiCommandNameBaseline は operationId 機械派生への移行が完了していないコマンドの
// 現行パスと、対応する SDK の operation 定数を登録します。
// 値は SDK の ogen 生成コードが公開する operation 定数で、operationId を正規化した
// operation 名を値として持ちます。
// ドメインの移行を完了したら、該当ドメインのエントリを削除します。
// 詳細は docs/design/api-command-operation-id-naming.md を参照してください。
var apiCommandNameBaseline = map[string]string{
	// apprun-dedicated-api
	// eventbus-api
	"eventbus-api create-common-service-item":       eventbusv1.CreateCommonServiceItemOperation,
	"eventbus-api delete-common-service-item":       eventbusv1.DeleteCommonServiceItemOperation,
	"eventbus-api list-common-service-items":        eventbusv1.ListCommonServiceItemsOperation,
	"eventbus-api read-common-service-item":         eventbusv1.ReadCommonServiceItemOperation,
	"eventbus-api set-process-configuration-secret": eventbusv1.SetProcessConfigurationSecretOperation,
	"eventbus-api update-common-service-item":       eventbusv1.UpdateCommonServiceItemOperation,
}
