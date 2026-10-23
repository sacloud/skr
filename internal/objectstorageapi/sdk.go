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

package objectstorageapi

import (
	objectstorage "github.com/sacloud/sacloud-sdk-go/api/object-storage"
	"github.com/sacloud/skr/internal/saclient"
)

func NewAPI(siteID string, trace bool) (API, error) {
	client, err := saclient.New(trace)
	if err != nil {
		return API{}, err
	}
	fedClient, err := objectstorage.NewFedClient(client)
	if err != nil {
		return API{}, err
	}
	siteClient, err := objectstorage.NewSiteClient(client, siteID)
	if err != nil {
		return API{}, err
	}

	return API{
		Site:    objectstorage.NewSiteWithPlansOp(fedClient, siteClient),
		Account: objectstorage.NewAccountOp(siteClient),
		Bucket:  objectstorage.NewBucketOp(fedClient, siteClient),
		BucketExtra: func(bucket string) objectstorage.BucketExtraAPI {
			return objectstorage.NewBucketExtraOp(siteClient, fedClient, bucket)
		},
		Permission: objectstorage.NewPermissionOp(siteClient),
		SiteStatus: objectstorage.NewSiteStatusOp(siteClient),
	}, nil
}
