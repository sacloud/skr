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
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	skrSaclient "github.com/sacloud/skr/internal/saclient"
)

type authenticatedHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type authenticatedHTTPDoerFactory func() (authenticatedHTTPDoer, error)

type httpCommand struct {
	URL    string   `arg:"" name:"url" help:"呼び出す HTTPS URL。指定したホストへ SDK の認証情報が送信されます。"`
	Method string   `name:"method" short:"X" default:"GET" help:"HTTP メソッド。省略時は GET です。例: PATCH"`
	Data   *string  `name:"data" short:"d" help:"リクエスト本文。文字列、@ファイルパス、または -（標準入力）を指定します。秘密情報を含む本文はファイルまたは標準入力で渡してください。"`
	Header []string `name:"header" short:"H" help:"追加する HTTP ヘッダー。'名前: 値' の形式で複数指定できます。"`

	doerFactory authenticatedHTTPDoerFactory
}

func (c *cli) initHTTP() {
	c.HTTP.doerFactory = func() (authenticatedHTTPDoer, error) {
		return newAuthenticatedHTTPDoer(c.Trace)
	}
}

func newAuthenticatedHTTPDoer(trace bool) (authenticatedHTTPDoer, error) {
	client, err := skrSaclient.New(trace)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (c *httpCommand) Run(ctx *kong.Context) error {
	if err := validateHTTPSURL(c.URL); err != nil {
		return err
	}

	var body io.Reader
	if c.Data != nil {
		data, err := readHTTPData(*c.Data)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	method := c.Method
	if method == "" {
		method = http.MethodGet
	}
	request, err := http.NewRequestWithContext(context.Background(), method, c.URL, body)
	if err != nil {
		return fmt.Errorf("create HTTP request: %w", err)
	}
	if err := addHTTPHeaders(request, c.Header); err != nil {
		return err
	}

	if c.doerFactory == nil {
		return fmt.Errorf("HTTP client is not configured")
	}
	doer, err := c.doerFactory()
	if err != nil {
		return fmt.Errorf("initialize authenticated HTTP client: %w", err)
	}
	if doer == nil {
		return fmt.Errorf("initialize authenticated HTTP client: received an empty client")
	}
	response, err := doer.Do(request)
	if err != nil {
		return fmt.Errorf("send HTTP request: %w", err)
	}
	if response == nil {
		return fmt.Errorf("send HTTP request: received an empty response")
	}
	if response.Body == nil {
		return fmt.Errorf("read HTTP response: received an empty body")
	}

	_, copyErr := io.Copy(ctx.Stdout, response.Body)
	closeErr := response.Body.Close()
	if copyErr != nil {
		return fmt.Errorf("write HTTP response body: %w", errors.Join(copyErr, closeErr))
	}
	if closeErr != nil {
		return fmt.Errorf("close HTTP response body: %w", closeErr)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		status := response.Status
		if status == "" {
			status = http.StatusText(response.StatusCode)
		}
		return fmt.Errorf("HTTP request returned %s", status)
	}
	return nil
}

func validateHTTPSURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse HTTPS URL: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" || parsed.Opaque != "" {
		return fmt.Errorf("URL must be an absolute HTTPS URL with a host")
	}
	if parsed.User != nil {
		return fmt.Errorf("URL must not contain user information")
	}
	if parsed.Fragment != "" || strings.Contains(rawURL, "#") {
		return fmt.Errorf("URL must not contain a fragment")
	}
	return nil
}

func readHTTPData(input string) ([]byte, error) {
	switch {
	case input == "-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read HTTP request body from standard input: %w", err)
		}
		return data, nil
	case strings.HasPrefix(input, "@"):
		path := strings.TrimPrefix(input, "@")
		//nolint:gosec // The path is explicitly supplied by the user as request data.
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read HTTP request body file: %w", err)
		}
		return data, nil
	default:
		return []byte(input), nil
	}
}

func addHTTPHeaders(request *http.Request, headers []string) error {
	for _, value := range headers {
		name, headerValue, ok := strings.Cut(value, ":")
		if !ok {
			return fmt.Errorf("invalid --header %q: use 'name: value' format", value)
		}
		name = strings.TrimSpace(name)
		headerValue = strings.TrimSpace(headerValue)
		if !validHTTPHeaderName(name) {
			return fmt.Errorf("invalid --header %q: header name is not valid", value)
		}
		if strings.ContainsAny(headerValue, "\r\n") {
			return fmt.Errorf("invalid --header %q: header value must not contain a newline", value)
		}
		request.Header.Add(name, headerValue)
	}
	return nil
}

func validHTTPHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		char := name[i]
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			strings.ContainsRune("!#$%&'*+-.^_`|~", rune(char)) {
			continue
		}
		return false
	}
	return true
}
