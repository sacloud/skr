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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/internal/apigen"
	databaseapi "github.com/sacloud/skr/internal/iaas/databaseapi"
	"github.com/stretchr/testify/require"
)

func TestDatabaseGeneratedCodeMatchesConfig(t *testing.T) {
	configData, err := os.ReadFile(repositoryPath("api/commands/iaas-database.json"))
	require.NoError(t, err)
	config, err := apigen.DecodeConfig(configData, apigen.ModeIaaS)
	require.NoError(t, err)
	want, err := apigen.Generate(config, apigen.ModeIaaS)
	require.NoError(t, err)
	got, err := os.ReadFile(repositoryPath("internal/iaas/databaseapi/database_api_generated.go"))
	require.NoError(t, err)
	require.Equal(t, want, got, "generated Database commands are stale; run make generate-iaas-api API_CONFIG=api/commands/iaas-database.json API_OUTPUT=internal/iaas/databaseapi/database_api_generated.go")
}

func TestIaaSDatabaseHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want []string
	}{
		{args: []string{"iaas-api", "--help"}, want: []string{"database"}},
		{args: []string{"iaas-api", "database", "--help"}, want: []string{"find", "read", "create", "update", "delete", "boot", "shutdown", "reset", "monitor-cpu", "monitor-disk", "monitor-interface", "monitor-database", "list-parameter"}},
		{args: []string{"iaas-api", "database", "create", "--help"}, want: []string{"--zone", "--name", "--plan-id", "--switch-id", "--ip-address", "--network-mask-len", "--database-type", "--username", "--password", "@path", "インライン値は指定できません", "--backup-weekday", "--enable-replication", "aes256_xts", "@path.json", "起動の完了は待ちません"}},
		{args: []string{"iaas-api", "database", "update", "--help"}, want: []string{"--request", "@path.json", "--zone", "--id", "置き換わる"}},
		{args: []string{"iaas-api", "database", "find", "--help"}, want: []string{"--name", "部分一致", "Names", "併用不可"}},
		{args: []string{"iaas-api", "database", "delete", "--help"}, want: []string{"--force", "--fail-if-not-found", "電源 OFF"}},
		{args: []string{"iaas-api", "database", "monitor-cpu", "--help"}, want: []string{"--zone", "--id", "--start", "--end", "RFC3339", "併用不可"}},
	} {
		t.Run(strings.Join(test.args[1:], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			require.Equal(t, 0, run(test.args, &stdout, &stderr), "stderr: %s", stderr.String())
			help := strings.Join(strings.Fields(stdout.String()), " ")
			for _, expected := range test.want {
				require.Contains(t, help, expected, "help does not contain %q: %s", expected, help)
			}
		})
	}
}

func TestIaaSDatabaseCommandsBuildSDKRequests(t *testing.T) {
	t.Setenv("SAKURA_PROFILE_DIR", t.TempDir())
	api := &databaseAPITestDouble{}
	commandLine := newCLI()
	setDatabaseAPIFactory(commandLine, func() (databaseapi.API, error) { return api, nil })

	run := func(args ...string) (string, string, int) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "database"}, args...), &stdout, &stderr, commandLine)
		return stdout.String(), stderr.String(), code
	}

	passwordFile := filepath.Join(t.TempDir(), "db-password.txt")
	require.NoError(t, os.WriteFile(passwordFile, []byte("secret-password\n"), 0o600))
	requestFile := filepath.Join(t.TempDir(), "db-create.json")
	requestJSON := `{"Name":"db-json","PlanID":10,"SwitchID":112900500001,` +
		`"IPAddresses":["192.168.0.11"],"NetworkMaskLen":24,` +
		`"CommonSetting":{"DefaultUser":"dbuser","UserPassword":"json-password"}}`
	require.NoError(t, os.WriteFile(requestFile, []byte(requestJSON), 0o600))

	_, stderr, code := run("create", "--zone", "test-zone", "--name", "db-01",
		"--description", "tutorial", "--tags", "production,managed",
		"--plan-id", "10", "--switch-id", "112900500001",
		"--ip-address", "192.168.0.11", "--network-mask-len", "24",
		"--database-type", "mariadb", "--username", "dbuser",
		"--password", "@"+passwordFile,
		"--enable-backup", "--backup-weekday", "mon,tue",
		"--backup-start-time-hour", "2", "--backup-start-time-minute", "15",
	)
	require.Equal(t, 0, code, "create flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	// ゾーンは api 層の操作の引数として渡される
	require.Equal(t, "test-zone", api.createZone, "unexpected value")
	flagRequest := *api.createRequest
	require.Equal(t, "db-01", flagRequest.Name, "unexpected value")
	require.Equal(t, "tutorial", flagRequest.Description, "unexpected value")
	require.Equal(t, types.Tags{"production", "managed"}, flagRequest.Tags, "unexpected value")
	require.EqualValues(t, 10, flagRequest.PlanID, "unexpected value")
	require.EqualValues(t, 112900500001, flagRequest.SwitchID, "unexpected value")
	require.Equal(t, []string{"192.168.0.11"}, flagRequest.IPAddresses, "unexpected value")
	require.Equal(t, 24, flagRequest.NetworkMaskLen, "unexpected value")
	// RDBMS の種別、ユーザー名、パスワード、バックアップは API の対応するフィールドに反映されます
	require.NotNil(t, flagRequest.Conf)
	require.Equal(t, "MariaDB", flagRequest.Conf.DatabaseName, "unexpected value")
	require.NotNil(t, flagRequest.CommonSetting)
	require.Equal(t, "dbuser", flagRequest.CommonSetting.DefaultUser, "unexpected value")
	require.Equal(t, "secret-password", flagRequest.CommonSetting.UserPassword, "password should be read from the file")
	require.NotNil(t, flagRequest.BackupSetting)
	require.Equal(t, []types.EDayOfTheWeek{"mon", "tue"}, flagRequest.BackupSetting.DayOfWeek, "unexpected value")
	require.Equal(t, "02:15", flagRequest.BackupSetting.Time, "unexpected value")

	_, stderr, code = run("create", "--zone", "test-zone", "--request", "@"+requestFile)
	require.Equal(t, 0, code, "create JSON file: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	jsonRequest := *api.createRequest
	require.Equal(t, "db-json", jsonRequest.Name, "unexpected value")
	require.NotNil(t, jsonRequest.CommonSetting)
	require.Equal(t, "json-password", jsonRequest.CommonSetting.UserPassword, "unexpected value")

	_, stderr, code = run("find", "--zone", "test-zone", "--name", "db-01", "--tags", "production,managed", "--count", "10", "--from", "2")
	require.Equal(t, 0, code, "find flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Equal(t, "test-zone", api.findRequest.Zone, "unexpected value")
	require.Equal(t, 10, api.findRequest.Count, "unexpected value")
	require.Equal(t, 2, api.findRequest.From, "unexpected value")
	require.Equal(t, []string{"db-01"}, api.findRequest.Names, "unexpected value")
	require.Equal(t, []string{"production", "managed"}, api.findRequest.Tags, "unexpected value")

	_, stderr, code = run("find", "--request", `{"Zone":"test-zone","Names":["one","two"]}`)
	require.Equal(t, 0, code, "find JSON: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Equal(t, []string{"one", "two"}, api.findRequest.Names, "unexpected value")

	updateFile := filepath.Join(t.TempDir(), "db-update.json")
	require.NoError(t, os.WriteFile(updateFile, []byte(`{"Description":"updated","CommonSetting":{"DefaultUser":"dbuser"}}`), 0o600))
	_, stderr, code = run("update", "--zone", "test-zone", "--id", "123", "--request", "@"+updateFile)
	require.Equal(t, 0, code, "update: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	// ゾーンと ID は api 層の操作の引数として渡される
	require.Equal(t, "test-zone", api.updateZone, "unexpected value")
	require.EqualValues(t, 123, api.updateID, "unexpected value")
	require.NotNil(t, api.updateRequest, "update should call the SDK")
	require.Equal(t, "updated", api.updateRequest.Description, "unexpected value")
	require.Empty(t, api.updateRequest.Name, "omitted Name should stay empty")
	require.NotNil(t, api.updateRequest.CommonSetting, "unexpected value")

	_, stderr, code = run("delete", "--zone", "test-zone", "--id", "123", "--force", "--fail-if-not-found")
	require.Equal(t, 0, code, "delete flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Equal(t, "test-zone", api.deleteRequest.Zone, "unexpected value")
	require.EqualValues(t, 123, api.deleteRequest.ID, "unexpected value")
	require.True(t, api.deleteRequest.Force, "unexpected value")
	require.True(t, api.deleteRequest.FailIfNotFound, "unexpected value")

	_, stderr, code = run("delete", "--request", `{"Zone":"test-zone","ID":123}`)
	require.Equal(t, 0, code, "delete JSON: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.EqualValues(t, 123, api.deleteRequest.ID, "unexpected value")
	require.False(t, api.deleteRequest.Force, "unexpected value")
	require.False(t, api.deleteRequest.FailIfNotFound, "unexpected value")

	_, stderr, code = run("boot", "--zone", "test-zone", "--id", "123", "--no-wait")
	require.Equal(t, 0, code, "boot: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Equal(t, "test-zone", api.bootRequest.Zone, "unexpected value")
	require.EqualValues(t, 123, api.bootRequest.ID, "unexpected value")
	require.True(t, api.bootRequest.NoWait, "unexpected value")

	_, stderr, code = run("shutdown", "--zone", "test-zone", "--id", "123", "--force")
	require.Equal(t, 0, code, "shutdown: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Equal(t, "test-zone", api.shutdownRequest.Zone, "unexpected value")
	require.EqualValues(t, 123, api.shutdownRequest.ID, "unexpected value")
	require.True(t, api.shutdownRequest.ForceShutdown, "unexpected value")

	_, stderr, code = run("reset", "--zone", "test-zone", "--id", "123")
	require.Equal(t, 0, code, "reset: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Equal(t, "test-zone", api.resetRequest.Zone, "unexpected value")
	require.EqualValues(t, 123, api.resetRequest.ID, "unexpected value")

	_, stderr, code = run("monitor-cpu", "--zone", "test-zone", "--id", "123",
		"--start", "2026-10-14T00:00:00Z", "--end", "2026-10-15T00:00:00Z")
	require.Equal(t, 0, code, "monitor-cpu flags: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	flagMonitor := *api.monitorCPURequest
	require.Equal(t, "test-zone", flagMonitor.Zone, "unexpected value")
	require.EqualValues(t, 123, flagMonitor.ID, "unexpected value")
	require.Equal(t, time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), flagMonitor.Start, "unexpected value")
	require.Equal(t, time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), flagMonitor.End, "unexpected value")

	_, stderr, code = run("monitor-cpu", "--request", `{"Zone":"test-zone","ID":123}`)
	require.Equal(t, 0, code, "monitor-cpu JSON: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.True(t, api.monitorCPURequest.Start.IsZero(), "omitted Start should stay zero for the SDK default")
	require.True(t, api.monitorCPURequest.End.IsZero(), "omitted End should stay zero for the SDK default")

	for _, op := range []struct {
		name   string
		called func() any
	}{
		{"monitor-disk", func() any { return api.monitorDiskRequest }},
		{"monitor-interface", func() any { return api.monitorInterfaceRequest }},
		{"monitor-database", func() any { return api.monitorDatabaseRequest }},
	} {
		_, stderr, code = run(op.name, "--zone", "test-zone", "--id", "123")
		require.Equal(t, 0, code, "%s: code %d, stderr %q", op.name, code, stderr)
		require.Empty(t, stderr, "should be empty")
		require.NotNil(t, op.called(), "%s should call the SDK", op.name)
	}

	_, stderr, code = run("list-parameter", "--zone", "test-zone", "--id", "123")
	require.Equal(t, 0, code, "list-parameter: code %d, stderr %q", code, stderr)
	require.Empty(t, stderr, "should be empty")
	require.Equal(t, "test-zone", api.listParameterRequest.Zone, "unexpected value")
	require.EqualValues(t, 123, api.listParameterRequest.ID, "unexpected value")
}

func TestIaaSDatabaseRejectsInvalidInputBeforeCallingAPI(t *testing.T) {
	api := &databaseAPITestDouble{}
	passwordFile := filepath.Join(t.TempDir(), "db-password.txt")
	require.NoError(t, os.WriteFile(passwordFile, []byte("secret-password\n"), 0o600))
	requestFile := filepath.Join(t.TempDir(), "db-create.json")
	require.NoError(t, os.WriteFile(requestFile, []byte(`{"Name":"db"}`), 0o600))
	updateFile := filepath.Join(t.TempDir(), "db-update.json")
	require.NoError(t, os.WriteFile(updateFile, []byte(`{"Description":"updated"}`), 0o600))

	for _, args := range [][]string{
		{"find", "--zone", "all"},
		{"find", "--zone", "test-zone", "--request", `{"Zone":"test-zone"}`},
		{"read", "--zone", "test-zone"},
		{"create", "--zone", "test-zone"},
		{"create", "--zone", "test-zone", "--name", "db", "--plan-id", "10"},
		{"create", "--zone", "test-zone", "--name", "db", "--plan-id", "10", "--switch-id", "112900500001"},
		{"create", "--zone", "test-zone", "--name", "db", "--plan-id", "10", "--switch-id", "112900500001", "--ip-address", "192.168.0.11"},
		{"create", "--zone", "test-zone", "--name", "db", "--plan-id", "10", "--switch-id", "112900500001", "--ip-address", "192.168.0.11", "--network-mask-len", "24"},
		{"create", "--zone", "test-zone", "--name", "db", "--plan-id", "10", "--switch-id", "112900500001", "--ip-address", "192.168.0.11", "--network-mask-len", "24", "--database-type", "mariadb", "--username", "dbuser", "--password", "@" + passwordFile, "--enable-replication"},
		{"create", "--zone", "test-zone", "--request", `{"Name":"db"}`},
		{"create", "--request", "@" + requestFile, "--name", "other"},
		// 空の配列指定も Kong が空スライスを設定するため、--request との併用として検出されます
		{"create", "--request", "@" + requestFile, "--tags="},
		{"create", "--request", "@" + requestFile, "--backup-weekday="},
		{"update"},
		{"update", "--zone", "test-zone"},
		{"update", "--zone", "test-zone", "--id", "123"},
		{"update", "--zone", "test-zone", "--id", "123", "--request", `{"Description":"updated"}`},
		{"update", "--zone", "all", "--id", "123", "--request", "@" + updateFile},
		{"monitor-cpu"},
		{"monitor-cpu", "--zone", "test-zone"},
		{"monitor-cpu", "--zone", "test-zone", "--id", "123", "--start", "invalid"},
		{"monitor-cpu", "--zone", "test-zone", "--request", `{"Zone":"test-zone"}`},
	} {
		commandLine := newCLI()
		setDatabaseAPIFactory(commandLine, func() (databaseapi.API, error) { return api, nil })
		var stdout, stderr bytes.Buffer
		code := runCLI(append([]string{"iaas-api", "database"}, args...), &stdout, &stderr, commandLine)
		require.NotEqual(t, 0, code, "%v: code %d stdout %q stderr %q; want an input error", args, code, stdout.String(), stderr.String())
		require.EqualValues(t, 0, stdout.Len(), "unexpected value")
		require.NotEqualValues(t, 0, stderr.Len(), "unexpected value")
	}
	require.Nil(t, api.findRequest, "invalid request reached API")
	require.Nil(t, api.createRequest, "invalid request reached API")
	require.Nil(t, api.updateRequest, "invalid request reached API")
	require.Nil(t, api.deleteRequest, "invalid request reached API")
	require.Nil(t, api.bootRequest, "invalid request reached API")
	require.Nil(t, api.shutdownRequest, "invalid request reached API")
	require.Nil(t, api.monitorCPURequest, "invalid request reached API")
}

func TestIaaSDatabasePropagatesAPIErrors(t *testing.T) {
	apiFailure := errors.New("Database API failure")
	api := &databaseAPITestDouble{createErr: apiFailure}
	commandLine := newCLI()
	setDatabaseAPIFactory(commandLine, func() (databaseapi.API, error) { return api, nil })
	passwordFile := filepath.Join(t.TempDir(), "db-password.txt")
	require.NoError(t, os.WriteFile(passwordFile, []byte("secret-password\n"), 0o600))
	var stdout, stderr bytes.Buffer
	code := runCLI([]string{"iaas-api", "database", "create", "--zone", "test-zone", "--name", "db-01",
		"--plan-id", "10", "--switch-id", "112900500001",
		"--ip-address", "192.168.0.11", "--network-mask-len", "24",
		"--database-type", "mariadb", "--username", "dbuser",
		"--password", "@" + passwordFile,
	}, &stdout, &stderr, commandLine)
	require.NotEqual(t, 0, code, "create error: code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	require.EqualValues(t, 0, stdout.Len(), "unexpected value")
	require.Contains(t, stderr.String(), apiFailure.Error(), "missing expected text %q", apiFailure.Error())
	require.NotContains(t, stderr.String(), "secret-password", "error output should not contain the password")
}

// databaseAPITestDouble は databaseapi.API の動作を置き換えるテスト用実装です。
type databaseAPITestDouble struct {
	databaseapi.API
	findRequest             *databaseapi.FindRequest
	createZone              string
	createRequest           *iaas.DatabaseCreateRequest
	updateZone              string
	updateID                types.ID
	updateRequest           *iaas.DatabaseUpdateRequest
	deleteRequest           *databaseapi.DeleteRequest
	bootRequest             *databaseapi.BootRequest
	shutdownRequest         *databaseapi.ShutdownRequest
	resetRequest            *databaseapi.ResetRequest
	monitorCPURequest       *databaseapi.MonitorCPURequest
	monitorDiskRequest      *databaseapi.MonitorDiskRequest
	monitorInterfaceRequest *databaseapi.MonitorInterfaceRequest
	monitorDatabaseRequest  *databaseapi.MonitorDatabaseRequest
	listParameterRequest    *databaseapi.ListParameterRequest
	createErr               error
}

func (api *databaseAPITestDouble) FindWithContext(_ context.Context, request *databaseapi.FindRequest) ([]*iaas.Database, error) {
	api.findRequest = request
	return []*iaas.Database{}, nil
}

func (api *databaseAPITestDouble) ReadWithContext(context.Context, *databaseapi.ReadRequest) (*iaas.Database, error) {
	return &iaas.Database{ID: 123}, nil
}

func (api *databaseAPITestDouble) Create(_ context.Context, zone string, request *iaas.DatabaseCreateRequest) (*iaas.Database, error) {
	api.createZone = zone
	api.createRequest = request
	if api.createErr != nil {
		return nil, api.createErr
	}
	return &iaas.Database{ID: 123, Name: request.Name}, nil
}

func (api *databaseAPITestDouble) Update(_ context.Context, zone string, id types.ID, request *iaas.DatabaseUpdateRequest) (*iaas.Database, error) {
	api.updateZone = zone
	api.updateID = id
	api.updateRequest = request
	return &iaas.Database{ID: 123, Name: request.Name}, nil
}

func (api *databaseAPITestDouble) DeleteWithContext(_ context.Context, request *databaseapi.DeleteRequest) error {
	api.deleteRequest = request
	return nil
}

func (api *databaseAPITestDouble) BootWithContext(_ context.Context, request *databaseapi.BootRequest) error {
	api.bootRequest = request
	return nil
}

func (api *databaseAPITestDouble) ShutdownWithContext(_ context.Context, request *databaseapi.ShutdownRequest) error {
	api.shutdownRequest = request
	return nil
}

func (api *databaseAPITestDouble) ResetWithContext(_ context.Context, request *databaseapi.ResetRequest) error {
	api.resetRequest = request
	return nil
}

func (api *databaseAPITestDouble) MonitorCPUWithContext(_ context.Context, request *databaseapi.MonitorCPURequest) ([]*iaas.MonitorCPUTimeValue, error) {
	api.monitorCPURequest = request
	return []*iaas.MonitorCPUTimeValue{}, nil
}

func (api *databaseAPITestDouble) MonitorDiskWithContext(_ context.Context, request *databaseapi.MonitorDiskRequest) ([]*iaas.MonitorDiskValue, error) {
	api.monitorDiskRequest = request
	return []*iaas.MonitorDiskValue{}, nil
}

func (api *databaseAPITestDouble) MonitorInterfaceWithContext(_ context.Context, request *databaseapi.MonitorInterfaceRequest) ([]*iaas.MonitorInterfaceValue, error) {
	api.monitorInterfaceRequest = request
	return []*iaas.MonitorInterfaceValue{}, nil
}

func (api *databaseAPITestDouble) MonitorDatabaseWithContext(_ context.Context, request *databaseapi.MonitorDatabaseRequest) ([]*iaas.MonitorDatabaseValue, error) {
	api.monitorDatabaseRequest = request
	return []*iaas.MonitorDatabaseValue{}, nil
}

func (api *databaseAPITestDouble) ListParameterWithContext(_ context.Context, request *databaseapi.ListParameterRequest) ([]*databaseapi.Parameter, error) {
	api.listParameterRequest = request
	return []*databaseapi.Parameter{}, nil
}
