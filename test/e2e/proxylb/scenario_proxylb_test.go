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
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	diskSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/disk"
	proxylbSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/proxylb"
	serverSDK "github.com/sacloud/sacloud-sdk-go/service/iaas/server"
	iaasclient "github.com/sacloud/skr/internal/iaas/client"
	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

// proxylbNamePrefix はテスト用リソースの名前の接頭辞です。ランダムな接尾辞とともに使い、
// 過去の実行のリソースと衝突しないようにします。
const proxylbNamePrefix = "skr-e2e-proxylb-"

// serverDescription は配下サーバー用のテスト Server に付ける固定の説明です。後片付けの識別にも使います。
const serverDescription = "Temporary ProxyLB health-check server created by skr live E2E"

const (
	// proxylbWaitTimeout は ProxyLB の準備待ち (DNS 反映と稼働状態) の期限です。最大 10 分待ちます。
	proxylbWaitTimeout = 10 * time.Minute
	// proxylbPolling は ProxyLB の再確認間隔です。
	proxylbPolling = 10 * time.Second
	// curlCommandTimeout は curl 1回のタイムアウトです。
	curlCommandTimeout = 20 * time.Second
)

// backendResponse は配下サーバーの cloud-init で用意する応答本文です。
const backendResponse = "skr-e2e-proxylb-backend"

// proxylbCertCN は自己署名証明書の CN に使う検証用ホスト名です。
// FQDN は API が非同期に割り当てるため、証明書の CN には固定値を使います。
const proxylbCertCN = "proxylb.example.test"

// TestScenarioProxyLB は ProxyLB E2E です。配下サーバー用の Server を1台作成し、
// plan 100 の ProxyLB を作成してリッスンポートと配下サーバーを登録します。
// VIP 宛ての実際の HTTP GET で配下サーバーへ到達すること、health-status の稼働状態、
// 接続数モニタリング、自己署名証明書の設定・取得・削除、更新と
// Let's Encrypt 未設定時の renew のエラーを確認します。
// FQDN は API が非同期に割り当てるため、作成直後の応答には含まれません。トラフィックの確認は
// 作成直後に割り当てられる VIP で行います。
// テスト対象の操作は skr CLI と curl で実行し、事前確認と後片付けの確認と
// 起動待ちのポーリングは SDK で静かに実行します。途中の失敗でも後片付けは実行され、
// その失敗はテストの結果に含まれます。
func TestScenarioProxyLB(t *testing.T) {
	h := helper.New(t, "proxylb-api", "ProxyLB の作成・参照・更新・削除と、VIP 宛ての実際の HTTP リクエスト、稼働状態、証明書の設定・取得・削除が正しく動作することを確認する")

	// openssl は証明書の生成に必要です
	if _, err := exec.LookPath("openssl"); err != nil {
		require.NoError(t, err, "openssl is required for the ProxyLB live E2E")
	}
	// curl は VIP 宛ての HTTP リクエストに必要です
	if _, err := exec.LookPath("curl"); err != nil {
		require.NoError(t, err, "curl is required for the ProxyLB live E2E")
	}

	// 事前確認と後片付けの確認は SDK で静かに実行します。skr CLI と同じ認証設定 (プロファイル) を使います
	client, err := iaasclient.New(false)
	require.NoError(t, err, "create IaaS API client for verification")
	servers, disks := serverSDK.New(client), diskSDK.New(client)
	proxylbs := proxylbSDK.New(client)

	// cloud-init アーカイブとディスクプランを SDK で自動選択します
	params := helper.RequireServerDiskParams(t, client)

	// 配下サーバー用の cloud-init の user-data を作成します。80 番ポートで応答する Web サーバーを起動します
	material := helper.WriteSSHMaterialWithRuncmd(t, helper.ServerSSHUser, false,
		fmt.Sprintf("mkdir -p /tmp/proxylb-web && echo %s > /tmp/proxylb-web/index.html", backendResponse),
		"nohup python3 -m http.server 80 --directory /tmp/proxylb-web >/var/log/proxylb-web.log 2>&1 &")

	name := proxylbNamePrefix + helper.RandomSuffix(t)
	serverName := name + "-server"

	// 作成前に Server、Disk、ProxyLB の同名リソースが存在しないことを SDK で確認する。
	// 確認の実行は証跡に残らず、後片付けはまだ登録せず、既存リソースを変更しないまま終了する。
	helper.RequireNoServerNameCollision(t, servers, disks, serverName)
	requireNoProxylbNameCollision(t, proxylbs, name)

	// 後片付け: Server と ProxyLB を識別情報の再照合つきで削除する。
	// defer の呼び出し順 (LIFO) により、ProxyLB の削除が先に実行されます
	var serverID, proxylbID types.ID
	// knownDisks には作成直後に確認した Disk の ID を保持し、後片付けの --with-disks 前の再照合に使います
	var knownDisks []types.ID
	defer func() {
		if err := helper.CleanupServerResources(t, h, servers, disks, params, serverID, []helper.ServerIdentity{
			{Name: serverName, Description: serverDescription},
		}, knownDisks...); err != nil {
			t.Errorf("server cleanup failed: %v", err)
		}
	}()
	defer func() {
		if err := cleanupProxylb(t, h, proxylbs, name, proxylbID); err != nil {
			t.Errorf("ProxyLB cleanup failed: %v", err)
		}
	}()

	// 配下サーバーを cloud-init ディスク付きで作成し、user-data を指定して起動して SSH ログインを確認します
	server := helper.CreateCloudInitServer(t, h, disks, serverName, serverDescription, params, "shared")
	serverID = server.ID
	// 作成した Disk の ID を保持します。ヘルパーが作成直後に所属と指定内容を確認済みです
	createdServer, err := helper.FetchServer(t, servers, serverID)
	require.NoError(t, err, "read the created Server to keep the created Disk ID")
	require.Len(t, createdServer.Disks, 1, "the created Server should have the created Disk")
	knownDisks = []types.ID{createdServer.Disks[0].ID}
	helper.BootServerWithUserData(t, h, servers, serverID, material.UserDataPath)
	helper.WaitForServerSSH(t, h, servers, serverID, serverName, serverDescription, material)
	serverIP := sharedIPOfRunningServer(t, servers, serverID)

	// テスト用の ProxyLB を作成します。plan 100 を指定し、リッスンポートと配下サーバーを登録します
	data, _, err := h.CallAllowingFailure(t, "テスト用の ProxyLB を作成する",
		"proxylb-api", "create",
		"--name", name, "--description", proxylbDescription,
		"--plan", "100", "--region", "anycast",
		"--health-check-protocol", "http", "--health-check-delay-loop", "10",
		"--health-check-path", "/",
		"--bind-port", "proxy-mode=http,port=80",
		"--bind-port", "proxy-mode=https,port=443",
		"--server", "ip-address="+serverIP+",port=80,enabled=true",
		"--tags", "skr-e2e",
	)
	require.NoError(t, err, "create ProxyLB")
	created, err := parseProxylb(data)
	require.NoError(t, err)
	proxylbID = created.ID
	require.NoError(t, checkProxylbIdentity(created, proxylbID, name, proxylbDescription),
		"the create response should contain the created ProxyLB")
	verifyProxylbSettings(t, created, proxylbDescription, 10, serverIP)
	require.NotEmpty(t, created.VirtualIP, "the create response should contain the assigned VIP")

	// ProxyLB find が作成した ProxyLB を見つけられることを確認します
	found := findProxylbs(t, h, name)
	require.Len(t, found, 1, "proxylb-api find --name should return only the created ProxyLB")
	require.NoError(t, checkProxylbIdentity(found[0], proxylbID, name, proxylbDescription),
		"proxylb-api find should return the created ProxyLB")

	// ProxyLB read が作成済みの ProxyLB を返すことを確認します
	read := readProxylb(t, h, proxylbID)
	require.NoError(t, checkProxylbIdentity(read, proxylbID, name, proxylbDescription),
		"proxylb-api read should return the created ProxyLB")
	verifyProxylbSettings(t, read, proxylbDescription, 10, serverIP)

	// 配下サーバーの稼働状態が up になるまで待ちます
	waitForProxyLBServerStatus(t, h, proxylbID, serverIP)

	// VIP 宛ての実際の HTTP GET で配下サーバーへ到達することを確認します
	waitForProxyLBHTTP(t, h, created.VirtualIP, backendResponse)

	// 接続数モニタリングを取得します。現在時刻を終端とする観測期間を指定します
	getMonitorConnection(t, h, proxylbID)

	// 自己署名証明書を生成して設定し、応答と実際の HTTPS リクエストを確認します。
	// FQDN は作成直後の応答には含まれないため、証明書の CN は固定の検証用ホスト名を使います
	cert := generateSelfSignedCertificate(t, h, proxylbCertCN)
	_, _, err = h.CallAllowingFailure(t, "ProxyLB に自己署名証明書を設定する",
		"proxylb-api", "set-certificates", "--id", proxylbID.String(),
		"--primary-cert", "@"+cert.CertPath,
		"--primary-key", "@"+cert.KeyPath,
	)
	require.NoError(t, err, "set self-signed certificate")
	// 証明書モデルには秘密鍵も含まれるため、応答を共通名だけに絞り込み、実行記録に秘密鍵が残らないようにします
	cn := getCertificateCommonName(t, h, proxylbID)
	require.Equal(t, proxylbCertCN, cn,
		"the certificate common name should match the requested CN")
	waitForProxyLBHTTPS(t, h, created.VirtualIP, backendResponse)

	// 設定した証明書を削除します
	_, _, err = h.CallAllowingFailure(t, "ProxyLB の証明書を削除する",
		"proxylb-api", "delete-certificates", "--id", proxylbID.String(),
	)
	require.NoError(t, err, "delete certificate")

	// ProxyLB の説明と監視間隔を更新して、応答で反映を確認します。監視設定は置き換わるためパスも再指定します
	data, _, err = h.CallAllowingFailure(t, "ProxyLB の説明と監視間隔を更新する",
		"proxylb-api", "update", "--id", proxylbID.String(),
		"--description", updatedProxylbDescription,
		"--health-check-protocol", "http", "--health-check-delay-loop", "20",
		"--health-check-path", "/",
	)
	require.NoError(t, err, "update ProxyLB")
	updated, err := parseProxylb(data)
	require.NoError(t, err)
	require.NoError(t, checkProxylbIdentity(updated, proxylbID, name, updatedProxylbDescription),
		"the update should be reflected in the response")
	verifyProxylbSettings(t, updated, updatedProxylbDescription, 20, serverIP)

	// ProxyLB read が更新済みの ProxyLB を返すことを確認します
	read = readProxylb(t, h, proxylbID)
	require.NoError(t, checkProxylbIdentity(read, proxylbID, name, updatedProxylbDescription),
		"proxylb-api read should return the updated ProxyLB")
	verifyProxylbSettings(t, read, updatedProxylbDescription, 20, serverIP)

	// Let's Encrypt が未設定の ProxyLB に対する renew は API エラーになることを確認します。
	// 失敗応答は異常系の確認であり、実行結果は証跡に残ります
	_, stderr, err := h.CallAllowingFailure(t, "Let's Encrypt 未設定の ProxyLB で renew-lets-encrypt-cert がエラーになることを確認する",
		"proxylb-api", "renew-lets-encrypt-cert", "--id", proxylbID.String(),
	)
	require.Error(t, err, "renew-lets-encrypt-cert should fail without Let's Encrypt settings")
	require.NotEmpty(t, stderr, "the failure should report an error message")
}

// sharedIPOfRunningServer は SDK で Server を参照し、共有 NIC の IPv4 アドレスを返します。
func sharedIPOfRunningServer(t *testing.T, servers *serverSDK.Service, id types.ID) string {
	t.Helper()

	item, err := helper.FetchServer(t, servers, id)
	require.NoError(t, err, "read the running Server")
	ip := helper.SharedIPOfServer(item)
	require.NotEmpty(t, ip, "the running Server should have a shared IPv4 address")
	return ip
}

// verifyProxylbSettings は ProxyLB の応答が期待する設定 (説明・監視間隔・リッスンポート・配下サーバー) と
// 一致することを確認します。FQDN は API が非同期に割り当てるため、この確認では扱いません。
func verifyProxylbSettings(t *testing.T, item proxylbItem, description string, delayLoop int, serverIP string) {
	t.Helper()

	require.Equal(t, description, item.Description, "the ProxyLB description should match")
	require.Equal(t, 100, item.Plan, "the ProxyLB should use plan 100")
	require.NotNil(t, item.HealthCheck, "the ProxyLB should have a health check")
	require.Equal(t, delayLoop, item.HealthCheck.DelayLoop, "the ProxyLB should use the requested delay loop")
	require.Equal(t, types.ProxyLBProtocols.HTTP, item.HealthCheck.Protocol,
		"the ProxyLB should use the http health check")
	require.Len(t, item.BindPorts, 2, "the ProxyLB should have http and https listen ports")
	require.Equal(t, types.ProxyLBProxyModes.HTTP, item.BindPorts[0].ProxyMode,
		"the first listen port should be http")
	require.EqualValues(t, 80, item.BindPorts[0].Port, "the http listen port should be 80")
	require.Equal(t, types.ProxyLBProxyModes.HTTPS, item.BindPorts[1].ProxyMode,
		"the second listen port should be https")
	require.EqualValues(t, 443, item.BindPorts[1].Port, "the https listen port should be 443")
	require.Len(t, item.Servers, 1, "the ProxyLB should have exactly one backend server")
	require.Equal(t, serverIP, item.Servers[0].IPAddress,
		"the ProxyLB backend should be the created Server")
	require.EqualValues(t, 80, item.Servers[0].Port, "the ProxyLB backend port should be 80")
}

// proxylbMonitorWindow は現在時刻を終端とする観測期間の長さです。
const proxylbMonitorWindow = 10 * time.Minute

// monitorConnectionValue は接続数モニタリングの要素です。
type monitorConnectionValue struct {
	Time              time.Time `json:"Time"`
	ActiveConnections float64   `json:"ActiveConnections"`
	ConnectionsPerSec float64   `json:"ConnectionsPerSec"`
}

// getMonitorConnection は skr CLI で接続数モニタリングを取得します。
// モニタリングは監視データの反映待ちを必要としないため、作成直後でも実行できます。
// --start と --end は RFC3339 形式で、現在時刻を終端とする観測期間を指定します。
func getMonitorConnection(t *testing.T, h *helper.Helper, id types.ID) {
	t.Helper()

	end := time.Now().UTC()
	output := h.Call(t, "ProxyLB の接続数モニタリングを取得する",
		"proxylb-api", "monitor-connection", "--id", id.String(),
		"--start", end.Add(-proxylbMonitorWindow).Format(time.RFC3339),
		"--end", end.Format(time.RFC3339))
	require.NoError(t, checkMonitorConnectionValues(output),
		"monitor-connection should return well-formed monitor values")
}

// checkMonitorConnectionValues はモニタ値の応答が配列で、各要素が Time を持つことを確認します。
func checkMonitorConnectionValues(output []byte) error {
	var values []monitorConnectionValue
	if err := json.Unmarshal(output, &values); err != nil {
		return fmt.Errorf("decode monitor values: %w", err)
	}
	if values == nil {
		return errors.New("expected a JSON array of monitor values")
	}
	for i, value := range values {
		if value.Time.IsZero() {
			return fmt.Errorf("monitor values[%d] has no Time", i)
		}
	}
	return nil
}

// getCertificateCommonName は skr CLI で証明書の共通名を取得します。
// get-certificates の応答には主証明書・追加証明書の秘密鍵も含まれるため、
// 応答全体を実行記録に残さず、--query で共通名だけに絞り込みます。
func getCertificateCommonName(t *testing.T, h *helper.Helper, id types.ID) string {
	t.Helper()

	data, _, err := h.CallAllowingFailure(t, "ProxyLB の証明書の共通名を取得する",
		"proxylb-api", "get-certificates", "--id", id.String(),
		"--query", ".PrimaryCert.CertificateCommonName",
	)
	require.NoError(t, err, "get ProxyLB certificate common name")
	var cn string
	require.NoError(t, json.Unmarshal(data, &cn), "decode certificate common name")
	return cn
}

// requestProxyLBContent は curl でエンハンスドロードバランサの宛先 (VIP または FQDN) にリクエストし、応答本文を返します。
// 証明書を検証しないため https の自己署名証明書にも到達できます。
// 応答本文を返し、失敗した場合はエラーを返します。実行は実行記録に残ります。
func requestProxyLBContent(t *testing.T, h *helper.Helper, scheme, host string) (string, error) {
	t.Helper()

	stdout, stderr, err := h.RunLocalCommand(t, curlCommandTimeout, "curl", "エンハンスドロードバランサ宛てにリクエストして配下サーバーの応答を確認する",
		"--fail", "--silent", "--show-error", "--max-time", "15", "-k",
		scheme+"://"+host+"/",
	)
	if err != nil {
		return stdout, fmt.Errorf("curl %s://%s: %w (stderr: %s)", scheme, host, err, stderr)
	}
	return strings.TrimSpace(stdout), nil
}

// waitForProxyLBHTTP は HTTP GET が配下サーバーの応答本文を返すまで期限付きで待ちます。
// ProxyLB の準備待ちの間のエラーは再試行し、期限を過ぎたらテストを失敗させます。
func waitForProxyLBHTTP(t *testing.T, h *helper.Helper, host, want string) {
	t.Helper()

	waitForProxyLBContent(t, h, "http", host, want)
}

// waitForProxyLBHTTPS は HTTPS GET が配下サーバーの応答本文を返すまで期限付きで待ちます。
// 証明書設定の反映待ちの間のエラーは再試行し、期限を過ぎたらテストを失敗させます。
func waitForProxyLBHTTPS(t *testing.T, h *helper.Helper, host, want string) {
	t.Helper()

	waitForProxyLBContent(t, h, "https", host, want)
}

// waitForProxyLBContent は指定したスキームで宛先 (VIP または FQDN) にリクエストし、
// 配下サーバーの応答本文が返るまで期限付きで待ちます。期限を過ぎたらテストを失敗させます。
func waitForProxyLBContent(t *testing.T, h *helper.Helper, scheme, host, want string) {
	t.Helper()

	deadline := time.Now().Add(proxylbWaitTimeout)
	var lastBody string
	var lastErr error
	for {
		body, err := requestProxyLBContent(t, h, scheme, host)
		if err == nil && body == want {
			return
		}
		lastBody, lastErr = body, err
		if time.Now().Add(proxylbPolling).After(deadline) {
			t.Fatalf("%s response for %s did not become %q within %s: body=%q error=%v",
				strings.ToUpper(scheme), host, want, proxylbWaitTimeout, lastBody, lastErr)
		}
		h.Sleep(t, fmt.Sprintf("%s の反映を待つ (%s)", strings.ToUpper(scheme), host), proxylbPolling)
	}
}

// waitForProxyLBServerStatus は health-status で配下サーバーが up になるまで期限付きで待ちます。
// ProxyLB の準備待ちの間の応答は再試行し、期限を過ぎたらテストを失敗させます。
func waitForProxyLBServerStatus(t *testing.T, h *helper.Helper, id types.ID, serverIP string) {
	t.Helper()

	deadline := time.Now().Add(proxylbWaitTimeout)
	for {
		items := healthStatus(t, h, id)
		for _, item := range items {
			// API は UP を大文字で返すため、SDK の IsUp で判定します
			if item.IPAddress == serverIP && item.Status.IsUp() {
				return
			}
		}
		if time.Now().Add(proxylbPolling).After(deadline) {
			t.Fatalf("health status for backend %s did not become up within %s: items=%v",
				serverIP, proxylbWaitTimeout, items)
		}
		h.Sleep(t, "ProxyLB の配下サーバーが up になるのを待つ", proxylbPolling)
	}
}

// TestRequestProxyLBContent は curl の失敗時にエラーへ変換することを確かめる。
func TestRequestProxyLBContent(t *testing.T) {
	// curl が利用できない環境ではテストをスキップする
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl is not available")
	}
	h := helper.NewWithTemporaryProfileDir(t, "proxylb-api", "curl のエラー変換を確認する")
	_, err := requestProxyLBContent(t, h, "http", "localhost:1")
	require.Error(t, err, "connection refused should be an error")
}

// TestCheckMonitorConnectionValues は接続数モニタリングの応答の内容確認を確かめる。
func TestCheckMonitorConnectionValues(t *testing.T) {
	require.NoError(t, checkMonitorConnectionValues([]byte(`[{"Time":"2026-01-01T00:00:00Z","ActiveConnections":0,"ConnectionsPerSec":0}]`)))
	require.NoError(t, checkMonitorConnectionValues([]byte(`[]`)))

	require.Error(t, checkMonitorConnectionValues([]byte(`[{"Time":"invalid"}]`)),
		"an unparseable Time should be refused")
	require.Error(t, checkMonitorConnectionValues([]byte(`[{"ActiveConnections":0}]`)),
		"a missing Time should be refused")
	require.Error(t, checkMonitorConnectionValues([]byte(`{"Time":"2026-01-01T00:00:00Z"}`)),
		"a non-array response should be refused")
	require.Error(t, checkMonitorConnectionValues([]byte("{")),
		"invalid JSON should be refused")
}
