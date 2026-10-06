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

package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sacloud/sacloud-sdk-go/common/saclient"
)

type authenticatedHTTPDoerFunc func(*http.Request) (*http.Response, error)

func (f authenticatedHTTPDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestRunHTTPCommand(t *testing.T) {
	requestBody := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(requestBody, []byte(`{"name":"sample"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var gotMethod, gotURL, gotContentType, gotCustomHeader, gotBody string
	commandLine := newCLI()
	commandLine.HTTP.doerFactory = func() (authenticatedHTTPDoer, error) {
		return authenticatedHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
			gotMethod = request.Method
			gotURL = request.URL.String()
			gotContentType = request.Header.Get("Content-Type")
			gotCustomHeader = request.Header.Get("X-Feature")
			body, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			gotBody = string(body)
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("{\"id\":\"123\"}\n")),
			}, nil
		}), nil
	}

	var stdout, stderr bytes.Buffer
	args := []string{
		"http",
		"https://api.example.test/v1/resources?limit=2",
		"--method", "POST",
		"-H", "Content-Type: application/json",
		"--header", "X-Feature: enabled",
		"--data", "@" + requestBody,
		"--output", "table",
	}
	if code := runCLI(args, &stdout, &stderr, commandLine); code != 0 {
		t.Fatalf("runCLI() = %d; stderr: %s", code, stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("request method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotURL != "https://api.example.test/v1/resources?limit=2" {
		t.Errorf("request URL = %q", gotURL)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotCustomHeader != "enabled" {
		t.Errorf("X-Feature = %q, want enabled", gotCustomHeader)
	}
	if gotBody != `{"name":"sample"}` {
		t.Errorf("request body = %q", gotBody)
	}
	if got, want := stdout.String(), "{\"id\":\"123\"}\n"; got != want {
		t.Errorf("response body = %q, want unchanged body %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty", got)
	}
}

func TestRunHTTPCommandReturnsErrorForUnsuccessfulStatus(t *testing.T) {
	commandLine := newCLI()
	commandLine.HTTP.doerFactory = func() (authenticatedHTTPDoer, error) {
		return authenticatedHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Status:     "502 Bad Gateway",
				Body:       io.NopCloser(strings.NewReader("upstream unavailable")),
			}, nil
		}), nil
	}

	var stdout, stderr bytes.Buffer
	if code := runCLI([]string{"http", "https://api.example.test/resource"}, &stdout, &stderr, commandLine); code == 0 {
		t.Fatal("runCLI() = 0, want non-zero for an unsuccessful HTTP status")
	}
	if got, want := stdout.String(), "upstream unavailable"; got != want {
		t.Errorf("response body = %q, want %q", got, want)
	}
	if !strings.Contains(stderr.String(), "502 Bad Gateway") {
		t.Errorf("stderr = %q, want HTTP status", stderr.String())
	}
}

func TestRunHTTPCommandRejectsInvalidURLBeforeInitializingClient(t *testing.T) {
	commandLine := newCLI()
	called := false
	commandLine.HTTP.doerFactory = func() (authenticatedHTTPDoer, error) {
		called = true
		return nil, nil
	}

	for _, rawURL := range []string{
		"http://api.example.test/resource",
		"https:///resource",
		"https://user:password@api.example.test/resource",
		"https://api.example.test/resource#section",
	} {
		var stdout, stderr bytes.Buffer
		if code := runCLI([]string{"http", rawURL}, &stdout, &stderr, commandLine); code == 0 {
			t.Errorf("runCLI(%q) = 0, want non-zero", rawURL)
		}
	}
	if called {
		t.Fatal("HTTP client factory was called for an invalid URL")
	}
}

func TestAddHTTPHeadersValidatesInput(t *testing.T) {
	for _, header := range []string{
		"MissingSeparator",
		"Invalid Name: value",
		"X-Feature: first\r\nInjected: second",
	} {
		request, err := http.NewRequest(http.MethodGet, "https://api.example.test", nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := addHTTPHeaders(request, []string{header}); err == nil {
			t.Errorf("addHTTPHeaders(%q) succeeded, want an error", header)
		}
	}
}

func TestReadHTTPData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "body.bin")
	wantFile := []byte{0, 1, 2, 255}
	if err := os.WriteFile(path, wantFile, 0o600); err != nil {
		t.Fatal(err)
	}
	gotFile, err := readHTTPData("@" + path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotFile, wantFile) {
		t.Errorf("file data = %v, want %v", gotFile, wantFile)
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	originalStdin := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = originalStdin
		if err := reader.Close(); err != nil {
			t.Errorf("close standard input pipe: %v", err)
		}
	})
	wantStdin := []byte("stdin body")
	if _, err := writer.Write(wantStdin); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	gotStdin, err := readHTTPData("-")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotStdin, wantStdin) {
		t.Errorf("standard input data = %q, want %q", gotStdin, wantStdin)
	}
}

func TestHTTPCommandUsesSDKAuthentication(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "http-test-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "http-test-secret")

	var authorization string
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	commandLine := newCLI()
	commandLine.HTTP.doerFactory = func() (authenticatedHTTPDoer, error) {
		var client saclient.Client
		if err := client.SetEnviron(os.Environ()); err != nil {
			return nil, err
		}
		if err := client.SetWith(saclient.WithTestServer(server)); err != nil {
			return nil, err
		}
		return &client, nil
	}

	var stdout, stderr bytes.Buffer
	if code := runCLI([]string{"http", server.URL}, &stdout, &stderr, commandLine); code != 0 {
		t.Fatalf("runCLI() = %d; stderr: %s", code, stderr.String())
	}
	if authorization == "" {
		t.Fatal("request has no Authorization header")
	}
	if strings.Contains(authorization, "http-test-secret") {
		t.Fatal("request exposed the access token secret")
	}
}

func TestRunHTTPCommandHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"http", "--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d; stderr: %s", code, stderr.String())
	}
	for _, text := range []string{
		"--method",
		"--data",
		"--header",
		"-H",
		"HTTPS",
		"標準入力",
		"SDK の認証情報",
		"--output による変換は行いません",
	} {
		if !strings.Contains(stdout.String(), text) {
			t.Errorf("http help does not contain %q", text)
		}
	}
}
