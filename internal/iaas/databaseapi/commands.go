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
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/sacloud/sacloud-sdk-go/api/iaas"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/helper/power"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/helper/wait"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/search"
	"github.com/sacloud/sacloud-sdk-go/api/iaas/types"
	"github.com/sacloud/skr/internal/iaas/zones"
)

// databaseAPI は生成コマンドが使う API 実装です。
// すべての操作で api 層の DatabaseOp を直接呼び出し、service 層は使いません。
// 削除・起動・シャットダウンのワークフロー (強制終了と停止・起動待ち) もこの層で実装します。
type databaseAPI struct {
	op iaas.DatabaseAPI
}

// NewAPI は生成コマンドが使う API 実装を構築します。wiring の sdk_factory から呼ばれます。
func NewAPI(caller iaas.APICaller) API {
	return &databaseAPI{op: iaas.NewDatabaseOp(caller)}
}

// Find は api 層の DatabaseOp.Find を直接呼び出します。名前とタグの検索条件は
// api 層の FindCondition に変換します。単一名の部分一致検索は Names の1要素に変換します。
func (a *databaseAPI) FindWithContext(ctx context.Context, req *FindRequest) ([]*iaas.Database, error) {
	condition := &iaas.FindCondition{
		Filter: map[search.FilterKey]any{},
	}
	if len(req.Names) > 0 {
		condition.Filter[search.Key("Name")] = search.AndEqual(req.Names...)
	}
	if len(req.Tags) > 0 {
		condition.Filter[search.Key("Tags.Name")] = search.TagsAndEqual(req.Tags...)
	}
	condition.Sort = req.Sort
	condition.Count = req.Count
	condition.From = req.From
	found, err := a.op.Find(ctx, req.Zone, condition)
	if err != nil {
		return nil, err
	}
	return found.Databases, nil
}

// Read は api 層の DatabaseOp.Read を直接呼び出します。
func (a *databaseAPI) ReadWithContext(ctx context.Context, req *ReadRequest) (*iaas.Database, error) {
	return a.op.Read(ctx, req.Zone, req.ID)
}

// Delete は削除のワークフローを含めて api 層の DatabaseOp を呼び出します。
// 電源が起動中で Force が指定されていない場合はエラーにし、Force の場合は
// 強制終了してから停止を待ち、その後削除します。FailIfNotFound を指定しない場合、
// 対象が見つからないときはエラーにしません。
func (a *databaseAPI) DeleteWithContext(ctx context.Context, req *DeleteRequest) error {
	target, err := a.op.Read(ctx, req.Zone, req.ID)
	if err != nil {
		return handleNotFoundError(err, !req.FailIfNotFound)
	}
	if !req.Force && target.InstanceStatus.IsUp() {
		return fmt.Errorf("target %s:%q has not yet shut down", req.Zone, req.ID)
	}
	if target.InstanceStatus.IsUp() {
		if err := a.op.Shutdown(ctx, req.Zone, req.ID, &iaas.ShutdownOption{Force: true}); err != nil {
			return err
		}
		// 強制終了後に停止を待ちます
		if _, err := wait.UntilDatabaseIsDown(ctx, a.op, req.Zone, req.ID); err != nil {
			return err
		}
	} else if target.InstanceStatus != types.ServerInstanceStatuses.Unknown &&
		!(target.InstanceStatus == types.ServerInstanceStatuses.Down && target.Availability == types.Availabilities.Available) {
		// Unknown は状態が確認できないため待たずに削除します (移行前の service 層と同様)。
		// 停止済みでも可用性が準備中 (unknown や failed) の場合は待ちます。
		// Cleaning など停止処理中の状態も待ちます。可用性が available で停止済みの
		// 場合は待たずに削除します
		if _, err := wait.UntilDatabaseIsDown(ctx, a.op, req.Zone, req.ID); err != nil {
			return err
		}
	}
	if err := a.op.Delete(ctx, req.Zone, req.ID); err != nil {
		return handleNotFoundError(err, !req.FailIfNotFound)
	}
	return nil
}

// handleNotFoundError は対象が見つからない場合のエラーを、許可されているときは無視します。
func handleNotFoundError(err error, ignore bool) error {
	if ignore && iaas.IsNotFoundError(err) {
		return nil
	}
	return err
}

// Boot は起動のワークフローを含めて api 層の DatabaseOp を呼び出します。
// NoWait を指定しない場合は起動の完了を待ちます。
func (a *databaseAPI) BootWithContext(ctx context.Context, req *BootRequest) error {
	if req.NoWait {
		return a.op.Boot(ctx, req.Zone, req.ID)
	}
	return power.BootDatabase(ctx, a.op, req.Zone, req.ID)
}

// Shutdown は停止のワークフローを含めて api 層の DatabaseOp を呼び出します。
// NoWait を指定しない場合は停止の完了を待ちます。
func (a *databaseAPI) ShutdownWithContext(ctx context.Context, req *ShutdownRequest) error {
	if req.NoWait {
		return a.op.Shutdown(ctx, req.Zone, req.ID, &iaas.ShutdownOption{Force: req.ForceShutdown})
	}
	return power.ShutdownDatabase(ctx, a.op, req.Zone, req.ID, req.ForceShutdown)
}

// Reset は api 層の DatabaseOp.Reset を直接呼び出します。
func (a *databaseAPI) ResetWithContext(ctx context.Context, req *ResetRequest) error {
	return a.op.Reset(ctx, req.Zone, req.ID)
}

// MonitorCPU は api 層の DatabaseOp.MonitorCPU を直接呼び出します。
func (a *databaseAPI) MonitorCPUWithContext(ctx context.Context, req *MonitorCPURequest) ([]*iaas.MonitorCPUTimeValue, error) {
	condition, err := monitorCondition(req.Start, req.End)
	if err != nil {
		return nil, err
	}
	values, err := a.op.MonitorCPU(ctx, req.Zone, req.ID, condition)
	if err != nil {
		return nil, err
	}
	return values.Values, nil
}

// MonitorDisk は api 層の DatabaseOp.MonitorDisk を直接呼び出します。
func (a *databaseAPI) MonitorDiskWithContext(ctx context.Context, req *MonitorDiskRequest) ([]*iaas.MonitorDiskValue, error) {
	condition, err := monitorCondition(req.Start, req.End)
	if err != nil {
		return nil, err
	}
	values, err := a.op.MonitorDisk(ctx, req.Zone, req.ID, condition)
	if err != nil {
		return nil, err
	}
	return values.Values, nil
}

// MonitorInterface は api 層の DatabaseOp.MonitorInterface を直接呼び出します。
func (a *databaseAPI) MonitorInterfaceWithContext(ctx context.Context, req *MonitorInterfaceRequest) ([]*iaas.MonitorInterfaceValue, error) {
	condition, err := monitorCondition(req.Start, req.End)
	if err != nil {
		return nil, err
	}
	values, err := a.op.MonitorInterface(ctx, req.Zone, req.ID, condition)
	if err != nil {
		return nil, err
	}
	return values.Values, nil
}

// MonitorDatabase は api 層の DatabaseOp.MonitorDatabase を直接呼び出します。
func (a *databaseAPI) MonitorDatabaseWithContext(ctx context.Context, req *MonitorDatabaseRequest) ([]*iaas.MonitorDatabaseValue, error) {
	condition, err := monitorCondition(req.Start, req.End)
	if err != nil {
		return nil, err
	}
	values, err := a.op.MonitorDatabase(ctx, req.Zone, req.ID, condition)
	if err != nil {
		return nil, err
	}
	return values.Values, nil
}

// monitorCondition は取得期間の未指定値を既定に補完し、開始と終了の整合を検証します。
// 終了の未指定は現在時刻、開始の未指定は終了の 1 時間前です。
func monitorCondition(start, end time.Time) (*iaas.MonitorCondition, error) {
	e := end
	if e.IsZero() {
		e = time.Now()
	}
	s := start
	if s.IsZero() {
		s = e.Add(-1 * time.Hour)
	}
	if !(s.Unix() <= e.Unix()) {
		return nil, fmt.Errorf("start(%s) or end(%s) is invalid", start.String(), end.String())
	}
	return &iaas.MonitorCondition{Start: s, End: e}, nil
}

// ListParameter は api 層の DatabaseOp.GetParameter を直接呼び出し、
// パラメータのメタ情報と現在の設定値を対応付けて返します。
func (a *databaseAPI) ListParameterWithContext(ctx context.Context, req *ListParameterRequest) ([]*Parameter, error) {
	parameters, err := a.op.GetParameter(ctx, req.Zone, req.ID)
	if err != nil {
		return nil, err
	}
	var results []*Parameter
	for _, p := range parameters.MetaInfo {
		var setting any
		for k, v := range parameters.Settings {
			if p.Name == k {
				setting = v
				break
			}
		}
		results = append(results, &Parameter{
			Key:   p.Label,
			Value: setting,
			Meta:  p,
		})
	}
	return results, nil
}

// Create は api 層の DatabaseOp を直接呼びます。起動の完了待ちなどのワークフローは
// この層では行わず、リクエストをそのまま API に送ります。
func (a *databaseAPI) Create(ctx context.Context, zone string, param *iaas.DatabaseCreateRequest) (*iaas.Database, error) {
	return a.op.Create(ctx, zone, param)
}

// Update は api 層の DatabaseOp を直接呼びます。現在値の読み取りと送信内容の
// マージは行わないため、送信した内容で設定が置き換わります。
func (a *databaseAPI) Update(ctx context.Context, zone string, id types.ID, param *iaas.DatabaseUpdateRequest) (*iaas.Database, error) {
	return a.op.Update(ctx, zone, id, param)
}

// CreateCommand はデータベースアプライアンスの作成コマンドです。
// 作成リクエストにユーザー名とパスワード (API の Settings.DBConf.Common の既定ユーザー)
// という秘密情報が含まれるため、個別フラグと --request JSON の両経路を手書きで実装しています。
// 秘密情報の値は引数に指定できず、保護したファイルまたは標準入力から読み込みます。
type CreateCommand struct {
	Request *string `help:"API リクエスト JSON を @path.json または - (標準入力) で指定します。個別フラグと併用できません。パスワードを含むため、JSON を直接書く方法は使えません。必須: Zone、Name、PlanID、SwitchID、IPAddresses、NetworkMaskLen、DatabaseType、Username、Password。任意: Description、Tags、IconID、DefaultRoute、Port、SourceNetwork、EnableReplication、ReplicaUserPassword、EnableWebUI、EnableBackup、BackupWeekdays、BackupStartTimeHour、BackupStartTimeMinute、EnableBackupv2、Backupv2Weekdays、Backupv2StartTimeHour、Backupv2StartTimeMinute、Backupv2Connect、EnableMonitoringSuite、DiskEncryptionAlgorithm、DiskEncryptionKMSKey。RDBMS 固有のパラメータは Parameters で指定します。起動の完了は待ちません。"`

	Zone        string   `name:"zone" help:"必須: 作成先ゾーン名。例: --zone tk1v"`
	Name        *string  `name:"name" help:"必須: データベースアプライアンスの名前。"`
	Description *string  `name:"description" help:"任意: 説明 (最大512文字)。"`
	Tags        []string `name:"tags" sep:"," help:"任意: タグの配列。カンマ区切りで指定します。API の Tags フィールドに対応します。"`
	IconID      *int64   `name:"icon-id" help:"任意: アイコンの ID (数値)。"`
	PlanID      *int64   `name:"plan-id" help:"必須: データベースプランの ID (数値)。10: 10GB (1core-2gb)、30: 30GB (2core-4gb)、90: 90GB (4core-8gb)、240: 240GB (8core-16gb)、500: 500GB、1000: 1TB。例: --plan-id 10"`
	SwitchID    *int64   `name:"switch-id" help:"必須: 接続先スイッチの ID (数値)。スイッチ単体が対象で、ルータ＋スイッチでは利用できません。iaas-api switch list で確認できます。"`

	IPAddresses    []string `name:"ip-address" help:"必須: データベースアプライアンスに割り当てる IPv4 アドレス。1個または2個 (レプリケーション時) 指定します。例: --ip-address 192.168.0.11"`
	NetworkMaskLen *int     `name:"network-mask-len" help:"必須: ネットワークのサブネットマスク長。1 から 32 の整数。例: 24"`
	DefaultRoute   *string  `name:"default-route" help:"任意: デフォルトルート (IPv4 アドレス)。"`
	Port           *int     `name:"port" help:"任意: 接続用ポート番号。省略すると RDBMS の既定ポートを使います。1 から 65535 の整数。"`
	SourceNetwork  []string `name:"source-network" help:"任意: 接続を許可する送信元ネットワーク (CIDR 形式の IPv4)。例: --source-network 192.168.0.0/24"`

	DatabaseType    *string `name:"database-type" help:"必須: RDBMS の種別。mariadb または postgres。"`
	DatabaseVersion *string `name:"database-version" help:"任意: RDBMS のバージョン。省略時は提供中の既定バージョンになります。"`
	Username        *string `name:"username" help:"必須: 既定ユーザーのユーザー名。"`
	Password        *string `name:"password" help:"必須: 既定ユーザーのパスワード。@path でファイルから、- で標準入力から読み込みます。インライン値は指定できません。"`

	EnableReplication   *bool   `name:"enable-replication" help:"任意: レプリケーションを有効にします。有効時は --replica-user-password が必要で、--ip-address を2個指定します。"`
	ReplicaUserPassword *string `name:"replica-user-password" help:"レプリケーション有効時に必須: レプリケーションユーザーのパスワード。@path でファイルから、- で標準入力から読み込みます。インライン値は指定できません。"`
	EnableWebUI         *bool   `name:"enable-web-ui" help:"任意: Web UI (Admin UI) を有効にします。"`

	EnableBackup          *bool    `name:"enable-backup" help:"任意: バックアップを有効にします。有効時は --backup-weekday の指定を推奨します。"`
	BackupWeekdays        []string `name:"backup-weekday" sep:"," help:"任意: バックアップ取得日 (sun, mon, tue, wed, thu, fri, sat)。カンマ区切りで指定します。例: --backup-weekday mon,tue"`
	BackupStartTimeHour   *int     `name:"backup-start-time-hour" help:"任意: バックアップ開始時刻の時 (0-23)。分は 15 分単位です。"`
	BackupStartTimeMinute *int     `name:"backup-start-time-minute" help:"任意: バックアップ開始時刻の分 (0, 15, 30, 45)。"`

	EnableBackupv2          *bool    `name:"enable-backup-v2" help:"任意: Backup v2 を有効にします。有効時は --backup-v2-weekday と --backup-v2-connect が必要です。"`
	Backupv2Weekdays        []string `name:"backup-v2-weekday" sep:"," help:"Backup v2 有効時に必須: バックアップ取得日 (sun, mon, tue, wed, thu, fri, sat)。カンマ区切りで指定します。"`
	Backupv2StartTimeHour   *int     `name:"backup-v2-start-time-hour" help:"任意: Backup v2 開始時刻の時 (0-23)。"`
	Backupv2StartTimeMinute *int     `name:"backup-v2-start-time-minute" help:"任意: Backup v2 開始時刻の分 (0, 15, 30, 45)。"`
	Backupv2Connect         *string  `name:"backup-v2-connect" help:"Backup v2 有効時に必須: バックアップの接続方式。"`

	EnableMonitoringSuite   *bool   `name:"enable-monitoring-suite" help:"任意: モニタリングスイート連携を有効にします (有償オプション)。"`
	DiskEncryptionAlgorithm *string `name:"disk-encryption-algorithm" help:"任意: ディスク暗号化のアルゴリズム。aes256_xts。KMS 鍵とともに指定します。"`
	DiskEncryptionKMSKey    *int64  `name:"disk-encryption-kms-key" help:"任意: 暗号化に使う KMS 鍵の ID (数値)。--disk-encryption-algorithm とともに指定します。"`

	factory APIFactory
	runtime Runtime
}

// Run は作成リクエストを組み立てて実行します。起動の完了は待ちません。
func (c *CreateCommand) Run(ctx *kong.Context) error {
	if err := zones.ValidateSingleZone(c.Zone); err != nil {
		return err
	}
	flagsSet := c.flagsSet()
	if c.Request != nil && flagsSet {
		return fmt.Errorf("--request と個別フラグは併用できません")
	}
	var request *iaas.DatabaseCreateRequest
	if c.Request != nil {
		if c.runtime.DecodeRequest == nil {
			return fmt.Errorf("API リクエスト処理が設定されていません")
		}
		// リクエストにはパスワードが含まれるため、ファイルまたは標準入力からのみ読み込みます
		if *c.Request != "-" && (len(*c.Request) == 0 || (*c.Request)[0] != '@') {
			return fmt.Errorf("--request には @path.json または - (標準入力) を指定してください")
		}
		input := *c.Request
		if input == "-" {
			// 標準入力から JSON を読み込みます
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("--request の標準入力読み込みに失敗しました: %w", err)
			}
			input = string(data)
		}
		request = &iaas.DatabaseCreateRequest{}
		if err := c.runtime.DecodeRequest(input, request); err != nil {
			return err
		}
	} else {
		if !flagsSet {
			return fmt.Errorf("--request または個別フラグが必要です")
		}
		var err error
		request, err = c.buildRequest(os.Stdin)
		if err != nil {
			return err
		}
	}
	if err := c.runtime.ValidateOutput(ctx); err != nil {
		return err
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	result, err := op.Create(context.Background(), c.Zone, request)
	if err != nil {
		return err
	}
	return c.runtime.WriteOutput(ctx, result)
}

// flagsSet は個別フラグが1つでも明示的に指定されたかを返します。
// フラグはポインタ型で、Kong は明示的に指定されたときだけ値を設定するため、
// false・0・空文字列の明示も検出できます。スライス型は値型のまま、
// Kong が指定時に空スライスを設定する挙動に合わせて nil チェックで検出します
// (--tags= のような空の指定も検出対象です)。
func (c *CreateCommand) flagsSet() bool {
	return c.Name != nil ||
		c.Description != nil ||
		c.Tags != nil ||
		c.IconID != nil ||
		c.PlanID != nil ||
		c.SwitchID != nil ||
		c.IPAddresses != nil ||
		c.NetworkMaskLen != nil ||
		c.DefaultRoute != nil ||
		c.Port != nil ||
		c.SourceNetwork != nil ||
		c.DatabaseType != nil ||
		c.DatabaseVersion != nil ||
		c.Username != nil ||
		c.Password != nil ||
		c.EnableReplication != nil ||
		c.ReplicaUserPassword != nil ||
		c.EnableWebUI != nil ||
		c.EnableBackup != nil ||
		c.BackupWeekdays != nil ||
		c.BackupStartTimeHour != nil ||
		c.BackupStartTimeMinute != nil ||
		c.EnableBackupv2 != nil ||
		c.Backupv2Weekdays != nil ||
		c.Backupv2StartTimeHour != nil ||
		c.Backupv2StartTimeMinute != nil ||
		c.Backupv2Connect != nil ||
		c.EnableMonitoringSuite != nil ||
		c.DiskEncryptionAlgorithm != nil ||
		c.DiskEncryptionKMSKey != nil
}

// buildRequest は個別フラグから API の作成リクエストを組み立て、必須項目と制約を検証します。
func (c *CreateCommand) buildRequest(stdin io.Reader) (*iaas.DatabaseCreateRequest, error) {
	if c.Name == nil {
		return nil, fmt.Errorf("--name が必要です")
	}
	if c.PlanID == nil {
		return nil, fmt.Errorf("--plan-id が必要です")
	}
	if c.SwitchID == nil {
		return nil, fmt.Errorf("--switch-id が必要です")
	}
	if len(c.IPAddresses) == 0 {
		return nil, fmt.Errorf("--ip-address が必要です")
	}
	if len(c.IPAddresses) > 2 {
		return nil, fmt.Errorf("--ip-address は2個まで指定できます")
	}
	for i, ip := range c.IPAddresses {
		parsed := net.ParseIP(ip)
		if parsed == nil || parsed.To4() == nil {
			return nil, fmt.Errorf("--ip-address の %d 個目は IPv4 アドレスで指定してください: %s", i+1, ip)
		}
	}
	if c.NetworkMaskLen == nil {
		return nil, fmt.Errorf("--network-mask-len が必要です")
	}
	if *c.NetworkMaskLen < 1 || *c.NetworkMaskLen > 32 {
		return nil, fmt.Errorf("--network-mask-len は 1 から 32 の範囲で指定してください")
	}
	if c.DatabaseType == nil {
		return nil, fmt.Errorf("--database-type が必要です")
	}
	if typ := types.RDBMSTypeFromString(*c.DatabaseType); typ != types.RDBMSTypesMariaDB && typ != types.RDBMSTypesPostgreSQL {
		return nil, fmt.Errorf("--database-type は mariadb または postgres で指定してください")
	}
	if c.Username == nil {
		return nil, fmt.Errorf("--username が必要です")
	}
	if c.Password == nil {
		return nil, fmt.Errorf("--password が必要です")
	}
	password, err := readSecret(*c.Password, stdin, "--password")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(password) == "" {
		return nil, fmt.Errorf("--password が空です")
	}
	if port := derefValue(c.Port); port < 0 || port > 65535 {
		return nil, fmt.Errorf("--port は 1 から 65535 の範囲で指定してください")
	}
	backupWeekdays, err := parseWeekdays(c.BackupWeekdays, "--backup-weekday")
	if err != nil {
		return nil, err
	}
	if hour := derefValue(c.BackupStartTimeHour); hour < 0 || hour > 23 {
		return nil, fmt.Errorf("--backup-start-time-hour は 0 から 23 の範囲で指定してください")
	}
	if minute := derefValue(c.BackupStartTimeMinute); minute != 0 && minute != 15 && minute != 30 && minute != 45 {
		return nil, fmt.Errorf("--backup-start-time-minute は 0, 15, 30, 45 のいずれかで指定してください")
	}
	replicaPassword := ""
	if derefValue(c.EnableReplication) {
		if c.ReplicaUserPassword == nil {
			return nil, fmt.Errorf("--enable-replication を指定した場合は --replica-user-password が必要です")
		}
		replicaPassword, err = readSecret(*c.ReplicaUserPassword, stdin, "--replica-user-password")
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(replicaPassword) == "" {
			return nil, fmt.Errorf("--replica-user-password が空です")
		}
	} else if c.ReplicaUserPassword != nil {
		return nil, fmt.Errorf("--replica-user-password は --enable-replication とともに指定してください")
	}
	backupv2Weekdays, err := parseWeekdays(c.Backupv2Weekdays, "--backup-v2-weekday")
	if err != nil {
		return nil, err
	}
	if hour := derefValue(c.Backupv2StartTimeHour); hour < 0 || hour > 23 {
		return nil, fmt.Errorf("--backup-v2-start-time-hour は 0 から 23 の範囲で指定してください")
	}
	if minute := derefValue(c.Backupv2StartTimeMinute); minute != 0 && minute != 15 && minute != 30 && minute != 45 {
		return nil, fmt.Errorf("--backup-v2-start-time-minute は 0, 15, 30, 45 のいずれかで指定してください")
	}
	if derefValue(c.EnableBackupv2) {
		if len(backupv2Weekdays) == 0 {
			return nil, fmt.Errorf("--enable-backup-v2 を指定した場合は --backup-v2-weekday が必要です")
		}
		if c.Backupv2Connect == nil {
			return nil, fmt.Errorf("--enable-backup-v2 を指定した場合は --backup-v2-connect が必要です")
		}
	}
	// ディスク暗号化はアルゴリズムと KMS 鍵の両方が必要です
	if (c.DiskEncryptionAlgorithm != nil) != (c.DiskEncryptionKMSKey != nil) {
		return nil, fmt.Errorf("--disk-encryption-algorithm と --disk-encryption-kms-key は両方指定してください")
	}
	// API のディスク暗号化アルゴリズムは aes256_xts です。対応する値だけを受け付けます
	var encryptionAlgorithm types.EDiskEncryptionAlgorithm
	if c.DiskEncryptionAlgorithm != nil {
		algorithm, ok := types.DiskEncryptionAlgorithmMap[*c.DiskEncryptionAlgorithm]
		if !ok || algorithm != types.DiskEncryptionAlgorithms.AES256XTS {
			return nil, fmt.Errorf("--disk-encryption-algorithm は aes256_xts で指定してください")
		}
		encryptionAlgorithm = algorithm
	}

	// 以下は API のリクエストフィールドへの対応です。RDBMS の種別は Conf.DatabaseName、
	// ユーザー名とパスワードは CommonSetting の既定ユーザー (DefaultUser/UserPassword) に対応します。
	commonSetting := &iaas.DatabaseSettingCommon{
		WebUI:         types.ToWebUI(derefValue(c.EnableWebUI)),
		ServicePort:   derefValue(c.Port),
		SourceNetwork: c.SourceNetwork,
		DefaultUser:   *c.Username,
		UserPassword:  password,
	}
	if derefValue(c.EnableReplication) {
		commonSetting.ReplicaUser = "replica"
		commonSetting.ReplicaPassword = replicaPassword
	}
	request := &iaas.DatabaseCreateRequest{
		Name:           *c.Name,
		Description:    derefValue(c.Description),
		Tags:           types.Tags(c.Tags),
		IconID:         types.ID(derefValue(c.IconID)),
		PlanID:         types.ID(*c.PlanID),
		SwitchID:       types.ID(*c.SwitchID),
		IPAddresses:    c.IPAddresses,
		NetworkMaskLen: *c.NetworkMaskLen,
		DefaultRoute:   derefValue(c.DefaultRoute),
		Conf: &iaas.DatabaseRemarkDBConfCommon{
			DatabaseName:    types.RDBMSTypeFromString(*c.DatabaseType).String(),
			DatabaseVersion: derefValue(c.DatabaseVersion),
		},
		CommonSetting:   commonSetting,
		MonitoringSuite: &iaas.MonitoringSuite{Enabled: derefValue(c.EnableMonitoringSuite)},
	}
	if derefValue(c.EnableBackup) {
		request.BackupSetting = &iaas.DatabaseSettingBackup{
			Time:      fmt.Sprintf("%02d:%02d", derefValue(c.BackupStartTimeHour), derefValue(c.BackupStartTimeMinute)),
			DayOfWeek: backupWeekdays,
			Rotate:    8,
		}
	}
	if derefValue(c.EnableBackupv2) {
		request.Backupv2Setting = &iaas.DatabaseSettingBackupv2{
			Time:      fmt.Sprintf("%02d:%02d", derefValue(c.Backupv2StartTimeHour), derefValue(c.Backupv2StartTimeMinute)),
			DayOfWeek: backupv2Weekdays,
			Connect:   *c.Backupv2Connect,
			Rotate:    8,
		}
	}
	if derefValue(c.EnableReplication) {
		request.ReplicationSetting = &iaas.DatabaseReplicationSetting{
			Model: types.DatabaseReplicationModels.MasterSlave,
		}
	}
	if c.DiskEncryptionAlgorithm != nil && c.DiskEncryptionKMSKey != nil {
		request.Disk = &iaas.DatabaseDisk{
			EncryptionAlgorithm: encryptionAlgorithm,
			EncryptionKeyID:     types.ID(*c.DiskEncryptionKMSKey),
		}
	}
	return request, nil
}

// validWeekdays はバックアップ取得日として有効な曜日です。
// types.DayOfTheWeekFromString は不正な値を月曜に置き換えるため、
// CLI では明示的に検証してから変換します。
var validWeekdays = map[string]bool{
	"sun": true, "mon": true, "tue": true, "wed": true,
	"thu": true, "fri": true, "sat": true,
}

// UpdateCommand はデータベースアプライアンスの更新コマンドです。
// リクエスト JSON にパスワード (CommonSetting の UserPassword/ReplicaPassword など)
// が含まれる可能性があるため、JSON は保護したファイルまたは標準入力からのみ読み込みます。
// 更新は api 層の DatabaseOp を直接呼び、現在値の読み取りや送信内容のマージを行いません。
// 送信した内容で設定が置き換わるため、保持したい項目も含めて指定してください。
type UpdateCommand struct {
	Request *string `help:"API リクエスト JSON を @path.json または - (標準入力) で指定します。直接 JSON を書く方法は使えません。API の更新リクエストのフィールド: Name、Description、Tags、IconID、CommonSetting (WebUI、ServicePort、SourceNetwork、DefaultUser、UserPassword、ReplicaUser、ReplicaPassword)、BackupSetting (Time、DayOfWeek、Connect、Rotate)、Backupv2Setting (Time、DayOfWeek、Connect、Rotate)、ReplicationSetting (Model、IPAddress、Port、User、Password)、InterfaceSettings、MonitoringSuite (Enabled)。送信した内容で設定が置き換わるため、保持したい項目も含めて指定してください。read の応答の UserPassword と ReplicaPassword はマスク済みの値で返るため、実際のパスワードを指定してください。パスワードを含む JSON は保護したファイルか標準入力から渡してください。例: --request @db-update.json"`

	Zone string `name:"zone" help:"必須: 対象ゾーン名。例: --zone tk1v"`
	ID   int64  `name:"id" help:"必須: 更新するデータベースアプライアンスの ID (数値)。作成結果や find の出力にある ID です。"`

	factory APIFactory
	runtime Runtime
}

func (c *UpdateCommand) Run(ctx *kong.Context) error {
	if err := zones.ValidateSingleZone(c.Zone); err != nil {
		return err
	}
	if c.Request == nil {
		return fmt.Errorf("--request が必要です")
	}
	// リクエストにはパスワードが含まれる可能性があるため、ファイルまたは標準入力からのみ読み込みます
	if *c.Request != "-" && (len(*c.Request) == 0 || (*c.Request)[0] != '@') {
		return fmt.Errorf("--request には @path.json または - (標準入力) を指定してください")
	}
	if c.ID == 0 {
		return fmt.Errorf("--id が必要です")
	}
	if c.runtime.DecodeRequest == nil {
		return fmt.Errorf("API リクエスト処理が設定されていません")
	}
	input := *c.Request
	if input == "-" {
		// 標準入力から JSON を読み込みます
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("--request の標準入力読み込みに失敗しました: %w", err)
		}
		input = string(data)
	}
	request := &iaas.DatabaseUpdateRequest{}
	if err := c.runtime.DecodeRequest(input, request); err != nil {
		return err
	}
	if c.runtime.ValidateOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	if err := c.runtime.ValidateOutput(ctx); err != nil {
		return err
	}
	if c.factory == nil {
		return fmt.Errorf("API クライアントが設定されていません")
	}
	op, err := c.factory()
	if err != nil {
		return err
	}
	result, err := op.Update(context.Background(), c.Zone, types.ID(c.ID), request)
	if err != nil {
		return err
	}
	if c.runtime.WriteOutput == nil {
		return fmt.Errorf("API 出力処理が設定されていません")
	}
	return c.runtime.WriteOutput(ctx, result)
}

// parseWeekdays は曜日の指定を検証して API の値に変換します。
func parseWeekdays(input []string, flag string) ([]types.EDayOfTheWeek, error) {
	if len(input) == 0 {
		return nil, nil
	}
	days := make([]types.EDayOfTheWeek, 0, len(input))
	for i, day := range input {
		if !validWeekdays[day] {
			return nil, fmt.Errorf("%s の %d 個目は sun, mon, tue, wed, thu, fri, sat のいずれかで指定してください: %s", flag, i+1, day)
		}
		days = append(days, types.DayOfTheWeekFromString(day))
	}
	return days, nil
}

// SetMonitorDatabasePeriod は取得期間が未指定の場合に、秒精度の既定期間 (直近 1 時間) を設定します。
// データベースのモニタリング API はナノ秒精度の時刻を受け付けないため、
// 未指定時に設定する現在時刻の値を秒精度に丸めます。
func SetMonitorDatabasePeriod(request *MonitorDatabaseRequest) error {
	if request.End.IsZero() {
		request.End = time.Now().Truncate(time.Second)
	}
	if request.Start.IsZero() {
		request.Start = request.End.Add(-1 * time.Hour)
	}
	return nil
}

// derefValue は nil のときにゼロ値を返します。省略したフラグをリクエストに
// 展開するときに使います。
func derefValue[T any](v *T) T {
	if v == nil {
		var zero T
		return zero
	}
	return *v
}

// readSecret は秘密情報を @path または標準入力から読み込みます。
// インライン値は受け付けず、シェルの履歴に秘密情報が残らないようにします。
func readSecret(input string, stdin io.Reader, flag string) (string, error) {
	var data []byte
	switch {
	case input == "-":
		var err error
		data, err = io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read %s from standard input: %w", flag, err)
		}
	case strings.HasPrefix(input, "@") && len(input) > 1:
		var err error
		data, err = os.ReadFile(strings.TrimPrefix(input, "@"))
		if err != nil {
			return "", fmt.Errorf("read %s file: %w", flag, err)
		}
	default:
		return "", fmt.Errorf("%s は @path または - で指定してください。インライン値は指定できません", flag)
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}
