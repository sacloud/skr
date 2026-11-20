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
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/internal/nfs"
)

// initNFS は手書きの高レベル nfs コマンド群を配線します (ADR 0096)。
func (c *cli) initNFS() {
	c.NFS = nfs.NewCommands(
		nfs.Runtime{
			ValidateOutput: validateOutput,
			WriteOutput:    writeOutput,
		},
		func() (*nfs.Service, error) {
			client, err := iaasclient.New(c.Trace)
			if err != nil {
				return nil, err
			}
			return nfs.NewService(iaas.NewNoteOp(client)), nil
		},
	)
}
