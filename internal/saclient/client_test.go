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

package saclient

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	sacloudsdk "github.com/sacloud/sacloud-sdk-go"
	saclient "github.com/sacloud/sacloud-sdk-go/common/saclient"
	"github.com/sacloud/skr/version"
)

func TestNewTraceMode(t *testing.T) {
	t.Setenv("SAKURACLOUD_TRACE", "error")

	client, err := New(false)
	if err != nil {
		t.Fatal(err)
	}
	if got := client.JSON()["TraceMode"]; got != "error" {
		t.Errorf("TraceMode without --trace = %v, want error from environment", got)
	}

	client, err = New(true)
	if err != nil {
		t.Fatal(err)
	}
	if got := client.JSON()["TraceMode"]; got != "all" {
		t.Errorf("TraceMode with --trace = %v, want all", got)
	}
}

func TestNewUserAgent(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "user-agent-test-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "user-agent-test-secret")

	var got string
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		got = request.UserAgent()
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := New(false)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SetWith(
		saclient.WithTestServer(server),
		saclient.WithUserAgent("iam-api-go/v0.3.0"),
	); err != nil {
		t.Fatal(err)
	}

	request, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if want := "skr/v" + version.Version + " (" + runtime.GOOS + "/" + runtime.GOARCH + "; sacloud-sdk-go/v" + sacloudsdk.Version + ")"; got != want {
		t.Errorf("User-Agent = %q, want %q", got, want)
	}
}

func TestHTTPUserAgent(t *testing.T) {
	want := "skr-http/v" + version.Version + " (" + runtime.GOOS + "/" + runtime.GOARCH + "; sacloud-sdk-go/v" + sacloudsdk.Version + ")"
	if got := HTTPUserAgent(); got != want {
		t.Errorf("HTTPUserAgent() = %q, want %q", got, want)
	}
}
