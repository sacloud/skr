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

package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
)

func TestSubnetArgsUseSubnetOptions(t *testing.T) {
	request := map[string]any{"Zone": zone, "ID": types.ID(12), "NetworkMaskLen": int64(28), "NextHop": "192.0.2.2"}
	args, err := internetArgs("test-add-subnet", request, "")
	if err != nil || strings.Join(args, " ") != "iaas-api internet add-subnet --zone is1b --id 12 --network-mask-len 28 --next-hop 192.0.2.2" {
		t.Fatalf("add-subnet args = %v, error = %v", args, err)
	}
	request = map[string]any{"Zone": zone, "ID": types.ID(12), "SubnetID": int64(15), "NextHop": "192.0.2.3"}
	args, err = internetArgs("test-update-subnet", request, "")
	if err != nil || strings.Join(args, " ") != "iaas-api internet update-subnet --zone is1b --id 12 --subnet-id 15 --next-hop 192.0.2.3" {
		t.Fatalf("update-subnet args = %v, error = %v", args, err)
	}
	args, err = internetArgs("test-delete-subnet", request, "")
	if err != nil || strings.Join(args, " ") != "iaas-api internet delete-subnet --zone is1b --id 12 --subnet-id 15" {
		t.Fatalf("delete-subnet args = %v, error = %v", args, err)
	}
}

func TestSubnetArgsRejectInvalidRequests(t *testing.T) {
	for _, test := range []struct {
		name    string
		step    string
		request map[string]any
	}{
		{"add-subnet missing NextHop", "test-add-subnet", map[string]any{"Zone": zone, "ID": types.ID(12), "NetworkMaskLen": int64(28)}},
		{"add-subnet IPv6 next hop", "test-add-subnet", map[string]any{"Zone": zone, "ID": types.ID(12), "NetworkMaskLen": int64(28), "NextHop": "2001:db8::1"}},
		{"add-subnet invalid mask", "test-add-subnet", map[string]any{"Zone": zone, "ID": types.ID(12), "NetworkMaskLen": int64(0), "NextHop": "192.0.2.2"}},
		{"update-subnet missing SubnetID", "test-update-subnet", map[string]any{"Zone": zone, "ID": types.ID(12), "NextHop": "192.0.2.3"}},
		{"update-subnet IPv6 next hop", "test-update-subnet", map[string]any{"Zone": zone, "ID": types.ID(12), "SubnetID": int64(15), "NextHop": "::1"}},
		{"delete-subnet missing ID", "test-delete-subnet", map[string]any{"Zone": zone, "SubnetID": int64(15)}},
	} {
		if _, err := internetArgs(test.step, test.request, ""); err == nil {
			t.Errorf("%s: accepted an invalid request: %+v", test.name, test.request)
		}
	}
}

func TestIPv6ArgsUseIndividualFlags(t *testing.T) {
	request := map[string]any{"Zone": zone, "ID": types.ID(12)}
	for _, step := range []string{"test-enable-ipv6", "test-read-ipv6", "test-disable-ipv6"} {
		args, err := internetArgs(step, request, "")
		want := "iaas-api internet " + strings.TrimPrefix(step, "test-") + " --zone is1b --id 12"
		if err != nil || strings.Join(args, " ") != want {
			t.Fatalf("%s args = %v, error = %v", step, args, err)
		}
	}
}

func TestUpdatedNextHopIsFreeAddressInPrimarySubnet(t *testing.T) {
	nextHop, err := updatedNextHop("192.0.2.2")
	if err != nil || nextHop != "192.0.2.3" {
		t.Fatalf("updatedNextHop = %q, error = %v", nextHop, err)
	}
	for _, invalid := range []string{"2001:db8::1", "not-an-ip"} {
		if _, err := updatedNextHop(invalid); err == nil {
			t.Errorf("updatedNextHop accepted %q", invalid)
		}
	}
}

func TestSubnetScenarioAddsUpdatesDeletesAndVerifiesAbsence(t *testing.T) {
	fake := &fakeCLI{internet: &internetItem{ID: 12, Name: "router", Description: internetDesc}}
	fake.internet.Switch.ID = 13
	test := testScenario(fake, t.TempDir())
	if err := test.subnetScenario(context.Background(), 12, "192.0.2.2"); err != nil {
		t.Fatal(err)
	}
	if fake.addedSubnet != nil {
		t.Fatalf("added Subnet remains: %+v", fake.addedSubnet)
	}
	for _, step := range []string{"test-add-subnet", "test-update-subnet", "test-delete-subnet"} {
		if !containsStep(fake.calls, step) {
			t.Errorf("scenario did not call %s", step)
		}
	}
}

func TestSubnetScenarioReportsFailures(t *testing.T) {
	base := func() *fakeCLI {
		return &fakeCLI{internet: &internetItem{ID: 12, Name: "router", Description: internetDesc}}
	}
	t.Run("update ignored", func(t *testing.T) {
		fake := base()
		test := testScenario(fake, t.TempDir())
		test.client = cliFunc(func(ctx context.Context, step string, request map[string]any) ([]byte, error) {
			if step == "test-update-subnet" {
				return []byte(`{"ID":15,"NextHop":"192.0.2.2","NetworkAddress":"198.51.100.0","NetworkMaskLen":28}`), nil
			}
			return fake.call(ctx, step, request)
		})
		err := test.subnetScenario(context.Background(), 12, "192.0.2.2")
		if err == nil || !strings.Contains(err.Error(), "updated Subnet NextHop mismatch") {
			t.Fatalf("error = %v, want updated Subnet NextHop mismatch", err)
		}
	})
	t.Run("delete keeps block", func(t *testing.T) {
		fake := base()
		test := testScenario(fake, t.TempDir())
		test.client = cliFunc(func(ctx context.Context, step string, request map[string]any) ([]byte, error) {
			if step == "test-delete-subnet" {
				return nil, nil
			}
			return fake.call(ctx, step, request)
		})
		err := test.subnetScenario(context.Background(), 12, "192.0.2.2")
		if err == nil || !strings.Contains(err.Error(), "remains after delete-subnet") {
			t.Fatalf("error = %v, want absence check failure", err)
		}
	})
	t.Run("api error", func(t *testing.T) {
		fake := base()
		test := testScenario(fake, t.TempDir())
		test.client = cliFunc(func(ctx context.Context, step string, request map[string]any) ([]byte, error) {
			if step == "test-add-subnet" {
				return nil, errors.New("injected CLI failure")
			}
			return fake.call(ctx, step, request)
		})
		if err := test.subnetScenario(context.Background(), 12, "192.0.2.2"); err == nil {
			t.Fatal("subnet scenario succeeded despite an injected add-subnet failure")
		}
	})
}

func TestIPv6ScenarioEnablesReadsAndDisables(t *testing.T) {
	fake := &fakeCLI{internet: &internetItem{ID: 12, Name: "router", Description: internetDesc}}
	fake.internet.Switch.ID = 13
	test := testScenario(fake, t.TempDir())
	if err := test.ipv6Scenario(context.Background(), 12); err != nil {
		t.Fatal(err)
	}
	if fake.ipv6Net != nil {
		t.Fatalf("IPv6Net remains enabled: %+v", fake.ipv6Net)
	}
	for _, step := range []string{"test-enable-ipv6", "test-read-ipv6", "test-disable-ipv6"} {
		if !containsStep(fake.calls, step) {
			t.Errorf("scenario did not call %s", step)
		}
	}
}

func TestIPv6ScenarioReportsFailures(t *testing.T) {
	base := func() *fakeCLI {
		return &fakeCLI{internet: &internetItem{ID: 12, Name: "router", Description: internetDesc}}
	}
	t.Run("read-ipv6 succeeds after disable", func(t *testing.T) {
		fake := base()
		test := testScenario(fake, t.TempDir())
		test.client = cliFunc(func(ctx context.Context, step string, request map[string]any) ([]byte, error) {
			if step == "test-disable-ipv6" {
				return nil, nil
			}
			return fake.call(ctx, step, request)
		})
		err := test.ipv6Scenario(context.Background(), 12)
		if err == nil || !strings.Contains(err.Error(), "read-ipv6 succeeded after disable-ipv6") {
			t.Fatalf("error = %v, want disable verification failure", err)
		}
	})
	t.Run("read mismatch", func(t *testing.T) {
		fake := base()
		test := testScenario(fake, t.TempDir())
		test.client = cliFunc(func(ctx context.Context, step string, request map[string]any) ([]byte, error) {
			if step == "test-read-ipv6" {
				return []byte(`{"ID":99,"IPv6Prefix":"2001:db8:1::","IPv6PrefixLen":64}`), nil
			}
			return fake.call(ctx, step, request)
		})
		err := test.ipv6Scenario(context.Background(), 12)
		if err == nil || !strings.Contains(err.Error(), "read IPv6Net identity mismatch") {
			t.Fatalf("error = %v, want read identity mismatch", err)
		}
	})
}
