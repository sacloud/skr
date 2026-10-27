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
	skrSaclient "github.com/sacloud/skr/internal/saclient"
	"github.com/stretchr/testify/require"
)

type authenticatedHTTPDoerFunc func(*http.Request) (*http.Response, error)

func (f authenticatedHTTPDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestRunHTTPCommand(t *testing.T) {
	requestBody := filepath.Join(t.TempDir(), "request.json")
	err := os.WriteFile(requestBody, []byte(`{"name":"sample"}`), 0o600)
	require.NoError(t, err)

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
		"--query", ".id",
	}
	require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
	require.Equal(t, http.MethodPost, gotMethod, "request method = %q, want %q", gotMethod, http.MethodPost)
	require.Equal(t, "https://api.example.test/v1/resources?limit=2", gotURL, "request URL = %q", gotURL)
	require.Equal(t, "application/json", gotContentType, "Content-Type = %q, want application/json", gotContentType)
	require.Equal(t, "enabled", gotCustomHeader, "X-Feature = %q, want enabled", gotCustomHeader)
	require.Equal(t, `{"name":"sample"}`, gotBody, "request body = %q", gotBody)
	if got, want := stdout.String(), "{\"id\":\"123\"}\n"; got != want {
		t.Errorf("response body = %q, want unchanged body %q", got, want)
	}
	got := stderr.String()
	require.Empty(t, got, "stderr = %q, want empty", got)
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
	require.NotEqual(t, 0, runCLI([]string{"http", "https://api.example.test/resource"}, &stdout, &stderr, commandLine), "run should fail")
	got, want := stdout.String(), "upstream unavailable"
	require.Equal(t, want, got, "response body = %q, want %q")
	require.Contains(t, stderr.String(), "502 Bad Gateway", "stderr = %q, want HTTP status", stderr.String())
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
		require.NotEqual(t, 0, runCLI([]string{"http", rawURL}, &stdout, &stderr, commandLine), "run should fail")
	}
	require.False(t, called, "HTTP client factory was called for an invalid URL")
}

func TestAddHTTPHeadersValidatesInput(t *testing.T) {
	for _, header := range []string{
		"MissingSeparator",
		"Invalid Name: value",
		"X-Feature: first\r\nInjected: second",
	} {
		request, err := http.NewRequest(http.MethodGet, "https://api.example.test", nil)
		require.NoError(t, err)
		err = addHTTPHeaders(request, []string{header})
		require.Error(t, err, "addHTTPHeaders(%q) succeeded, want an error", header)
	}
}

func TestReadHTTPData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "body.bin")
	wantFile := []byte{0, 1, 2, 255}
	err := os.WriteFile(path, wantFile, 0o600)
	require.NoError(t, err)
	gotFile, err := readHTTPData("@" + path)
	require.NoError(t, err)
	require.Equal(t, wantFile, gotFile, "file data = %v, want %v", gotFile, wantFile)

	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	originalStdin := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = originalStdin
		err2 := reader.Close()
		require.NoError(t, err2, "close standard input pipe: %v")
	})
	wantStdin := []byte("stdin body")
	if _, err := writer.Write(wantStdin); err != nil {
		t.Fatal(err)
	}
	err3 := writer.Close()
	require.NoError(t, err3)
	gotStdin, err := readHTTPData("-")
	require.NoError(t, err)
	require.Equal(t, wantStdin, gotStdin, "standard input data = %q, want %q", gotStdin, wantStdin)
}

func TestHTTPCommandUsesSDKAuthenticationAndUserAgent(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	t.Setenv("SAKURA_ACCESS_TOKEN", "http-test-token")
	t.Setenv("SAKURA_ACCESS_TOKEN_SECRET", "http-test-secret")

	for _, test := range []struct {
		name             string
		args             []string
		wantAgent        string
		wantAgentPresent bool
	}{
		{
			name:             "default",
			wantAgent:        skrSaclient.HTTPUserAgent(),
			wantAgentPresent: true,
		},
		{
			name:             "explicit override",
			args:             []string{"--header", "User-Agent: custom-agent/1.0"},
			wantAgent:        "custom-agent/1.0",
			wantAgentPresent: true,
		},
		{
			name:      "empty override",
			args:      []string{"--header", "User-Agent:"},
			wantAgent: "",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var authorization, userAgent string
			var userAgentPresent bool
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				authorization = request.Header.Get("Authorization")
				userAgent = request.UserAgent()
				_, userAgentPresent = request.Header["User-Agent"]
				writer.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)

			commandLine := newCLI()
			commandLine.HTTP.doerFactory = func() (authenticatedHTTPDoer, error) {
				client, err := skrSaclient.NewHTTP(false)
				if err != nil {
					return nil, err
				}
				if err := client.SetWith(saclient.WithTestServer(server)); err != nil {
					return nil, err
				}
				return client, nil
			}

			args := append([]string{"http", server.URL}, test.args...)
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, runCLI(args, &stdout, &stderr, commandLine), "stderr: %s", stderr.String())
			require.NotEmpty(t, authorization, "request has no Authorization header")
			require.Equal(t, test.wantAgent, userAgent, "User-Agent = %q, want %q", userAgent, test.wantAgent)
			require.Equal(t, test.wantAgentPresent, userAgentPresent, "User-Agent present = %t, want %t", userAgentPresent, test.wantAgentPresent)
			require.NotContains(t, authorization, "http-test-secret", "request exposed the access token secret")
		})
	}
}

func TestRunHTTPCommandHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run([]string{"http", "--help"}, &stdout, &stderr), "stderr: %s", stderr.String())
	for _, text := range []string{
		"--method",
		"--data",
		"--header",
		"-H",
		"HTTPS",
		"標準入力",
		"SDK の認証情報",
		"--query による加工は行いません",
	} {
		require.Contains(t, stdout.String(), text, "http help does not contain %q", text)
	}
}
