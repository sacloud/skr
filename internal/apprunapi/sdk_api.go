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

import (
	"github.com/sacloud/sacloud-sdk-go/api/apprun"
	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun/apis/v1"
	clientconfig "github.com/sacloud/skr/internal/saclient"
)

func NewApplicationAPI(trace bool) (ApplicationAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return apprun.NewApplicationOp(client), nil
}

func NewVersionAPI(trace bool) (VersionAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return apprun.NewVersionOp(client), nil
}

func NewPacketFilterAPI(trace bool) (PacketFilterAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return apprun.NewPacketFilterOp(client), nil
}

func NewTrafficAPI(trace bool) (TrafficAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return apprun.NewTrafficOp(client), nil
}

func NewUserAPI(trace bool) (UserAPI, error) {
	client, err := newClient(trace)
	if err != nil {
		return nil, err
	}
	return apprun.NewUserOp(client), nil
}

func newClient(trace bool) (*v1.Client, error) {
	client, err := clientconfig.New(trace)
	if err != nil {
		return nil, err
	}
	return apprun.NewClient(client)
}

func NewOptApplicationSortOrder(value string) v1.OptListApplicationsSortOrder {
	return v1.NewOptListApplicationsSortOrder(v1.ListApplicationsSortOrder(value))
}

func NewOptVersionSortOrder(value string) v1.OptListApplicationVersionsSortOrder {
	return v1.NewOptListApplicationVersionsSortOrder(v1.ListApplicationVersionsSortOrder(value))
}
