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
	// dnsQueryTimeout は dig 1回のタイムアウトです。
	dnsQueryTimeout = 10 * time.Second
)

// lookupDNSRecord は dig で指定した権威 DNS サーバーに直接問い合わせ、応答ヘッダーとレコードを解析します。
// 応答コードが NOERROR の場合だけレコード値を返し、SERVFAIL、NXDOMAIN、REFUSED、応答ヘッダーの欠落、
// 複数の応答ヘッダー、不正な A・AAAA レコードはエラーにします。委譲されていないゾーンも、
// 権威サーバーに直接問い合わせることで応答を確認できます。問い合わせの実行は実行記録に残ります。
func lookupDNSRecord(t *testing.T, h *helper.Helper, nameServer, fqdn, recordType string) ([]string, error) {
	t.Helper()

	if recordType != "A" && recordType != "AAAA" {
		return nil, fmt.Errorf("unsupported record type for dig check: %s", recordType)
	}
	args := []string{"@" + nameServer, "+norecurse", "+noall", "+comments", "+answer", "+time=3", "+tries=1", fqdn, recordType}
	stdout, stderr, err := h.RunLocalCommand(t, dnsQueryTimeout, "dig", "権威 DNS サーバーにレコードを問い合わせる", args...)
	if err != nil {
		return nil, fmt.Errorf("run dig: %w (stderr: %s)", err, stderr)
	}
	return parseDNSRecordAnswers(stdout, recordType)
}

// parseDNSRecordAnswers は dig の応答から応答ヘッダーの status とレコード値を解析します。
func parseDNSRecordAnswers(output, recordType string) ([]string, error) {
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
		if len(fields) == 5 && fields[2] == "IN" && fields[3] == recordType {
			ip := net.ParseIP(fields[4])
			if ip == nil {
				return nil, errors.New("DNS query returned an invalid address record")
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

// TestParseDNSRecordAnswers は dig の応答の解析を確かめます。
func TestParseDNSRecordAnswers(t *testing.T) {
	for _, test := range []struct {
		name       string
		output     string
		recordType string
		want       []string
		wantErr    string
	}{
		{
			name:       "A レコードを取り出す",
			output:     ";; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 123\nwww.example.test. 600 IN A 192.0.2.10\n",
			recordType: "A",
			want:       []string{"192.0.2.10"},
		},
		{
			name:       "AAAA レコードを取り出す",
			output:     ";; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 123\nwww6.example.test. 600 IN AAAA 2001:db8::10\n",
			recordType: "AAAA",
			want:       []string{"2001:db8::10"},
		},
		{
			name:       "NXDOMAIN はエラー",
			output:     ";; ->>HEADER<<- opcode: QUERY, status: NXDOMAIN, id: 123\n",
			recordType: "A",
			wantErr:    `status "NXDOMAIN", want NOERROR`,
		},
		{
			name:       "応答ヘッダーがないときはエラー",
			output:     "www.example.test. 600 IN A 192.0.2.10\n",
			recordType: "A",
			wantErr:    "no response header",
		},
		{
			name:       "不正なアドレスはエラー",
			output:     ";; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 123\nwww.example.test. 600 IN A not-an-address\n",
			recordType: "A",
			wantErr:    "invalid address record",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			answers, err := parseDNSRecordAnswers(test.output, test.recordType)
			if test.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, answers)
		})
	}
}
