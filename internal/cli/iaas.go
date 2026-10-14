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
	diskplanSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/diskplan"
	serverSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
	zoneSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/zone"
	archiveresource "github.com/sacloud/skr/internal/iaas/archive"
	archiveapi "github.com/sacloud/skr/internal/iaas/archiveapi"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	diskresource "github.com/sacloud/skr/internal/iaas/disk"
	diskapi "github.com/sacloud/skr/internal/iaas/diskapi"
	diskplanresource "github.com/sacloud/skr/internal/iaas/diskplan"
	diskplanapi "github.com/sacloud/skr/internal/iaas/diskplanapi"
	serverresource "github.com/sacloud/skr/internal/iaas/server"
	serverapi "github.com/sacloud/skr/internal/iaas/serverapi"
	switchresource "github.com/sacloud/skr/internal/iaas/switch"
	switchapi "github.com/sacloud/skr/internal/iaas/switchapi"
	zoneapi "github.com/sacloud/skr/internal/iaas/zoneapi"
)

type iaasAPICommand struct {
	Archive  archiveapi.Commands  `cmd:"" help:"さくらのクラウドのアーカイブを検索・参照します。検索では対象ゾーン、タグまたは OS 種別を指定できます。"`
	Disk     diskapi.Commands     `cmd:"" help:"さくらのクラウドのディスクを検索、参照、作成、更新、削除します。作成と更新は --request JSON を使用します。"`
	DiskPlan diskplanapi.Commands `cmd:"" help:"指定したゾーンのディスクプランを検索・参照します。プランの利用可否とサイズを確認できます。"`
	Switch   switchapi.Commands   `cmd:"" help:"さくらのクラウドのスイッチを操作します。各操作で --zone などのフラグ、または --request JSON に Zone を指定します。両経路は併用できません。"`
	Server   serverapi.Commands   `cmd:"" help:"さくらのクラウドのサーバを検索、参照、作成、更新、起動、削除します。作成時はcloud-init用ディスクやNICを個別フラグで指定できます。"`
	Zone     zoneapi.Commands     `cmd:"" help:"さくらのクラウドのゾーン一覧を取得します。"`
}

func (c *cli) initIaaSAPI() {
	c.IaaSAPI.Archive.SetRuntime(archiveapi.Runtime{
		DecodeRequest:   decodeRequest,
		ValidateOutput:  validateOutput,
		WriteOutput:     writeOutput,
		ValidateRequest: archiveresource.ValidateRequest,
	})
	c.IaaSAPI.Archive.SetFactory(func() (archiveapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return archiveSDK.New(client), nil
	})

	c.IaaSAPI.Zone.SetRuntime(zoneapi.Runtime{
		DecodeRequest:  decodeRequest,
		ValidateOutput: validateOutput,
		WriteOutput:    writeOutput,
	})
	c.IaaSAPI.Zone.SetFactory(func() (zoneapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return zoneSDK.New(client), nil
	})

	c.IaaSAPI.Disk.SetRuntime(diskapi.Runtime{
		DecodeRequest:   decodeRequest,
		ValidateOutput:  validateOutput,
		WriteOutput:     writeOutput,
		ValidateRequest: diskresource.ValidateRequest,
	})
	c.IaaSAPI.Disk.SetFactory(func() (diskapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return diskSDK.New(client), nil
	})
	c.IaaSAPI.DiskPlan.SetRuntime(diskplanapi.Runtime{
		DecodeRequest:   decodeRequest,
		ValidateOutput:  validateOutput,
		WriteOutput:     writeOutput,
		ValidateRequest: diskplanresource.ValidateRequest,
	})
	c.IaaSAPI.DiskPlan.SetFactory(func() (diskplanapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return diskplanSDK.New(client), nil
	})
	c.IaaSAPI.Switch.SetRuntime(switchapi.Runtime{
		DecodeRequest:   decodeRequest,
		ValidateOutput:  validateOutput,
		WriteOutput:     writeOutput,
		ValidateRequest: switchresource.ValidateRequest,
	})
	c.IaaSAPI.Switch.SetFactory(func() (switchapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return swytch.New(client), nil
	})
	c.IaaSAPI.Server.SetRuntime(serverapi.Runtime{
		DecodeRequest:          decodeRequest,
		ValidateOutput:         validateOutput,
		WriteOutput:            writeOutput,
		ValidateRequest:        serverresource.ValidateRequest,
		TransformCreateRequest: serverapi.TransformCreateRequest,
	})
	c.IaaSAPI.Server.SetFactory(func() (serverapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return serverSDK.New(client), nil
	})
}
