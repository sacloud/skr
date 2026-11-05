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

package archiveapi

import (
	"context"
	"maps"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/ostype"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
)

type sdkAPI struct {
	op iaas.ArchiveAPI
}

func NewAPI(caller iaas.APICaller) API {
	return &sdkAPI{op: iaas.NewArchiveOp(caller)}
}

func (api *sdkAPI) FindWithContext(ctx context.Context, request *FindRequest) ([]*iaas.Archive, error) {
	condition := &iaas.FindCondition{
		Count:  request.Count,
		From:   request.From,
		Sort:   request.Sort,
		Filter: map[search.FilterKey]any{},
	}
	if filter, ok := ostypeCriteria(request.OSType); ok {
		maps.Copy(condition.Filter, filter)
	}
	if len(request.Names) != 0 {
		condition.Filter[search.Key("Name")] = search.AndEqual(request.Names...)
	}
	if len(request.Tags) != 0 {
		condition.Filter[search.Key("Tags.Name")] = search.TagsAndEqual(request.Tags...)
	}
	if request.Scope != "" {
		condition.Filter[search.Key("Scope")] = search.OrEqual(request.Scope)
	}
	result, err := api.op.Find(ctx, request.Zone, condition)
	if err != nil {
		return nil, err
	}
	return result.Archives, nil
}

func (api *sdkAPI) ReadWithContext(ctx context.Context, request *ReadRequest) (*iaas.Archive, error) {
	return api.op.Read(ctx, request.Zone, request.ID)
}

func ostypeCriteria(value ostype.ArchiveOSType) (map[search.FilterKey]any, bool) {
	filter, ok := ostype.ArchiveCriteria[value]
	if !ok {
		return nil, false
	}
	result := make(map[search.FilterKey]any, len(filter))
	for key, value := range filter {
		result[key] = value
	}
	return result, true
}
