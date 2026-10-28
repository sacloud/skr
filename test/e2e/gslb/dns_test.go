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
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/skr/test/e2e/internal/helper"
	"github.com/stretchr/testify/require"
)

const (
	// dnsWaitTimeout は DNS の反映待ちの期限です。作成・切り替えの反映を最大 10 分待ちます。
	dnsWaitTimeout = 10 * time.Minute
	// dnsPolling は DNS の再問い合わせ間隔です。
	dnsPolling = 10 * time.Second
	// dnsCommandTimeout は dig 1回のタイムアウトです。
	dnsCommandTimeout = 5 * time.Second
)

// parseDNSAnswers は dig の応答ヘッダーと A レコードを解析します。
// 応答コードが NOERROR の場合だけ A レコードを返し、SERVFAIL、NXDOMAIN、応答ヘッダーの欠落、
// 複数の応答ヘッダー、不正な A レコードはエラーにします。
func parseDNSAnswers(output string) ([]string, error) {
	var answers []string
	headerSeen := false
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, ";; ->>HEADER<<-") {
			if headerSeen {
				return nil, errors.New("DNS query returned multiple response headers")
			}
			headerSeen = true
			status, err := dnsResponseStatus(line)
			if err != nil {
				return nil, err
			}
			if status != "NOERROR" {
				return nil, fmt.Errorf("DNS query returned status %q, want NOERROR", status)
			}
		}
		fields := strings.Fields(line)
		if len(fields) == 5 && fields[2] == "IN" && fields[3] == "A" {
			ip := net.ParseIP(fields[4])
			if ip == nil || ip.To4() == nil {
				return nil, errors.New("DNS query returned an invalid A record")
			}
			answers = append(answers, ip.String())
		}
	}
	if !headerSeen {
		return nil, errors.New("DNS query returned no response header")
	}
	return answers, nil
}

// dnsResponseStatus は dig の応答ヘッダー行から status の値を取り出します。ヘッダーに status がない場合はエラーを返します。
func dnsResponseStatus(header string) (string, error) {
	fields := strings.Fields(header)
	for i, field := range fields {
		if field == "status:" && i+1 < len(fields) {
			return strings.TrimSuffix(fields[i+1], ","), nil
		}
	}
	return "", errors.New("DNS response header has no status, want NOERROR")
}

// lookupDNS は dig で A レコードを問い合わせ、応答ヘッダーと A レコードを解析します。
// 応答コードと A レコードで応答先を判定するため、dig の短縮出力は使いません。
// 問い合わせの実行は実行記録に残ります。成否の判断は呼び出し元が行います。
func lookupDNS(t *testing.T, h *helper.Helper, fqdn string) ([]string, error) {
	t.Helper()

	args := []string{"+noall", "+comments", "+answer", "+time=3", "+tries=1", fqdn, "A"}
	stdout, stderr, err := h.RunLocalCommand(t, dnsCommandTimeout, "dig", "DNS の A レコードを問い合わせる", args...)
	if err != nil {
		return nil, fmt.Errorf("run dig: %w (stderr: %s)", err, stderr)
	}
	return parseDNSAnswers(stdout)
}

// waitForDNS は DNS の A レコードが期待する IP アドレスだけになるまで期限付きで待ちます。
// 反映待ちの間の問い合わせエラーは再試行し、期限を過ぎたらテストを失敗させます。
func waitForDNS(t *testing.T, h *helper.Helper, fqdn, ip string) {
	t.Helper()

	deadline := time.Now().Add(dnsWaitTimeout)
	for {
		answers, err := lookupDNS(t, h, fqdn)
		if err == nil && len(answers) == 1 && answers[0] == ip {
			return
		}
		if time.Now().Add(dnsPolling).After(deadline) {
			t.Fatalf("DNS answers for %s did not become exactly [%s] within %s: answers=%v error=%v",
				fqdn, ip, dnsWaitTimeout, answers, err)
		}
		time.Sleep(dnsPolling)
	}
}

// TestParseDNSAnswers は dig の応答の解析を確かめる。
func TestParseDNSAnswers(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantIP   string
		wantErr  string
		wantNone bool
	}{
		{
			name: "healthy",
			output: ";; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 123\n" +
				"example.test. 10 IN A 203.0.113.10\n",
			wantIP: "203.0.113.10",
		},
		{
			name:     "no A records",
			output:   ";; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 123\n",
			wantNone: true,
		},
		{
			name:    "SERVFAIL",
			output:  ";; ->>HEADER<<- opcode: QUERY, status: SERVFAIL, id: 123\n",
			wantErr: "SERVFAIL",
		},
		{
			name:    "NXDOMAIN",
			output:  ";; ->>HEADER<<- opcode: QUERY, status: NXDOMAIN, id: 123\n",
			wantErr: "NXDOMAIN",
		},
		{name: "empty output", wantErr: "no response header"},
		{
			name:    "short output is insufficient",
			output:  "203.0.113.10\n",
			wantErr: "no response header",
		},
		{
			name:    "missing status",
			output:  ";; ->>HEADER<<- opcode: QUERY, id: 123\n",
			wantErr: "want NOERROR",
		},
		{
			name: "invalid A record",
			output: ";; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 123\n" +
				"example.test. 10 IN A invalid\n",
			wantErr: "invalid A record",
		},
		{
			name: "multiple headers",
			output: ";; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 123\n" +
				";; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 124\n",
			wantErr: "multiple response headers",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answers, err := parseDNSAnswers(tt.output)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantNone {
				require.Empty(t, answers, "no A records should be returned")
				return
			}
			require.Equal(t, []string{tt.wantIP}, answers)
		})
	}
}

// TestDNSResponseStatus は応答ヘッダー行からの status の取り出しを確かめる。
func TestDNSResponseStatus(t *testing.T) {
	status, err := dnsResponseStatus(";; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 123")
	require.NoError(t, err)
	require.Equal(t, "NOERROR", status)

	status, err = dnsResponseStatus(";; ->>HEADER<<- opcode: QUERY, status: SERVFAIL, id: 123")
	require.NoError(t, err)
	require.Equal(t, "SERVFAIL", status)

	_, err = dnsResponseStatus(";; ->>HEADER<<- opcode: QUERY, id: 123")
	require.Error(t, err, "a header without status should be refused")
}
