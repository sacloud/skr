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

//go:build unix

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/common/saclient"
	skrSaclient "github.com/sacloud/skr/internal/saclient"
	"github.com/stretchr/testify/require"
)

// sendSelfSignal はテストプロセス自身へシグナルを送ります。
func sendSelfSignal(t *testing.T, sig os.Signal) {
	t.Helper()
	process, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	require.NoError(t, process.Signal(sig))
}

// newTestServerDoerFactory はテストサーバーへ接続する SDK の認証付きクライアントを返す
// doerFactory を構築します。
func newTestServerDoerFactory(t *testing.T, server *httptest.Server) authenticatedHTTPDoerFactory {
	t.Helper()
	return func() (authenticatedHTTPDoer, error) {
		client, err := skrSaclient.NewHTTP(false)
		if err != nil {
			return nil, err
		}
		if err := client.SetWith(saclient.WithTestServer(server)); err != nil {
			return nil, err
		}
		return client, nil
	}
}

func TestRunHTTPCommandInterruptsSlowResponseBodyOnSignal(t *testing.T) {
	// プロファイルや環境変数による設定差異の影響を受けないよう、一時プロファイル
	// ディレクトリに切り離す。
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())

	// 応答の先頭を送った後、接続を閉じずに待つサーバーを用意する。本文の読み込み中に
	// SIGINT がリクエストへ伝播し、応答を待たずにエラーで終了することを確認する。
	block := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(http.StatusOK)
		if flusher, ok := writer.(http.Flusher); ok {
			_, _ = writer.Write([]byte("partial"))
			flusher.Flush()
		}
		<-block
	}))
	t.Cleanup(func() {
		close(block)
		server.Close()
	})

	commandLine := newCLI()
	commandLine.HTTP.doerFactory = newTestServerDoerFactory(t, server)

	done := make(chan int, 1)
	var stdout, stderr bytes.Buffer
	go func() {
		done <- runCLI([]string{"http", server.URL}, &stdout, &stderr, commandLine)
	}()

	// 応答本文の先頭が届くのを待ってから SIGINT を送る。
	deadline := time.Now().Add(30 * time.Second)
	for !strings.Contains(stdout.String(), "partial") {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the response body")
		}
		time.Sleep(10 * time.Millisecond)
	}
	sendSelfSignal(t, syscall.SIGINT)

	select {
	case code := <-done:
		require.NotEqual(t, 0, code, "run should fail after SIGINT")
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for runCLI to return")
	}
	require.Contains(t, stderr.String(), "interrupted by signal", "stderr = %q, want interrupted", stderr.String())
}

func TestRunHTTPCommandInterruptsRetryWaitOnSignal(t *testing.T) {
	// プロファイルの RetryMax=0 などの設定に影響されないよう、一時プロファイル
	// ディレクトリに切り離す。既定のリトライ回数 (SDK 既定) でリトライが発生する。
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	// 500 を返すサーバーに対して SDK がリトライする。リトライの待ち時間を長くし、
	// 待ち時間中に SIGINT を送ると、待ち時間を待たずに中断されることを確認する。
	t.Setenv("SAKURA_RETRY_WAIT_MIN", "60")
	t.Setenv("SAKURA_RETRY_WAIT_MAX", "60")

	var attempts int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		atomic.AddInt32(&attempts, 1)
		http.Error(writer, "server error", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	commandLine := newCLI()
	commandLine.HTTP.doerFactory = newTestServerDoerFactory(t, server)

	done := make(chan int, 1)
	var stdout, stderr bytes.Buffer
	go func() {
		done <- runCLI([]string{"http", server.URL}, &stdout, &stderr, commandLine)
	}()

	// 1 回目の試行が終わるのを待ってから SIGINT を送る。
	deadline := time.Now().Add(30 * time.Second)
	for atomic.LoadInt32(&attempts) < 1 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the first request attempt")
		}
		time.Sleep(10 * time.Millisecond)
	}
	sendSelfSignal(t, syscall.SIGINT)

	start := time.Now()
	select {
	case code := <-done:
		require.NotEqual(t, 0, code, "run should fail after SIGINT")
		require.Less(t, time.Since(start), 60*time.Second, "run waited for the retry backoff")
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for runCLI to return")
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&attempts), "attempts = %d, want 1 (no retry after SIGINT)", atomic.LoadInt32(&attempts))
	require.Contains(t, stderr.String(), "interrupted by signal", "stderr = %q, want interrupted", stderr.String())
}
