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

package apprunapi

type Commands struct {
	// 生成・手書きコマンド集合です。operationId 派生名への移行 (ADR 0074) に合わせ、
	// application / version / packet-filter / traffic / user のグループ階層を
	// Kong の embed で展開して 1 階層のフラット構造にしています。
	Application  ApplicationCommands  `embed:""`
	Version      VersionCommands      `embed:""`
	PacketFilter PacketFilterCommands `embed:""`
	Traffic      TrafficCommands      `embed:""`
	User         UserCommands         `embed:""`
}
