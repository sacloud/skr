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

package iconapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/icon"
)

type createAPITestDouble struct {
	API
	request *icon.CreateRequest
	err     error
}

func (api *createAPITestDouble) CreateWithContext(_ context.Context, request *icon.CreateRequest) (*iaas.Icon, error) {
	api.request = request
	if api.err != nil {
		return nil, api.err
	}
	return &iaas.Icon{ID: 123, Name: request.Name, Tags: request.Tags}, nil
}

func TestCreateCommandEncodesImageFileAndTags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "icon.png")
	image := []byte{0x89, 'P', 'N', 'G'}
	if err := os.WriteFile(path, image, 0o600); err != nil {
		t.Fatal(err)
	}
	api := &createAPITestDouble{}
	command := CreateCommand{
		Name:      stringPointer("test-icon"),
		ImageFile: stringPointer(path),
		Tags:      &[]string{"one", "two"},
		factory:   func() (API, error) { return api, nil },
		runtime:   testRuntime(),
	}
	if err := command.Run(&kong.Context{}); err != nil {
		t.Fatal(err)
	}
	if api.request == nil || api.request.Name != "test-icon" || api.request.Image != base64.StdEncoding.EncodeToString(image) || len(api.request.Tags) != 2 {
		t.Fatalf("CreateRequest = %+v, want file image encoded and tags set", api.request)
	}
}

func TestCreateCommandReadsJSONRequest(t *testing.T) {
	api := &createAPITestDouble{}
	request := `{"Name":"json-icon","Image":"aW1hZ2U=","Tags":["test"]}`
	command := CreateCommand{
		Request: stringPointer(request),
		factory: func() (API, error) { return api, nil },
		runtime: Runtime{
			DecodeRequest: func(input string, destination any) error {
				return json.Unmarshal([]byte(input), destination)
			},
			ValidateOutput: func(*kong.Context) error { return nil },
			WriteOutput:    func(*kong.Context, any) error { return nil },
		},
	}
	if err := command.Run(&kong.Context{}); err != nil {
		t.Fatal(err)
	}
	if api.request == nil || api.request.Name != "json-icon" || api.request.Image != "aW1hZ2U=" || api.request.Tags[0] != "test" {
		t.Fatalf("CreateRequest = %+v, want JSON fields forwarded", api.request)
	}
}

func TestCreateCommandRejectsMissingFieldsAndMixedInputs(t *testing.T) {
	for _, test := range []struct {
		name    string
		command CreateCommand
		wantErr string
	}{
		{name: "missing name", command: CreateCommand{ImageFile: stringPointer("unused")}, wantErr: "--name が必要"},
		{name: "missing image", command: CreateCommand{Name: stringPointer("test")}, wantErr: "--image-file が必要"},
		{name: "request with flags", command: CreateCommand{Request: stringPointer("{}"), Name: stringPointer("test")}, wantErr: "--request と個別フラグ"},
		{name: "request with empty tags", command: CreateCommand{Request: stringPointer("{}"), Tags: &[]string{}}, wantErr: "--request と個別フラグ"},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.command.factory = func() (API, error) {
				t.Fatal("API factory called for invalid input")
				return nil, nil
			}
			test.command.runtime = testRuntime()
			err := test.command.Run(&kong.Context{})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestCreateCommandSurfacesImageAndSDKErrors(t *testing.T) {
	t.Run("file read", func(t *testing.T) {
		command := CreateCommand{
			Name:      stringPointer("test"),
			ImageFile: stringPointer(filepath.Join(t.TempDir(), "missing.png")),
			factory: func() (API, error) {
				t.Fatal("API factory called after file read failure")
				return nil, nil
			},
			runtime: testRuntime(),
		}
		if err := command.Run(&kong.Context{}); err == nil || !strings.Contains(err.Error(), "read icon image file") {
			t.Fatalf("error = %v, want image file read error", err)
		}
	})
	t.Run("sdk", func(t *testing.T) {
		want := errors.New("API rejected icon")
		api := &createAPITestDouble{err: want}
		path := filepath.Join(t.TempDir(), "icon.png")
		if err := os.WriteFile(path, []byte("image"), 0o600); err != nil {
			t.Fatal(err)
		}
		command := CreateCommand{
			Name:      stringPointer("test"),
			ImageFile: stringPointer(path),
			factory:   func() (API, error) { return api, nil },
			runtime:   testRuntime(),
		}
		if err := command.Run(&kong.Context{}); !errors.Is(err, want) {
			t.Fatalf("error = %v, want %v", err, want)
		}
	})
}

func TestReadImageFileEncodesExactFileBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "icon.jpg")
	input := []byte{0xff, 0xd8, 0xff, 0xd9}
	if err := os.WriteFile(path, input, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readImageFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := base64.StdEncoding.EncodeToString(input); got != want {
		t.Fatalf("encoded image = %q, want %q", got, want)
	}
}

func TestFindCommandAllowsUnfilteredList(t *testing.T) {
	var got *icon.FindRequest
	api := &findAPITestDouble{find: func(request *icon.FindRequest) ([]*iaas.Icon, error) {
		got = request
		return []*iaas.Icon{}, nil
	}}
	command := FindCommand{
		factory: func() (API, error) { return api, nil },
		runtime: testRuntime(),
	}
	if err := command.Run(&kong.Context{}); err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Count != 0 || got.From != 0 {
		t.Fatalf("FindRequest = %+v, want empty request for unfiltered listing", got)
	}
}

func TestFindCommandRejectsRequestAndFlagsTogether(t *testing.T) {
	command := FindCommand{
		Request: stringPointer(`{"Names":["icon"]}`),
		Count:   intPointer(1),
		runtime: testRuntime(),
		factory: func() (API, error) {
			t.Fatal("API factory called for mixed input")
			return nil, nil
		},
	}
	if err := command.Run(&kong.Context{}); err == nil || !strings.Contains(err.Error(), "--request と個別フラグ") {
		t.Fatalf("error = %v, want mixed-input error", err)
	}
}

func TestUpdateCommandPreservesIDInSDKResponse(t *testing.T) {
	api := &updateAPITestDouble{result: &iaas.Icon{Name: "updated"}}
	var output any
	command := UpdateCommand{
		ID:   int64Pointer(123),
		Name: stringPointer("updated"),
		factory: func() (API, error) {
			return api, nil
		},
		runtime: Runtime{
			ValidateOutput: func(*kong.Context) error { return nil },
			WriteOutput: func(_ *kong.Context, value any) error {
				output = value
				return nil
			},
		},
	}
	if err := command.Run(&kong.Context{}); err != nil {
		t.Fatal(err)
	}
	if api.request == nil || api.request.ID != 123 || api.request.Name == nil || *api.request.Name != "updated" {
		t.Fatalf("UpdateRequest = %+v", api.request)
	}
	if result, ok := output.(*iaas.Icon); !ok || result.ID != 123 {
		t.Fatalf("Update output = %#v, want Icon ID restored from request", output)
	}
}

type findAPITestDouble struct {
	API
	find func(*icon.FindRequest) ([]*iaas.Icon, error)
}

func (api *findAPITestDouble) FindWithContext(_ context.Context, request *icon.FindRequest) ([]*iaas.Icon, error) {
	return api.find(request)
}

type updateAPITestDouble struct {
	API
	request *icon.UpdateRequest
	result  *iaas.Icon
}

func (api *updateAPITestDouble) UpdateWithContext(_ context.Context, request *icon.UpdateRequest) (*iaas.Icon, error) {
	api.request = request
	return api.result, nil
}

func testRuntime() Runtime {
	return Runtime{
		ValidateOutput: func(*kong.Context) error { return nil },
		WriteOutput:    func(*kong.Context, any) error { return nil },
	}
}

func stringPointer(value string) *string {
	return &value
}

func intPointer(value int) *int {
	return &value
}

func int64Pointer(value int64) *int64 {
	return &value
}
