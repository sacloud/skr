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

package zones

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/zone"
)

func TestFindInAllReturnsErrorWithoutPartialResults(t *testing.T) {
	newZoneAPI := func() (API, error) {
		return testAPI{zones: []*iaas.Zone{{Name: "zone-a"}, {Name: "zone-b"}}}, nil
	}
	result, err := FindInAll(context.Background(), newZoneAPI, func(_ context.Context, zone string) ([]string, error) {
		if zone == "zone-b" {
			return []string{"partial"}, errors.New("mock search failure")
		}
		return []string{"first"}, nil
	})
	if err == nil || !strings.Contains(err.Error(), `zone "zone-b"`) {
		t.Fatalf("FindInAll error = %v, want zone-specific error", err)
	}
	if result != nil {
		t.Fatalf("FindInAll returned partial results %#v, want nil", result)
	}
}

type testAPI struct {
	zones []*iaas.Zone
}

func (api testAPI) FindWithContext(context.Context, *zone.FindRequest) ([]*iaas.Zone, error) {
	return api.zones, nil
}
