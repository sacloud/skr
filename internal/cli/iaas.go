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
	serverSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/swytch"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	serverresource "github.com/sacloud/skr/internal/iaas/server"
	serverapi "github.com/sacloud/skr/internal/iaas/serverapi"
	switchresource "github.com/sacloud/skr/internal/iaas/switch"
	switchapi "github.com/sacloud/skr/internal/iaas/switchapi"
	iaaszones "github.com/sacloud/skr/internal/iaas/zones"
)

type iaasAPICommand struct {
	Switch switchapi.Commands `cmd:"" help:"さくらのクラウドのスイッチを操作します。各操作で --zone などのフラグ、または --request JSON に Zone を指定します。find は --zone all で全ゾーンを検索できます。両経路は併用できません。"`
	Server serverapi.Commands `cmd:"" help:"さくらのクラウドのサーバを検索、参照、作成、更新、削除します。作成や複雑な構成には --request JSON を使用します。find は --zone all で全ゾーンを検索できます。"`
}

func (c *cli) initIaaSAPI() {
	c.IaaSAPI.Switch.SetRuntime(switchapi.Runtime{
		DecodeRequest:   decodeRequest,
		OutputType:      outputType,
		WriteOutput:     writeOutputWithFormat,
		ValidateRequest: switchresource.ValidateRequest,
	})
	c.IaaSAPI.Switch.SetFactory(func() (switchapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return swytch.New(client), nil
	})
	c.IaaSAPI.Switch.SetZoneFactory(func() (iaaszones.API, error) {
		return iaaszones.New(c.Trace)
	})

	c.IaaSAPI.Server.SetRuntime(serverapi.Runtime{
		DecodeRequest:   decodeRequest,
		OutputType:      outputType,
		WriteOutput:     writeOutputWithFormat,
		ValidateRequest: serverresource.ValidateRequest,
	})
	c.IaaSAPI.Server.SetFactory(func() (serverapi.API, error) {
		client, err := iaasclient.New(c.Trace)
		if err != nil {
			return nil, err
		}
		return serverSDK.New(client), nil
	})
	c.IaaSAPI.Server.SetZoneFactory(func() (iaaszones.API, error) {
		return iaaszones.New(c.Trace)
	})
}
