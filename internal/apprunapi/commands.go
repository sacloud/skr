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

import "github.com/alecthomas/kong"

type Commands struct {
	Application  ApplicationCommands  `cmd:"" help:"AppRun アプリケーションを管理します。"`
	Version      VersionCommands      `cmd:"" help:"アプリケーションのバージョンを確認・削除します。"`
	PacketFilter PacketFilterCommands `cmd:"" name:"packet-filter" help:"アプリケーションのパケットフィルターを確認・変更します。"`
	Traffic      TrafficCommands      `cmd:"" help:"アプリケーションのバージョン間でトラフィックを分散します。"`
	User         UserCommands         `cmd:"" help:"AppRun のユーザー情報を確認します。"`
}

type Runtime struct {
	DecodeRequest  func(string, any) error
	ValidateOutput func(*kong.Context) error
	WriteOutput    func(*kong.Context, any) error
}

type Factories struct {
	Application  ApplicationAPIFactory
	Version      VersionAPIFactory
	PacketFilter PacketFilterAPIFactory
	Traffic      TrafficAPIFactory
	User         UserAPIFactory
}

func NewCommands(runtime Runtime, factories Factories) Commands {
	return Commands{
		Application: NewApplicationCommands(ApplicationRuntime{
			DecodeRequest: runtime.DecodeRequest, ValidateOutput: runtime.ValidateOutput, WriteOutput: runtime.WriteOutput,
		}, factories.Application),
		Version: NewVersionCommands(VersionRuntime{
			DecodeRequest: runtime.DecodeRequest, ValidateOutput: runtime.ValidateOutput, WriteOutput: runtime.WriteOutput,
		}, factories.Version),
		PacketFilter: NewPacketFilterCommands(PacketFilterRuntime{
			DecodeRequest: runtime.DecodeRequest, ValidateOutput: runtime.ValidateOutput, WriteOutput: runtime.WriteOutput,
		}, factories.PacketFilter),
		Traffic: NewTrafficCommands(TrafficRuntime{
			DecodeRequest: runtime.DecodeRequest, ValidateOutput: runtime.ValidateOutput, WriteOutput: runtime.WriteOutput,
		}, factories.Traffic),
		User: NewUserCommands(UserRuntime{
			DecodeRequest: runtime.DecodeRequest, ValidateOutput: runtime.ValidateOutput, WriteOutput: runtime.WriteOutput,
		}, factories.User),
	}
}
