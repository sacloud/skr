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
	"strings"
	"testing"

	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun/apis/v1"
)

func TestValidateApplicationListRequestSortOrder(t *testing.T) {
	for _, order := range []string{"asc", "desc"} {
		request := &v1.ListApplicationsParams{SortOrder: NewOptApplicationSortOrder(order)}
		if err := ValidateApplicationListRequest(request); err != nil {
			t.Errorf("ValidateApplicationListRequest(%q) error = %v", order, err)
		}
	}
	if err := ValidateApplicationListRequest(&v1.ListApplicationsParams{}); err != nil {
		t.Errorf("ValidateApplicationListRequest(omitted) error = %v", err)
	}
	request := &v1.ListApplicationsParams{SortOrder: NewOptApplicationSortOrder("invalid")}
	if err := ValidateApplicationListRequest(request); err == nil || !strings.Contains(err.Error(), "sort-order must be asc or desc") {
		t.Fatalf("ValidateApplicationListRequest(invalid) error = %v", err)
	}
}

func TestValidateVersionListRequestSortOrder(t *testing.T) {
	for _, order := range []string{"asc", "desc"} {
		request := &v1.ListApplicationVersionsParams{SortOrder: NewOptVersionSortOrder(order)}
		if err := ValidateVersionListRequest(request); err != nil {
			t.Errorf("ValidateVersionListRequest(%q) error = %v", order, err)
		}
	}
	if err := ValidateVersionListRequest(&v1.ListApplicationVersionsParams{}); err != nil {
		t.Errorf("ValidateVersionListRequest(omitted) error = %v", err)
	}
	request := &v1.ListApplicationVersionsParams{SortOrder: NewOptVersionSortOrder("invalid")}
	if err := ValidateVersionListRequest(request); err == nil || !strings.Contains(err.Error(), "sort-order must be asc or desc") {
		t.Fatalf("ValidateVersionListRequest(invalid) error = %v", err)
	}
}
