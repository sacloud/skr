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
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/stretchr/testify/require"
)

func TestDeleteWithForceUsesDirectAPIOperations(t *testing.T) {
	api := &internetAPIFake{
		readResult: &iaas.Internet{
			ID: 123,
			Switch: &iaas.SwitchInfo{
				IPv6Nets: []*iaas.IPv6NetInfo{{ID: 456}},
				Subnets:  []*iaas.InternetSubnet{{ID: 789, NextHop: "192.0.2.1"}},
			},
		},
	}
	sdk := &sdkAPI{op: api}

	err := sdk.DeleteWithContext(context.Background(), &DeleteRequest{
		Zone:  "is1b",
		ID:    123,
		Force: true,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"read", "disable-ipv6", "delete-subnet", "delete"}, api.calls)
	require.EqualValues(t, 456, api.ipv6ID)
	require.EqualValues(t, 789, api.subnetID)
}

func TestUpdateWithBandwidthOnlyCallsBandwidthAPI(t *testing.T) {
	api := &internetAPIFake{bandwidthResult: &iaas.Internet{ID: 123, BandWidthMbps: 200}}
	sdk := &sdkAPI{op: api}

	result, err := sdk.UpdateWithContext(context.Background(), &UpdateRequest{
		Zone:          "is1b",
		ID:            123,
		BandWidthMbps: intPointer(200),
	})
	require.NoError(t, err)
	require.EqualValues(t, 200, result.BandWidthMbps)
	require.Equal(t, 0, api.updateCalls)
	require.Equal(t, 1, api.bandwidthCalls)
}

func TestUpdateDoesNotMutateWhenReadReturnsNoInternet(t *testing.T) {
	api := &internetAPIFake{}
	sdk := &sdkAPI{op: api}
	name := "updated"

	_, err := sdk.UpdateWithContext(context.Background(), &UpdateRequest{
		Zone: "is1b",
		ID:   123,
		Name: &name,
	})
	require.ErrorContains(t, err, "読み込み結果が空")
	require.Zero(t, api.updateCalls)
}

func TestUpdatePreservesUnspecifiedFieldsInAPIRequest(t *testing.T) {
	caller := &internetAPICallerFake{}
	sdk := &sdkAPI{op: iaas.NewInternetOp(caller)}
	name := "updated"

	_, err := sdk.UpdateWithContext(context.Background(), &UpdateRequest{
		Zone: "is1b",
		ID:   123,
		Name: &name,
	})
	require.NoError(t, err)
	var body struct {
		Internet map[string]json.RawMessage
	}
	require.NoError(t, json.Unmarshal(caller.body, &body))
	require.JSONEq(t, `"updated"`, string(body.Internet["Name"]))
	require.JSONEq(t, `"preserve me"`, string(body.Internet["Description"]))
	require.JSONEq(t, `["keep"]`, string(body.Internet["Tags"]))
	require.JSONEq(t, `{"ID":42,"Tags":[]}`, string(body.Internet["Icon"]))
}

func TestListSubnetReadsFullSubnetDetailsFromAPI(t *testing.T) {
	caller := &internetSubnetCallerFake{}
	sdk := NewAPI(caller)

	result, err := sdk.ListSubnetWithContext(context.Background(), &ListSubnetRequest{
		Zone: "is1b",
		ID:   123,
	})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.EqualValues(t, 789, result[0].ID)
	require.EqualValues(t, 123, result[0].InternetID)
	require.EqualValues(t, 456, result[0].SwitchID)
	require.Len(t, result[0].IPAddresses, 1)
	require.Equal(t, "192.0.2.2", result[0].IPAddresses[0].IPAddress)
	require.Len(t, caller.uris, 2)
	require.Contains(t, caller.uris[0], "/internet/123")
	require.Contains(t, caller.uris[1], "/subnet/789")
}

func TestCreateRejectsServiceOnlyOptions(t *testing.T) {
	sdk := &sdkAPI{op: &internetAPIFake{}}
	_, err := sdk.CreateWithContext(context.Background(), &CreateRequest{
		Zone:   "is1b",
		Name:   "router",
		NoWait: true,
	})
	require.ErrorContains(t, err, "NoWait")
}

func TestCreateWaitsForInternetToBecomeReadableBeforeEnablingIPv6(t *testing.T) {
	notFound := iaas.NewAPIError(http.MethodGet, &url.URL{Path: "/internet/123"}, http.StatusNotFound, &iaas.APIErrorResponse{})
	op := &internetAPIFake{
		createResult: &iaas.Internet{ID: 123},
		readResults: []internetReadResult{
			{err: notFound},
			{internet: &iaas.Internet{ID: 123}},
		},
	}
	sdk := &sdkAPI{op: op}

	result, err := sdk.CreateWithContext(context.Background(), &CreateRequest{
		Zone:       "is1b",
		Name:       "router",
		EnableIPv6: true,
	})

	require.NoError(t, err)
	require.EqualValues(t, 123, result.ID)
	require.Equal(t, []string{"create", "read", "read", "enable-ipv6"}, op.calls)
}

func TestMonitorRouterUsesOneHourWindowWhenOnlyEndIsSpecified(t *testing.T) {
	end := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	op := &internetAPIFake{routerActivity: &iaas.RouterActivity{}}
	sdk := &sdkAPI{op: op}

	_, err := sdk.MonitorRouterWithContext(context.Background(), &MonitorRouterRequest{
		Zone: "is1b",
		ID:   123,
		End:  end,
	})

	require.NoError(t, err)
	require.NotNil(t, op.monitorCondition)
	require.Equal(t, end, op.monitorCondition.End)
	require.Equal(t, end.Add(-time.Hour), op.monitorCondition.Start)
}

func TestMonitorRouterDefaultsBothTimes(t *testing.T) {
	op := &internetAPIFake{routerActivity: &iaas.RouterActivity{}}
	sdk := &sdkAPI{op: op}
	before := time.Now()

	_, err := sdk.MonitorRouterWithContext(context.Background(), &MonitorRouterRequest{
		Zone: "is1b",
		ID:   123,
	})

	after := time.Now()
	require.NoError(t, err)
	require.NotNil(t, op.monitorCondition)
	require.False(t, op.monitorCondition.End.Before(before))
	require.False(t, op.monitorCondition.End.After(after))
	require.Equal(t, time.Hour, op.monitorCondition.End.Sub(op.monitorCondition.Start))
}

func intPointer(value int) *int {
	return &value
}

type internetReadResult struct {
	internet *iaas.Internet
	err      error
}

type internetAPIFake struct {
	iaas.InternetAPI
	createResult     *iaas.Internet
	readResult       *iaas.Internet
	readResults      []internetReadResult
	bandwidthResult  *iaas.Internet
	routerActivity   *iaas.RouterActivity
	monitorCondition *iaas.MonitorCondition
	calls            []string
	ipv6ID           types.ID
	subnetID         types.ID
	updateCalls      int
	bandwidthCalls   int
}

type internetAPICallerFake struct {
	body []byte
}

type internetSubnetCallerFake struct {
	uris []string
}

func (caller *internetSubnetCallerFake) Do(_ context.Context, _, uri string, _ any) ([]byte, error) {
	caller.uris = append(caller.uris, uri)
	if strings.Contains(uri, "/subnet/") {
		return []byte(`{"Subnet":{"ID":789,"Switch":{"ID":456,"Internet":{"ID":123}},"NetworkAddress":"192.0.2.0","NetworkMaskLen":28,"DefaultRoute":"192.0.2.1","IPAddresses":[{"IPAddress":"192.0.2.2"}]}}`), nil
	}
	return []byte(`{"Internet":{"ID":123,"Switch":{"ID":456,"Subnets":[{"ID":789}]}}}`), nil
}

func (caller *internetAPICallerFake) Do(_ context.Context, _, _ string, body any) ([]byte, error) {
	if body != nil {
		var err error
		caller.body, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	return []byte(`{"Internet":{"ID":123,"Name":"current","Description":"preserve me","Tags":["keep"],"Icon":{"ID":42}}}`), nil
}

func (api *internetAPIFake) Create(_ context.Context, _ string, _ *iaas.InternetCreateRequest) (*iaas.Internet, error) {
	api.calls = append(api.calls, "create")
	return api.createResult, nil
}

func (api *internetAPIFake) Read(ctx context.Context, zone string, id types.ID) (*iaas.Internet, error) {
	api.calls = append(api.calls, "read")
	if len(api.readResults) > 0 {
		result := api.readResults[0]
		api.readResults = api.readResults[1:]
		return result.internet, result.err
	}
	return api.readResult, nil
}

func (api *internetAPIFake) EnableIPv6(_ context.Context, _ string, _ types.ID) (*iaas.IPv6NetInfo, error) {
	api.calls = append(api.calls, "enable-ipv6")
	return &iaas.IPv6NetInfo{}, nil
}

func (api *internetAPIFake) MonitorRouter(_ context.Context, _ string, _ types.ID, condition *iaas.MonitorCondition) (*iaas.RouterActivity, error) {
	api.monitorCondition = condition
	return api.routerActivity, nil
}

func (api *internetAPIFake) DisableIPv6(ctx context.Context, zone string, id, ipv6ID types.ID) error {
	api.calls = append(api.calls, "disable-ipv6")
	api.ipv6ID = ipv6ID
	return nil
}

func (api *internetAPIFake) DeleteSubnet(ctx context.Context, zone string, id, subnetID types.ID) error {
	api.calls = append(api.calls, "delete-subnet")
	api.subnetID = subnetID
	return nil
}

func (api *internetAPIFake) Delete(ctx context.Context, zone string, id types.ID) error {
	api.calls = append(api.calls, "delete")
	return nil
}

func (api *internetAPIFake) Update(ctx context.Context, zone string, id types.ID, request *iaas.InternetUpdateRequest) (*iaas.Internet, error) {
	api.updateCalls++
	return &iaas.Internet{ID: id}, nil
}

func (api *internetAPIFake) UpdateBandWidth(ctx context.Context, zone string, id types.ID, request *iaas.InternetUpdateBandWidthRequest) (*iaas.Internet, error) {
	api.bandwidthCalls++
	return api.bandwidthResult, nil
}
