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

package databaseapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/sacloud-sdk-go/service/iaas/database"
	"github.com/stretchr/testify/require"
)

// fakeDatabaseAPI は API の動作を置き換えるテスト用実装です。
type fakeDatabaseAPI struct {
	createZone    string
	createRequest *iaas.DatabaseCreateRequest
	createErr     error
	readDatabase  *iaas.Database
	readErr       error
	readRequests  []*database.ReadRequest
	updateZone    string
	updateID      types.ID
	updateRequest *iaas.DatabaseUpdateRequest
	updateErr     error
}

func (f *fakeDatabaseAPI) FindWithContext(context.Context, *database.FindRequest) ([]*iaas.Database, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseAPI) ReadWithContext(_ context.Context, request *database.ReadRequest) (*iaas.Database, error) {
	f.readRequests = append(f.readRequests, request)
	if f.readErr != nil {
		return nil, f.readErr
	}
	return f.readDatabase, nil
}

func (f *fakeDatabaseAPI) Create(_ context.Context, zone string, request *iaas.DatabaseCreateRequest) (*iaas.Database, error) {
	f.createZone = zone
	f.createRequest = request
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &iaas.Database{ID: 123, Name: request.Name}, nil
}

func (f *fakeDatabaseAPI) Update(_ context.Context, zone string, id types.ID, request *iaas.DatabaseUpdateRequest) (*iaas.Database, error) {
	f.updateZone = zone
	f.updateID = id
	f.updateRequest = request
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &iaas.Database{ID: 123, Name: "db-01"}, nil
}

func (f *fakeDatabaseAPI) DeleteWithContext(context.Context, *database.DeleteRequest) error {
	return errors.New("not implemented")
}

func (f *fakeDatabaseAPI) BootWithContext(context.Context, *database.BootRequest) error {
	return errors.New("not implemented")
}

func (f *fakeDatabaseAPI) ShutdownWithContext(context.Context, *database.ShutdownRequest) error {
	return errors.New("not implemented")
}

func (f *fakeDatabaseAPI) ResetWithContext(context.Context, *database.ResetRequest) error {
	return errors.New("not implemented")
}

func (f *fakeDatabaseAPI) MonitorCPUWithContext(context.Context, *database.MonitorCPURequest) ([]*iaas.MonitorCPUTimeValue, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseAPI) MonitorDiskWithContext(context.Context, *database.MonitorDiskRequest) ([]*iaas.MonitorDiskValue, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseAPI) MonitorInterfaceWithContext(context.Context, *database.MonitorInterfaceRequest) ([]*iaas.MonitorInterfaceValue, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseAPI) MonitorDatabaseWithContext(context.Context, *database.MonitorDatabaseRequest) ([]*iaas.MonitorDatabaseValue, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseAPI) ListParameterWithContext(context.Context, *database.ListParameterRequest) ([]*database.Parameter, error) {
	return nil, errors.New("not implemented")
}

// testRuntime は出力処理をスタブに差し替えた Runtime です。
func testRuntime() Runtime {
	return Runtime{
		DecodeRequest:  decodeTestRequest,
		ValidateOutput: func(*kong.Context) error { return nil },
		WriteOutput: func(ctx *kong.Context, value any) error {
			encoder := json.NewEncoder(ctx.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(value)
		},
	}
}

// decodeTestRequest は --request JSON を復号するテスト用ヘルパーです。inline JSON と
// @path 形式に対応し、internal/cli の decodeRequest と同じ動作を模擬します。
func decodeTestRequest(input string, destination any) error {
	data := []byte(input)
	if len(input) > 0 && input[0] == '@' {
		read, err := os.ReadFile(input[1:])
		if err != nil {
			return fmt.Errorf("read request file: %w", err)
		}
		data = read
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	return nil
}

func parseCommand(t *testing.T, command any, args ...string) *kong.Context {
	t.Helper()
	parser, err := kong.New(command, kong.Exit(func(int) {}))
	require.NoError(t, err)
	ctx, err := parser.Parse(args)
	require.NoError(t, err)
	return ctx
}

// writePasswordFile はパスワードを一時ファイルに書き込み、パスを返します。
func writePasswordFile(t *testing.T, password string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "db-password.txt")
	require.NoError(t, os.WriteFile(path, []byte(password+"\n"), 0o600))
	return path
}

func TestDatabaseCreateCommandRunWithFlags(t *testing.T) {
	t.Run("basic mariadb", func(t *testing.T) {
		fake := &fakeDatabaseAPI{}
		command := &CreateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		passwordFile := writePasswordFile(t, "secret-password")
		kongCtx := parseCommand(t, command,
			"--zone", "is1b", "--name", "db-01",
			"--plan-id", "10", "--switch-id", "112900500001",
			"--ip-address", "192.168.0.11", "--network-mask-len", "24",
			"--database-type", "mariadb", "--username", "dbuser",
			"--password", "@"+passwordFile,
		)
		require.NoError(t, command.Run(kongCtx))
		// ゾーンは api 層の操作の引数として渡される
		require.Equal(t, "is1b", fake.createZone)
		request := fake.createRequest
		require.Equal(t, "db-01", request.Name)
		require.EqualValues(t, 10, request.PlanID)
		require.EqualValues(t, 112900500001, request.SwitchID)
		require.Equal(t, []string{"192.168.0.11"}, request.IPAddresses)
		require.Equal(t, 24, request.NetworkMaskLen)
		// RDBMS の種別は API の Conf.DatabaseName に対応します (MariaDB の表記は API の値)
		require.NotNil(t, request.Conf)
		require.Equal(t, "MariaDB", request.Conf.DatabaseName)
		require.NotNil(t, request.CommonSetting)
		require.Equal(t, "dbuser", request.CommonSetting.DefaultUser)
		require.Equal(t, "secret-password", request.CommonSetting.UserPassword)
		require.Nil(t, request.ReplicationSetting)
		require.Nil(t, request.BackupSetting)
	})

	t.Run("postgres with backup and web ui", func(t *testing.T) {
		fake := &fakeDatabaseAPI{}
		command := &CreateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		passwordFile := writePasswordFile(t, "secret-password")
		kongCtx := parseCommand(t, command,
			"--zone", "is1b", "--name", "db-02",
			"--description", "tutorial", "--tags", "production,managed",
			"--plan-id", "30", "--switch-id", "112900500001",
			"--ip-address", "192.168.0.12", "--network-mask-len", "24",
			"--default-route", "192.168.0.1", "--port", "55432",
			"--source-network", "192.168.0.0/24",
			"--database-type", "postgres", "--database-version", "17",
			"--username", "dbuser", "--password", "@"+passwordFile,
			"--enable-web-ui", "--enable-backup", "--backup-weekday", "mon,tue",
			"--backup-start-time-hour", "2", "--backup-start-time-minute", "15",
			"--enable-monitoring-suite",
			"--disk-encryption-algorithm", "aes256_xts", "--disk-encryption-kms-key", "123",
		)
		require.NoError(t, command.Run(kongCtx))
		request := fake.createRequest
		require.Equal(t, "db-02", request.Name)
		require.Equal(t, "tutorial", request.Description)
		require.Equal(t, types.Tags{"production", "managed"}, request.Tags)
		require.EqualValues(t, 30, request.PlanID)
		require.Equal(t, []string{"192.168.0.12"}, request.IPAddresses)
		require.Equal(t, "192.168.0.1", request.DefaultRoute)
		require.NotNil(t, request.CommonSetting)
		// ポートは API の CommonSetting.ServicePort に対応します
		require.Equal(t, 55432, request.CommonSetting.ServicePort)
		require.Equal(t, []string{"192.168.0.0/24"}, request.CommonSetting.SourceNetwork)
		// Web UI は API の CommonSetting.WebUI に対応します
		require.Equal(t, types.WebUI("true"), request.CommonSetting.WebUI)
		// postgres の表記は API の値のまま
		require.NotNil(t, request.Conf)
		require.Equal(t, "postgres", request.Conf.DatabaseName)
		require.Equal(t, "17", request.Conf.DatabaseVersion)
		require.NotNil(t, request.BackupSetting)
		// バックアップ開始時刻は API の BackupSetting.Time (HH:MM) に対応します
		require.Equal(t, "02:15", request.BackupSetting.Time)
		require.Equal(t, []types.EDayOfTheWeek{"mon", "tue"}, request.BackupSetting.DayOfWeek)
		require.Equal(t, 8, request.BackupSetting.Rotate)
		require.NotNil(t, request.MonitoringSuite)
		require.True(t, request.MonitoringSuite.Enabled)
		require.NotNil(t, request.Disk)
		require.Equal(t, types.DiskEncryptionAlgorithms.AES256XTS, request.Disk.EncryptionAlgorithm)
		require.EqualValues(t, 123, request.Disk.EncryptionKeyID)
	})

	t.Run("replication with two ip addresses", func(t *testing.T) {
		fake := &fakeDatabaseAPI{}
		command := &CreateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		passwordFile := writePasswordFile(t, "secret-password")
		replicaFile := writePasswordFile(t, "replica-secret")
		kongCtx := parseCommand(t, command,
			"--zone", "is1b", "--name", "db-03",
			"--plan-id", "10", "--switch-id", "112900500001",
			"--ip-address", "192.168.0.11", "--ip-address", "192.168.0.12",
			"--network-mask-len", "24",
			"--database-type", "mariadb", "--username", "dbuser",
			"--password", "@"+passwordFile,
			"--enable-replication", "--replica-user-password", "@"+replicaFile,
		)
		require.NoError(t, command.Run(kongCtx))
		request := fake.createRequest
		require.Equal(t, []string{"192.168.0.11", "192.168.0.12"}, request.IPAddresses)
		// レプリケーションユーザーは API の CommonSetting のレプリカユーザーに対応します
		require.NotNil(t, request.CommonSetting)
		require.Equal(t, "replica", request.CommonSetting.ReplicaUser)
		require.Equal(t, "replica-secret", request.CommonSetting.ReplicaPassword)
		require.NotNil(t, request.ReplicationSetting)
		require.Equal(t, types.DatabaseReplicationModels.MasterSlave, request.ReplicationSetting.Model)
	})

	t.Run("password from standard input", func(t *testing.T) {
		fake := &fakeDatabaseAPI{}
		command := &CreateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command,
			"--zone", "is1b", "--name", "db-04",
			"--plan-id", "10", "--switch-id", "112900500001",
			"--ip-address", "192.168.0.11", "--network-mask-len", "24",
			"--database-type", "mariadb", "--username", "dbuser",
			"--password", "-",
		)
		request, err := command.buildRequest(strings.NewReader("stdin-password\n"))
		require.NoError(t, err)
		_ = kongCtx
		require.Equal(t, "stdin-password", request.CommonSetting.UserPassword)
	})
}

func TestDatabaseCreateCommandRequestJSON(t *testing.T) {
	requestJSON := `{"Name":"db-json","PlanID":10,"SwitchID":112900500001,` +
		`"IPAddresses":["192.168.0.11"],"NetworkMaskLen":24,` +
		`"CommonSetting":{"DefaultUser":"dbuser","UserPassword":"json-password"}}`

	t.Run("accepts request file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "db-create.json")
		require.NoError(t, os.WriteFile(path, []byte(requestJSON), 0o600))
		fake := &fakeDatabaseAPI{}
		command := &CreateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--zone", "is1b", "--request", "@"+path)
		require.NoError(t, command.Run(kongCtx))
		require.Equal(t, "is1b", fake.createZone)
		request := fake.createRequest
		require.Equal(t, "db-json", request.Name)
		require.NotNil(t, request.CommonSetting)
		require.Equal(t, "json-password", request.CommonSetting.UserPassword)
	})

	t.Run("rejects inline json because it may contain secrets", func(t *testing.T) {
		fake := &fakeDatabaseAPI{}
		command := &CreateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--zone", "is1b", "--request", requestJSON)
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "@path.json")
	})

	t.Run("request and flags are mutually exclusive", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "db-create.json")
		require.NoError(t, os.WriteFile(path, []byte(requestJSON), 0o600))
		fake := &fakeDatabaseAPI{}
		command := &CreateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--zone", "is1b", "--request", "@"+path, "--name", "other")
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--request と個別フラグは併用できません")
	})

	t.Run("no flags and no request", func(t *testing.T) {
		fake := &fakeDatabaseAPI{}
		command := &CreateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, "--zone", "is1b")
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--request または個別フラグが必要です")
	})
}

func TestDatabaseCreateCommandRejectsInvalidInput(t *testing.T) {
	passwordFile := writePasswordFile(t, "secret-password")
	base := []string{
		"--zone", "is1b", "--name", "db-01",
		"--plan-id", "10", "--switch-id", "112900500001",
		"--ip-address", "192.168.0.11", "--network-mask-len", "24",
		"--database-type", "mariadb", "--username", "dbuser",
		"--password", "@" + passwordFile,
	}
	run := func(t *testing.T, args ...string) error {
		t.Helper()
		fake := &fakeDatabaseAPI{}
		command := &CreateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
		kongCtx := parseCommand(t, command, args...)
		err := command.Run(kongCtx)
		if err == nil {
			require.Nil(t, fake.createRequest, "invalid input reached API: %+v", fake.createRequest)
		}
		return err
	}

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"missing name", []string{"--zone", "is1b", "--plan-id", "10", "--switch-id", "112900500001", "--ip-address", "192.168.0.11", "--network-mask-len", "24", "--database-type", "mariadb", "--username", "dbuser", "--password", "@" + passwordFile}, "--name が必要です"},
		{"missing plan id", []string{"--zone", "is1b", "--name", "db", "--switch-id", "112900500001", "--ip-address", "192.168.0.11", "--network-mask-len", "24", "--database-type", "mariadb", "--username", "dbuser", "--password", "@" + passwordFile}, "--plan-id が必要です"},
		{"missing switch id", []string{"--zone", "is1b", "--name", "db", "--plan-id", "10", "--ip-address", "192.168.0.11", "--network-mask-len", "24", "--database-type", "mariadb", "--username", "dbuser", "--password", "@" + passwordFile}, "--switch-id が必要です"},
		{"missing ip address", []string{"--zone", "is1b", "--name", "db", "--plan-id", "10", "--switch-id", "112900500001", "--network-mask-len", "24", "--database-type", "mariadb", "--username", "dbuser", "--password", "@" + passwordFile}, "--ip-address が必要です"},
		{"too many ip addresses", append(append([]string{}, base[:10]...), "--ip-address", "192.168.0.11", "--ip-address", "192.168.0.12", "--ip-address", "192.168.0.13", "--network-mask-len", "24", "--database-type", "mariadb", "--username", "dbuser", "--password", "@"+passwordFile), "2個まで"},
		{"invalid ipv4", append(append([]string{}, base...), "--ip-address", "2001:db8::1"), "IPv4 アドレス"},
		{"missing mask len", []string{"--zone", "is1b", "--name", "db", "--plan-id", "10", "--switch-id", "112900500001", "--ip-address", "192.168.0.11", "--database-type", "mariadb", "--username", "dbuser", "--password", "@" + passwordFile}, "--network-mask-len が必要です"},
		{"mask len out of range", append(append([]string{}, base...), "--network-mask-len", "33"), "1 から 32"},
		{"invalid database type", append(append([]string{}, base...), "--database-type", "oracle"), "mariadb または postgres"},
		{"missing username", []string{"--zone", "is1b", "--name", "db", "--plan-id", "10", "--switch-id", "112900500001", "--ip-address", "192.168.0.11", "--network-mask-len", "24", "--database-type", "mariadb", "--password", "@" + passwordFile}, "--username が必要です"},
		{"missing password flag", []string{"--zone", "is1b", "--name", "db", "--plan-id", "10", "--switch-id", "112900500001", "--ip-address", "192.168.0.11", "--network-mask-len", "24", "--database-type", "mariadb", "--username", "dbuser"}, "--password が必要です"},
		{"port out of range", append(append([]string{}, base...), "--port", "65536"), "1 から 65535"},
		{"replication without replica password", append(append([]string{}, base...), "--enable-replication"), "--replica-user-password が必要です"},
		{"replica password without replication", append(append([]string{}, base...), "--replica-user-password", "@"+passwordFile), "--enable-replication とともに"},
		{"invalid weekday", append(append([]string{}, base...), "--enable-backup", "--backup-weekday", "someday"), "someday"},
		{"backup v2 without weekday", append(append([]string{}, base...), "--enable-backup-v2", "--backup-v2-connect", "objectstorage"), "--backup-v2-weekday が必要です"},
		{"backup v2 without connect", append(append([]string{}, base...), "--enable-backup-v2", "--backup-v2-weekday", "mon"), "--backup-v2-connect が必要です"},
		{"encryption without kms key", append(append([]string{}, base...), "--disk-encryption-algorithm", "aes256_xts"), "両方指定"},
		{"kms key without algorithm", append(append([]string{}, base...), "--disk-encryption-kms-key", "123"), "両方指定"},
		{"invalid encryption algorithm", append(append([]string{}, base...), "--disk-encryption-algorithm", "AES256-XTS", "--disk-encryption-kms-key", "123"), "aes256_xts で指定"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := run(t, test.args...)
			require.Error(t, err)
			require.Contains(t, err.Error(), test.want)
		})
	}

	t.Run("zone all is rejected", func(t *testing.T) {
		err := run(t, append([]string{"--zone", "all"}, base[2:]...)...)
		require.Error(t, err)
	})
}

func TestDatabaseCreateCommandSecretInput(t *testing.T) {
	// buildRequest は必須項目を検証するため、秘密情報のテストでは必要な項目を設定します
	secretBase := func(t *testing.T) *CreateCommand {
		t.Helper()
		return &CreateCommand{
			Name:           "db-01",
			PlanID:         10,
			SwitchID:       112900500001,
			IPAddresses:    []string{"192.168.0.11"},
			NetworkMaskLen: 24,
			DatabaseType:   "mariadb",
			Username:       "dbuser",
		}
	}

	t.Run("rejects inline password", func(t *testing.T) {
		command := secretBase(t)
		command.Password = &[]string{"inline-secret"}[0]
		_, err := command.buildRequest(strings.NewReader(""))
		require.Error(t, err)
		require.Contains(t, err.Error(), "@path または - で指定")
	})

	t.Run("rejects missing password file", func(t *testing.T) {
		command := secretBase(t)
		command.Password = &[]string{"@/nonexistent/password.txt"}[0]
		_, err := command.buildRequest(strings.NewReader(""))
		require.Error(t, err)
		require.Contains(t, err.Error(), "read --password file")
	})

	t.Run("rejects empty password file", func(t *testing.T) {
		path := writePasswordFile(t, "")
		command := secretBase(t)
		command.Password = &[]string{"@" + path}[0]
		_, err := command.buildRequest(strings.NewReader(""))
		require.Error(t, err)
		require.Contains(t, err.Error(), "--password が空です")
	})

	t.Run("trims trailing newline from password file", func(t *testing.T) {
		path := writePasswordFile(t, "secret")
		command := secretBase(t)
		command.Password = &[]string{"@" + path}[0]
		request, err := command.buildRequest(strings.NewReader(""))
		require.NoError(t, err)
		require.Equal(t, "secret", request.CommonSetting.UserPassword)
	})

	t.Run("rejects missing replica password file", func(t *testing.T) {
		command := secretBase(t)
		command.EnableReplication = true
		command.ReplicaUserPassword = &[]string{"@/nonexistent/replica.txt"}[0]
		command.Password = &[]string{"-"}[0]
		_, err := command.buildRequest(strings.NewReader("secret\n"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "read --replica-user-password file")
	})

	t.Run("error message does not contain password", func(t *testing.T) {
		command := secretBase(t)
		command.Password = &[]string{"inline-secret"}[0]
		_, err := command.buildRequest(strings.NewReader(""))
		require.Error(t, err)
		require.NotContains(t, err.Error(), "inline-secret")
	})
}

func TestDatabaseCreateCommandPropagatesAPIErrors(t *testing.T) {
	apiFailure := errors.New("Database API failure")
	fake := &fakeDatabaseAPI{createErr: apiFailure}
	command := &CreateCommand{
		factory: func() (API, error) { return fake, nil },
		runtime: testRuntime(),
	}
	passwordFile := writePasswordFile(t, "secret-password")
	kongCtx := parseCommand(t, command,
		"--zone", "is1b", "--name", "db-01",
		"--plan-id", "10", "--switch-id", "112900500001",
		"--ip-address", "192.168.0.11", "--network-mask-len", "24",
		"--database-type", "mariadb", "--username", "dbuser",
		"--password", "@"+passwordFile,
	)
	err := command.Run(kongCtx)
	require.Error(t, err)
	require.Contains(t, err.Error(), apiFailure.Error())
}

func TestSetMonitorDatabasePeriod(t *testing.T) {
	t.Run("未指定の場合は秒精度の既定期間を設定する", func(t *testing.T) {
		request := &database.MonitorDatabaseRequest{Zone: "is1b", ID: 123}
		require.NoError(t, SetMonitorDatabasePeriod(request))
		require.False(t, request.End.IsZero(), "End should be filled")
		require.False(t, request.Start.IsZero(), "Start should be filled")
	})

	t.Run("未指定時の既定値は直近 1 時間で秒精度に丸められる", func(t *testing.T) {
		request := &database.MonitorDatabaseRequest{Zone: "is1b", ID: 123}
		require.NoError(t, SetMonitorDatabasePeriod(request))
		require.Zero(t, request.End.Nanosecond(), "End should be truncated to seconds")
		require.Equal(t, -1*time.Hour, request.Start.Sub(request.End))
	})

	t.Run("End のみ指定時は Start を既定値で補完する", func(t *testing.T) {
		end := time.Now().Truncate(time.Second)
		request := &database.MonitorDatabaseRequest{Zone: "is1b", ID: 123, End: end}
		require.NoError(t, SetMonitorDatabasePeriod(request))
		require.Equal(t, end, request.End, "End should be kept")
		require.Equal(t, end.Add(-1*time.Hour), request.Start, "Start should be filled")
	})

	t.Run("Start のみ指定時は End を現在時刻で補完する", func(t *testing.T) {
		start := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
		request := &database.MonitorDatabaseRequest{Zone: "is1b", ID: 123, Start: start}
		require.NoError(t, SetMonitorDatabasePeriod(request))
		require.Equal(t, start, request.Start, "Start should be kept")
		require.False(t, request.End.IsZero(), "End should be filled")
		require.Zero(t, request.End.Nanosecond(), "End should be truncated to seconds")
	})

	t.Run("両方指定時は指定値を変更しない", func(t *testing.T) {
		start := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
		end := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
		request := &database.MonitorDatabaseRequest{Zone: "is1b", ID: 123, Start: start, End: end}
		require.NoError(t, SetMonitorDatabasePeriod(request))
		require.Equal(t, start, request.Start)
		require.Equal(t, end, request.End)
	})
}

func TestDatabaseUpdateCommandRun(t *testing.T) {
	newCommand := func(fake *fakeDatabaseAPI) *UpdateCommand {
		return &UpdateCommand{
			factory: func() (API, error) { return fake, nil },
			runtime: testRuntime(),
		}
	}

	t.Run("updates the api request with zone and id", func(t *testing.T) {
		fake := &fakeDatabaseAPI{}
		command := newCommand(fake)
		requestFile := writeRequestFile(t, `{"Name":"db-01-renamed","Description":"updated","CommonSetting":{"DefaultUser":"dbuser"}}`)
		kongCtx := parseCommand(t, command, "--zone", "is1b", "--id", "123", "--request", "@"+requestFile)
		err := command.Run(kongCtx)
		require.NoError(t, err)
		// ゾーンと ID は api 層の操作の引数として渡される
		require.Equal(t, "is1b", fake.updateZone)
		require.EqualValues(t, 123, fake.updateID)
		request := fake.updateRequest
		require.NotNil(t, request)
		require.Equal(t, "db-01-renamed", request.Name)
		require.Equal(t, "updated", request.Description)
		require.NotNil(t, request.CommonSetting)
		require.Equal(t, "dbuser", request.CommonSetting.DefaultUser)
	})

	t.Run("omitted fields are not sent", func(t *testing.T) {
		fake := &fakeDatabaseAPI{}
		command := newCommand(fake)
		requestFile := writeRequestFile(t, `{"Description":"updated"}`)
		kongCtx := parseCommand(t, command, "--zone", "is1b", "--id", "123", "--request", "@"+requestFile)
		err := command.Run(kongCtx)
		require.NoError(t, err)
		request := fake.updateRequest
		require.NotNil(t, request)
		require.Equal(t, "updated", request.Description)
		require.Empty(t, request.Name, "omitted Name should stay empty")
		require.Nil(t, request.CommonSetting, "omitted CommonSetting should stay nil")
		require.Nil(t, request.BackupSetting, "omitted BackupSetting should stay nil")
	})

	t.Run("--request が未指定の場合はエラー", func(t *testing.T) {
		fake := &fakeDatabaseAPI{}
		command := newCommand(fake)
		kongCtx := parseCommand(t, command, "--zone", "is1b", "--id", "123")
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--request が必要です")
		require.Nil(t, fake.updateRequest, "input error should not reach API")
	})

	t.Run("--id が未指定の場合はエラー", func(t *testing.T) {
		fake := &fakeDatabaseAPI{}
		command := newCommand(fake)
		requestFile := writeRequestFile(t, `{"Description":"updated"}`)
		kongCtx := parseCommand(t, command, "--zone", "is1b", "--request", "@"+requestFile)
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--id が必要です")
		require.Nil(t, fake.updateRequest, "input error should not reach API")
	})

	t.Run("インライン JSON はエラー", func(t *testing.T) {
		fake := &fakeDatabaseAPI{}
		command := newCommand(fake)
		kongCtx := parseCommand(t, command, "--zone", "is1b", "--id", "123", "--request", `{"Description":"updated"}`)
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "@path.json または -")
		require.Nil(t, fake.updateRequest, "input error should not reach API")
	})

	t.Run("zone all はエラー", func(t *testing.T) {
		fake := &fakeDatabaseAPI{}
		command := newCommand(fake)
		requestFile := writeRequestFile(t, `{"Description":"updated"}`)
		kongCtx := parseCommand(t, command, "--zone", "all", "--id", "123", "--request", "@"+requestFile)
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Nil(t, fake.updateRequest, "input error should not reach API")
	})

	t.Run("API エラーを伝播する", func(t *testing.T) {
		apiFailure := errors.New("update API failure")
		fake := &fakeDatabaseAPI{updateErr: apiFailure}
		command := newCommand(fake)
		requestFile := writeRequestFile(t, `{"Description":"updated"}`)
		kongCtx := parseCommand(t, command, "--zone", "is1b", "--id", "123", "--request", "@"+requestFile)
		err := command.Run(kongCtx)
		require.Error(t, err)
		require.Contains(t, err.Error(), apiFailure.Error())
	})
}

// writeRequestFile は API リクエスト JSON を一時ファイルに書き込み、パスを返します。
func writeRequestFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "db-request.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
