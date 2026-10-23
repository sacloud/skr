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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	objectstorage "github.com/sacloud/sacloud-sdk-go/api/object-storage"
	v2 "github.com/sacloud/sacloud-sdk-go/api/object-storage/apis/v2"
)

func TestBucketCreateUsesSiteBucketAndPlan(t *testing.T) {
	var got *objectstorage.BucketCreateParams
	commands := NewCommands(testRuntime(), func(siteID string) (API, error) {
		return API{
			Bucket: &fakeBucketAPI{create: func(_ context.Context, params *objectstorage.BucketCreateParams) (*v2.ModelBucket, error) {
				got = params
				return &v2.ModelBucket{}, nil
			}},
		}, nil
	})

	runCommand(t, commands, []string{"bucket", "create", "arc02", "test-bucket", "--plan", "example"})

	if got == nil {
		t.Fatal("Bucket.Create was not called")
	}
	if got.SiteId != "arc02" || got.Bucket != "test-bucket" || got.Plan != "example" {
		t.Fatalf("Bucket.Create params = %#v", got)
	}
}

func TestBucketCreateValidatesSDKSupportedSitesAndPlans(t *testing.T) {
	for _, test := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "archive plan required", args: []string{"bucket", "create", "arc02", "test-bucket"}, wantErr: "--site-id arc02 では --plan が必要です"},
		{name: "plan not used on standard site", args: []string{"bucket", "create", "isk01", "test-bucket", "--plan", "example"}, wantErr: "--plan はアーカイブプラン用サイト arc02 でのみ指定できます"},
		{name: "unsupported site", args: []string{"bucket", "create", "unknown", "test-bucket"}, wantErr: "バケット作成に対応するサイト ID は isk01、tky01、arc02"},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			commands := NewCommands(testRuntime(), func(string) (API, error) {
				called = true
				return API{}, nil
			})
			err := runCommandError(t, commands, test.args)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
			if called {
				t.Fatal("API factory was called for invalid bucket creation input")
			}
		})
	}
}

func TestPermissionCreateDecodesSDKRequest(t *testing.T) {
	var gotName string
	var gotControls v2.BucketControls
	commands := NewCommands(testRuntime(), func(string) (API, error) {
		return API{
			Permission: &fakePermissionAPI{create: func(_ context.Context, name string, controls v2.BucketControls) (*v2.PermissionData, error) {
				gotName = name
				gotControls = controls
				return &v2.PermissionData{}, nil
			}},
		}, nil
	})

	runCommand(t, commands, []string{
		"permission", "create", "isk01",
		`--request={"display_name":"tutorial","bucket_controls":[{"bucket_name":"example-bucket","can_read":true,"can_write":false}]}`,
	})

	if gotName != "tutorial" {
		t.Fatalf("display name = %q, want tutorial", gotName)
	}
	var want v2.BucketControls
	if err := json.Unmarshal([]byte(`[{"bucket_name":"example-bucket","can_read":true,"can_write":false}]`), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotControls, want) {
		t.Fatalf("bucket controls = %#v, want %#v", gotControls, want)
	}
}

func TestBucketEncryptionUsesBucketBoundOperation(t *testing.T) {
	var gotBucket, gotKey string
	commands := NewCommands(testRuntime(), func(string) (API, error) {
		return API{
			BucketExtra: func(bucket string) objectstorage.BucketExtraAPI {
				gotBucket = bucket
				return &fakeBucketExtraAPI{enableEncryption: func(_ context.Context, key string) error {
					gotKey = key
					return nil
				}}
			},
		}, nil
	})

	runCommand(t, commands, []string{"bucket", "encryption", "enable", "isk01", "example-bucket", "--kms-key-id", "kms-key"})
	if gotBucket != "example-bucket" || gotKey != "kms-key" {
		t.Fatalf("bucket = %q, key = %q", gotBucket, gotKey)
	}
}

func TestPermissionCreateReturnsSDKError(t *testing.T) {
	wantErr := errors.New("permission create failed")
	commands := NewCommands(testRuntime(), func(string) (API, error) {
		return API{
			Permission: &fakePermissionAPI{create: func(context.Context, string, v2.BucketControls) (*v2.PermissionData, error) {
				return nil, wantErr
			}},
		}, nil
	})

	err := runCommandError(t, commands, []string{
		"permission", "create", "isk01",
		`--request={"display_name":"tutorial","bucket_controls":[]}`,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

type fakeBucketAPI struct {
	objectstorage.BucketAPI
	create func(context.Context, *objectstorage.BucketCreateParams) (*v2.ModelBucket, error)
	list   func(context.Context) ([]v2.BucketListDataItem, error)
}

func (f *fakeBucketAPI) List(ctx context.Context) ([]v2.BucketListDataItem, error) {
	return f.list(ctx)
}

func (f *fakeBucketAPI) Create(ctx context.Context, params *objectstorage.BucketCreateParams) (*v2.ModelBucket, error) {
	return f.create(ctx, params)
}

type fakePermissionAPI struct {
	objectstorage.PermissionsAPI
	create func(context.Context, string, v2.BucketControls) (*v2.PermissionData, error)
}

func (f *fakePermissionAPI) Create(ctx context.Context, name string, controls v2.BucketControls) (*v2.PermissionData, error) {
	return f.create(ctx, name, controls)
}

type fakeBucketExtraAPI struct {
	objectstorage.BucketExtraAPI
	enableEncryption func(context.Context, string) error
}

func (f *fakeBucketExtraAPI) EnableEncryption(ctx context.Context, keyID string) error {
	return f.enableEncryption(ctx, keyID)
}

func testRuntime() Runtime {
	return Runtime{
		DecodeRequest: func(input string, destination any) error {
			return json.Unmarshal([]byte(input), destination)
		},
		ValidateOutput: func(*kong.Context) error { return nil },
		WriteOutput:    func(*kong.Context, any) error { return nil },
	}
}

func runCommand(t *testing.T, commands Commands, args []string) {
	t.Helper()
	if err := runCommandError(t, commands, args); err != nil {
		t.Fatal(err)
	}
}

func runCommandError(t *testing.T, commands Commands, args []string) error {
	t.Helper()
	var stdout, stderr bytes.Buffer
	parser, err := kong.New(&commands, kong.Writers(&stdout, &stderr))
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := parser.Parse(args)
	if err != nil {
		t.Fatalf("parse command: %v; stderr: %s", err, stderr.String())
	}
	return ctx.Run()
}

func TestFederationOperationsRefuseUnconfirmedBucketSite(t *testing.T) {
	for _, args := range [][]string{
		{"bucket", "delete", "tky01", "source"},
		{"bucket", "replication", "read", "tky01", "source"},
		{"bucket", "replication", "enable", "tky01", "source", "--target-bucket", "target"},
		{"bucket", "replication", "disable", "tky01", "source"},
	} {
		for _, response := range []string{"[]", "null", `[{"name":"other"}]`, `[{"name":"source"},{"name":"source"}]`, "error"} {
			t.Run(strings.Join(args, " ")+"/"+response, func(t *testing.T) {
				lookupError := errors.New("bucket list failed")
				commands := NewCommands(testRuntime(), func(siteID string) (API, error) {
					if siteID != "tky01" {
						t.Fatalf("site = %q", siteID)
					}
					return API{
						Bucket: &fakeBucketAPI{list: func(context.Context) ([]v2.BucketListDataItem, error) {
							if response == "error" {
								return nil, lookupError
							}
							var buckets []v2.BucketListDataItem
							if err := json.Unmarshal([]byte(response), &buckets); err != nil {
								t.Fatal(err)
							}
							return buckets, nil
						}},
						BucketExtra: func(string) objectstorage.BucketExtraAPI {
							t.Fatal("replication operation opened before site membership was confirmed")
							return nil
						},
					}, nil
				})
				err := runCommandError(t, commands, args)
				if err == nil {
					t.Fatal("unconfirmed bucket site was accepted")
				}
				if response == "error" && !errors.Is(err, lookupError) {
					t.Fatalf("lookup error was lost: %v", err)
				}
			})
		}
	}
}
