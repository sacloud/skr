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

package iaas

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

// httpBridge はインメモリの Server を HTTP で公開するブリッジです。
// チュートリアル実行 harness は skr をサブプロセスで実行するため、モックの状態を
// プロセス間で共有するには HTTP 経由にする必要があります。
type httpBridge struct {
	inner *Server
}

// NewTestHTTPServer はインメモリの Server を HTTP で公開するテストサーバーを起動します。
// レスポンスとエラーは Do の戻り値をそのまま伝えます。
func NewTestHTTPServer(inner *Server) *httptest.Server {
	return httptest.NewServer(&httpBridge{inner: inner})
}

// ServeHTTP は HTTP リクエストを inner.Do の呼び出しに変換します。
// リクエスト本文は JSON として渡し、エラーは 400 とエラーメッセージ本文で返します。
func (b *httpBridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("read IaaS request body: %v", err), http.StatusBadRequest)
		return
	}
	var body any
	if len(bytes.TrimSpace(raw)) > 0 {
		// Do の実装は任意の値を JSON に変換するため、生の JSON をそのまま渡す。
		body = json.RawMessage(raw)
	}
	data, err := b.inner.Do(r.Context(), r.Method, r.URL.RequestURI(), body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(data) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// 書き込みエラーはクライアント側で切断として検出できるため無視する。
	_, _ = w.Write(data) //nolint:gosec // モックサーバーが生成した JSON を書き込むだけ。
}

// remoteCaller は HTTP ブリッジ経由で Server.Do を呼び出す iaas.APICaller 実装です。
type remoteCaller struct {
	baseURL string
	client  *http.Client
}

// NewRemoteCaller は HTTP ブリッジに接続する iaas.APICaller を返します。
func NewRemoteCaller(baseURL string) (*remoteCaller, error) {
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return nil, fmt.Errorf("invalid IaaS mock URL: %s", baseURL)
	}
	return &remoteCaller{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client:  &http.Client{}, // タイムアウトは呼び出し側の context に委ねる。
	}, nil
}

// Do は HTTP リクエストをブリッジに送り、Server.Do と同じ契約で応答を返します。
func (c *remoteCaller) Do(ctx context.Context, method, uri string, body any) ([]byte, error) {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode IaaS request body: %w", err)
		}
		payload = bytes.NewReader(data)
	}
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+uri, payload)
	if err != nil {
		return nil, fmt.Errorf("build IaaS mock request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send IaaS mock request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // 読み込み済みのレスポンス本文のクローズエラーは無視する。
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read IaaS mock response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(data)))
	}
	return data, nil
}
