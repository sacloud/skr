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

package tutorials

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadBaseline(t *testing.T) {
	t.Run("正常系", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "baseline.txt")
		content := "# コメント行\n" +
			"\n" +
			"iaas-api/zone.md implemented 6\n" +
			"eventbus.md deferred 10 sakumock が SimpleMQ の API キー発行に未対応\n"
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

		entries, err := loadBaseline(path)
		require.NoError(t, err)
		require.Len(t, entries, 2)
		require.Equal(t, "implemented", entries["iaas-api/zone.md"].Status)
		require.Equal(t, 6, entries["iaas-api/zone.md"].Blocks)
		require.Equal(t, "deferred", entries["eventbus.md"].Status)
		require.Equal(t, "sakumock が SimpleMQ の API キー発行に未対応", entries["eventbus.md"].Reason)
	})

	t.Run("フィールド不足", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "baseline.txt")
		require.NoError(t, os.WriteFile(path, []byte("zone.md implemented\n"), 0o600))
		_, err := loadBaseline(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid baseline entry")
	})

	t.Run("不正なステータス", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "baseline.txt")
		require.NoError(t, os.WriteFile(path, []byte("zone.md pending 6\n"), 0o600))
		_, err := loadBaseline(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid status")
	})

	t.Run("implemented に理由は書けない", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "baseline.txt")
		require.NoError(t, os.WriteFile(path, []byte("zone.md implemented 6 なぜか理由\n"), 0o600))
		_, err := loadBaseline(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "must not have a reason")
	})

	t.Run("deferred には理由が必須", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "baseline.txt")
		require.NoError(t, os.WriteFile(path, []byte("zone.md deferred 6\n"), 0o600))
		_, err := loadBaseline(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "must have a reason")
	})

	t.Run("重複したエントリ", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "baseline.txt")
		content := "zone.md implemented 6\nzone.md implemented 6\n"
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		_, err := loadBaseline(path)
		require.Error(t, err)
		require.Contains(t, err.Error(), "duplicate entry")
	})
}
