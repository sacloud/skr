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

import "testing"

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
