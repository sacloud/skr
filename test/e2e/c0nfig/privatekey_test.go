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
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
)

func TestWritePrivateKey(t *testing.T) {
	dir := t.TempDir()
	path, err := writePrivateKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	if path != dir+"/key.pem" && path != dir+"\\key.pem" {
		t.Fatalf("path = %q, want key.pem under %q", path, dir)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %04o, want 0600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path) //nolint:gosec // 直前に書き込まれたプライベートな一時ファイルのみを読み込む。
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		t.Fatalf("pem block = %+v, want a PRIVATE KEY block", block)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse PKCS8 key: %v", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		t.Fatalf("key type = %T, want *rsa.PrivateKey", key)
	}
	if rsaKey.N.BitLen() != 2048 {
		t.Fatalf("key size = %d, want 2048", rsaKey.N.BitLen())
	}
}
