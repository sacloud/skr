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

package containerregistryapi

import (
	"context"
	"fmt"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	sdk "github.com/sacloud/sacloud-sdk-go/service/iaas/containerregistry"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
)

type sdkAPI struct {
	registries *sdk.Service
	users      iaas.ContainerRegistryAPI
}

func NewAPI(trace bool) (API, error) {
	client, err := iaasclient.New(trace)
	if err != nil {
		return nil, err
	}
	return newSDKAPI(client), nil
}

func newSDKAPI(caller iaas.APICaller) *sdkAPI {
	return &sdkAPI{
		registries: sdk.New(caller),
		users:      iaas.NewContainerRegistryOp(caller),
	}
}

func (a *sdkAPI) Find(ctx context.Context, request *sdk.FindRequest) ([]*iaas.ContainerRegistry, error) {
	result, err := a.registries.FindWithContext(ctx, request)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []*iaas.ContainerRegistry{}, nil
	}
	return result, nil
}

func (a *sdkAPI) Read(ctx context.Context, request *sdk.ReadRequest) (*iaas.ContainerRegistry, error) {
	return a.registries.ReadWithContext(ctx, request)
}

func (a *sdkAPI) Create(ctx context.Context, request *sdk.CreateRequest) (*iaas.ContainerRegistry, error) {
	return a.registries.CreateWithContext(ctx, request)
}

func (a *sdkAPI) Update(ctx context.Context, request *sdk.UpdateRequest) (*iaas.ContainerRegistry, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	current, err := a.users.Read(ctx, request.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf("API returned an empty container registry")
	}
	update := &iaas.ContainerRegistryUpdateRequest{
		Name:          current.Name,
		Description:   current.Description,
		Tags:          current.Tags,
		IconID:        current.IconID,
		AccessLevel:   current.AccessLevel,
		VirtualDomain: current.VirtualDomain,
		SettingsHash:  current.SettingsHash,
	}
	if request.Description != nil {
		update.Description = *request.Description
	}
	if request.Tags != nil {
		update.Tags = *request.Tags
	}
	if request.IconID != nil {
		update.IconID = *request.IconID
	}
	if request.VirtualDomain != nil {
		update.VirtualDomain = *request.VirtualDomain
	}
	return a.users.Update(ctx, request.ID, update)
}

func (a *sdkAPI) Delete(ctx context.Context, request *sdk.DeleteRequest) error {
	return a.registries.DeleteWithContext(ctx, request)
}

func (a *sdkAPI) ListUsers(ctx context.Context, id types.ID) ([]User, error) {
	result, err := a.users.ListUsers(ctx, id)
	if err != nil {
		return nil, err
	}
	users := make([]User, 0)
	if result == nil {
		return users, nil
	}
	for _, user := range result.Users {
		if user == nil {
			return nil, fmt.Errorf("SDK returned an empty container registry user")
		}
		users = append(users, User{
			UserName:   user.UserName,
			Permission: user.Permission,
		})
	}
	return users, nil
}

func (a *sdkAPI) AddUser(ctx context.Context, id types.ID, request *iaas.ContainerRegistryUserCreateRequest) error {
	return a.users.AddUser(ctx, id, request)
}

func (a *sdkAPI) UpdateUser(ctx context.Context, id types.ID, userName string, request *iaas.ContainerRegistryUserUpdateRequest) error {
	return a.users.UpdateUser(ctx, id, userName, request)
}

func (a *sdkAPI) DeleteUser(ctx context.Context, id types.ID, userName string) error {
	return a.users.DeleteUser(ctx, id, userName)
}
