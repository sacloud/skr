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

package internetapi

import (
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

type FindRequest struct {
	Zone        string
	Names       []string
	SwitchIDs   []string
	SwitchNames []string
	Tags        types.Tags
	Sort        search.SortKeys
	Count       int
	From        int
}

type ReadRequest struct {
	Zone string
	ID   types.ID
}

type CreateRequest struct {
	Zone           string
	Name           string
	Description    string
	Tags           types.Tags
	IconID         types.ID
	NetworkMaskLen int
	BandWidthMbps  int
	EnableIPv6     bool
	NoWait         bool
	NotFoundRetry  int
}

type UpdateRequest struct {
	Zone          string
	ID            types.ID
	Tags          *types.Tags
	Name          *string
	Description   *string
	IconID        *types.ID
	BandWidthMbps *int
	EnableIPv6    *bool
}

type DeleteRequest struct {
	Zone           string
	ID             types.ID
	Force          bool
	FailIfNotFound bool
}

type ListSubnetRequest struct {
	Zone string
	ID   types.ID
}

type AddSubnetRequest struct {
	Zone           string
	ID             types.ID
	NetworkMaskLen int
	NextHop        string
}

type UpdateSubnetRequest struct {
	Zone     string
	ID       types.ID
	SubnetID types.ID
	NextHop  string
}

type DeleteSubnetRequest struct {
	Zone     string
	ID       types.ID
	SubnetID types.ID
}

type ReadIPv6Request struct {
	Zone string
	ID   types.ID
}

type EnableIPv6Request struct {
	Zone string
	ID   types.ID
}

type DisableIPv6Request struct {
	Zone string
	ID   types.ID
}

type MonitorRouterRequest struct {
	Zone  string
	ID    types.ID
	Start time.Time
	End   time.Time
}
