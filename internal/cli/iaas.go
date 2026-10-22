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
	archiveSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/archive"
	diskSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	iconSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/icon"
	internetSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/internet"
	packetfilterSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/packetfilter"
	regionSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/region"
	serverSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	serviceclassSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/serviceclass"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
	zoneSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/zone"
	archiveapi "github.com/sacloud/skr/internal/iaas/archiveapi"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	diskapi "github.com/sacloud/skr/internal/iaas/diskapi"
	diskplanapi "github.com/sacloud/skr/internal/iaas/diskplanapi"
	iconapi "github.com/sacloud/skr/internal/iaas/iconapi"
	internetapi "github.com/sacloud/skr/internal/iaas/internetapi"
	internetplanapi "github.com/sacloud/skr/internal/iaas/internetplanapi"
	licenseinfoapi "github.com/sacloud/skr/internal/iaas/licenseinfoapi"
	packetfilterapi "github.com/sacloud/skr/internal/iaas/packetfilterapi"
	privatehostplanapi "github.com/sacloud/skr/internal/iaas/privatehostplanapi"
	regionapi "github.com/sacloud/skr/internal/iaas/regionapi"
	serverapi "github.com/sacloud/skr/internal/iaas/serverapi"
	serverplanapi "github.com/sacloud/skr/internal/iaas/serverplanapi"
	serviceclassapi "github.com/sacloud/skr/internal/iaas/serviceclassapi"
	switchapi "github.com/sacloud/skr/internal/iaas/switchapi"
	zoneapi "github.com/sacloud/skr/internal/iaas/zoneapi"
)

type iaasAPICommand struct {
	Archive      archiveapi.Commands      `cmd:"" help:"さくらのクラウドのアーカイブを検索・参照します。検索では対象ゾーン、タグまたは OS 種別を指定できます。"`
	Disk         diskapi.Commands         `cmd:"" help:"さくらのクラウドのディスクを検索、参照、作成、更新、削除します。作成と更新は --request JSON を使用します。"`
	Icon         iconapi.Commands         `cmd:"" help:"ユーザー作成アイコンを検索、参照、作成、更新、削除します。"`
	Internet     internetapi.Commands     `cmd:"" help:"さくらのクラウドのルータ＋スイッチと、関連する IP アドレスブロックおよび IPv6 設定を操作します。"`
	PacketFilter packetfilterapi.Commands `cmd:"" name:"packetfilter" help:"さくらのクラウドのパケットフィルタを検索、参照、作成、更新、削除します。ルール配列は --expressions JSON または --request JSON で指定します。"`
	Product      iaasProductCommand       `cmd:"" help:"商品 API のプラン情報を検索・参照します。"`
	Region       regionapi.Commands       `cmd:"" help:"リージョン情報を検索・参照します。"`
	Switch       switchapi.Commands       `cmd:"" help:"さくらのクラウドのスイッチを操作します。各操作で --zone などのフラグ、または --request JSON に Zone を指定します。両経路は併用できません。"`
	Server       serverapi.Commands       `cmd:"" help:"さくらのクラウドのサーバを検索、参照、作成、更新、起動、削除します。作成時はcloud-init用ディスクやNICを個別フラグで指定できます。"`
	ServiceClass serviceclassapi.Commands `cmd:"" help:"指定したゾーンのサービスクラスを検索します。"`
	Zone         zoneapi.Commands         `cmd:"" help:"さくらのクラウドのゾーン一覧を取得します。"`
}

type iaasProductCommand struct {
	Disk        diskplanapi.Commands        `cmd:"" help:"ディスクプランを検索・参照します。プランの利用可否とサイズを確認できます。"`
	Internet    internetplanapi.Commands    `cmd:"" help:"インターネットプランを検索・参照します。帯域幅と利用可否を確認できます。"`
	License     licenseinfoapi.Commands     `cmd:"" name:"license" help:"ライセンス情報を検索し、名称や利用条件を確認します。"`
	PrivateHost privatehostplanapi.Commands `cmd:"" name:"privatehost" help:"専有ホストプランを検索・参照します。CPU、メモリ、利用可否を確認できます。"`
	Server      serverplanapi.Commands      `cmd:"" help:"サーバプランを検索・参照します。CPU、メモリ、GPU、利用可否を確認できます。"`
}

func (c *cli) initIaaSAPI() {
	c.IaaSAPI.Archive = archiveapi.NewCommands(archiveapi.Runtime{
		DecodeRequest:  decodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	}, func() (archiveapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return archiveSDK.New(client), nil
	})

	c.IaaSAPI.Zone = zoneapi.NewCommands(zoneapi.Runtime{
		DecodeRequest:  decodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	}, func() (zoneapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return zoneSDK.New(client), nil
	})

	c.IaaSAPI.Disk = diskapi.NewCommands(diskapi.Runtime{
		DecodeRequest:  decodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	}, func() (diskapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return diskSDK.New(client), nil
	})
	c.IaaSAPI.Product.Disk = diskplanapi.NewCommands(diskplanapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, func() (diskplanapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return diskplanapi.NewSDKAPI(client), nil
	})
	c.IaaSAPI.Product.Internet = internetplanapi.NewCommands(internetplanapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, func() (internetplanapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return internetplanapi.NewSDKAPI(client), nil
	})
	c.IaaSAPI.Product.License = licenseinfoapi.NewCommands(licenseinfoapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, func() (licenseinfoapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return licenseinfoapi.NewSDKAPI(client), nil
	})
	c.IaaSAPI.Product.PrivateHost = privatehostplanapi.NewCommands(privatehostplanapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, func() (privatehostplanapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return privatehostplanapi.NewSDKAPI(client), nil
	})
	c.IaaSAPI.Region = regionapi.NewCommands(regionapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, func() (regionapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return regionSDK.New(client), nil
	})
	c.IaaSAPI.ServiceClass = serviceclassapi.NewCommands(serviceclassapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, func() (serviceclassapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return serviceclassSDK.New(client), nil
	})
	c.IaaSAPI.Icon = iconapi.NewCommands(iconapi.Runtime{
		DecodeRequest:  decodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	}, func() (iconapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return iconSDK.New(client), nil
	})
	c.IaaSAPI.Internet = internetapi.NewCommands(internetapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, func() (internetapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return internetSDK.New(client), nil
	})
	c.IaaSAPI.PacketFilter = packetfilterapi.NewCommands(packetfilterapi.Runtime{
		DecodeRequest:          decodeRequest,
		ValidateOutput:         validateOutput,
		WriteOutput:            writeOutput,
		TransformCreateRequest: transformPacketFilterCreateRequest,
		TransformUpdateRequest: transformPacketFilterUpdateRequest,
	}, func() (packetfilterapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return packetfilterSDK.New(client), nil
	})
	c.IaaSAPI.Switch = switchapi.NewCommands(switchapi.Runtime{
		DecodeRequest:  decodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	}, func() (switchapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return swytch.New(client), nil
	})
	c.IaaSAPI.Server = serverapi.NewCommands(serverapi.Runtime{
		DecodeRequest:          decodeRequest,
		ValidateOutput:         validateOutput,
		WriteOutput:            writeOutput,
		TransformCreateRequest: serverapi.TransformCreateRequest,
	}, func() (serverapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return serverSDK.New(client), nil
	})
	c.IaaSAPI.Product.Server = serverplanapi.NewCommands(serverplanapi.Runtime{
		DecodeRequest: decodeRequest, ValidateOutput: validateOutput, WriteOutput: writeOutput,
	}, func() (serverplanapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return serverplanapi.NewSDKAPI(client), nil
	})
}
