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

package apprundedicated

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	v1 "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/v1"
	version "github.com/sacloud/sacloud-sdk-go/api/apprun-dedicated/apis/version"
)

// ApplicationDefinition は、AppRun 専有型アプリケーションのデプロイ対象を表す定義です。
//
// フィールド名は API のバージョン作成リクエスト (CreateApplicationVersionRequest) と
// 同じ camelCase です。cluster と name はデプロイ対象の識別子で、API リクエストには
// 含まれません。exposedPorts と env の要素は API と同じ型 (version.ExposedPort、
// version.EnvironmentVariable) を使います。
//
// 定義は ParseDefinition で JSON から読み込みます。registryPassword と secret=true の
// 環境変数の値は秘密情報のため、コマンドライン引数には渡さず、ファイルまたは標準入力
// から入力してください。
type ApplicationDefinition struct {
	// Cluster はデプロイ先クラスタの名前です。
	Cluster string `json:"cluster"`
	// Name はアプリケーションの名前です。クラスタの中で一意である必要があります。
	Name string `json:"name"`
	// CPU は割り当てる CPU です (単位: mCPU)。
	CPU int64 `json:"cpu,omitempty"`
	// Memory は割り当てるメモリです (単位: MB)。
	Memory int64 `json:"memory,omitempty"`
	// ScalingMode はスケーリング方式です。manual または cpu を指定します。
	ScalingMode v1.ScalingMode `json:"scalingMode,omitempty"`
	// FixedScale は manual 方式のインスタンス数です。
	FixedScale *int32 `json:"fixedScale,omitempty"`
	// MinScale は cpu 方式のインスタンス数の下限です。
	MinScale *int32 `json:"minScale,omitempty"`
	// MaxScale は cpu 方式のインスタンス数の上限です。
	MaxScale *int32 `json:"maxScale,omitempty"`
	// ScaleInThreshold は cpu 方式のスケールイン閾値です (単位: %)。
	ScaleInThreshold *int32 `json:"scaleInThreshold,omitempty"`
	// ScaleOutThreshold は cpu 方式のスケールアウト閾値です (単位: %)。
	ScaleOutThreshold *int32 `json:"scaleOutThreshold,omitempty"`
	// Image はコンテナイメージです。
	Image string `json:"image,omitempty"`
	// Cmd は起動コマンドです。
	Cmd []string `json:"cmd,omitempty"`
	// RegistryUsername はプライベートレジストリのユーザー名です。
	RegistryUsername *string `json:"registryUsername,omitempty"`
	// RegistryPassword はプライベートレジストリのパスワードです。秘密情報のため、
	// 出力やログには含めません。
	RegistryPassword *string `json:"registryPassword,omitempty"`
	// ExposedPorts は公開ポートの設定です。
	ExposedPorts []version.ExposedPort `json:"exposedPorts,omitempty"`
	// Env は環境変数です。secret=true の要素の値は API から取得できず、
	// このバージョンの作成で値を指定しなかった場合は前のバージョンの値が
	// 引き継がれます。
	Env []version.EnvironmentVariable `json:"env,omitempty"`
}

// ParseDefinition は定義の JSON バイト列を解釈して ApplicationDefinition を返します。
//
// 未知のフィールドや末尾の余分なデータはエラーにします。Jsonnet がなく単純な JSON
// であるため、typo を確実に検出できます。
func ParseDefinition(data []byte) (*ApplicationDefinition, error) {
	def := &ApplicationDefinition{}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(def); err != nil {
		return nil, fmt.Errorf("定義の JSON を解釈できません: %w", err)
	}
	var extra struct{}
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("定義の JSON の末尾に余分なデータがあります")
		}
		return nil, fmt.Errorf("定義の JSON の末尾に余分なデータがあります: %w", err)
	}
	return def, nil
}

// Validate は識別子として必要なフィールドを検証します。
//
// 参照・停止・削除系の操作 (status、versions、containers、rollback、deactivate、delete)
// はこの検証だけを行い、デプロイ内容を含まない定義でも読み込めるようにします。
func (d *ApplicationDefinition) Validate() error {
	if d.Cluster == "" {
		return errors.New("cluster は必須です")
	}
	if d.Name == "" {
		return errors.New("name は必須です")
	}
	return nil
}

// ValidateForDeploy はデプロイ系の操作 (deploy、render、diff) に必要なフィールドを
// 検証します。
//
// Validate に加えて、cpu、memory、scalingMode、image を必須とします。scalingMode が
// manual の場合は fixedScale を、cpu の場合は minScale と maxScale を必須とします。
// exposedPorts の要素で loadBalancerPort を指定した場合は host が必須、未指定の場合は
// host が指定不可であることを検証します。この検証は API の制約の再実装ではなく、
// CLI 経路で host の取り違えや欠落を確実に防ぐためのものです。
func (d *ApplicationDefinition) ValidateForDeploy() error {
	if err := d.Validate(); err != nil {
		return err
	}
	if d.CPU <= 0 {
		return errors.New("cpu には正の整数を指定してください")
	}
	if d.Memory <= 0 {
		return errors.New("memory には正の整数を指定してください")
	}
	switch d.ScalingMode {
	case v1.ScalingModeManual:
		if d.FixedScale == nil {
			return errors.New("scalingMode が manual のときは fixedScale が必須です")
		}
	case v1.ScalingModeCPU:
		if d.MinScale == nil || d.MaxScale == nil {
			return errors.New("scalingMode が cpu のときは minScale と maxScale が必須です")
		}
	case "":
		return errors.New("scalingMode は必須です")
	default:
		return fmt.Errorf("scalingMode は manual または cpu で指定してください: %q", d.ScalingMode)
	}
	if d.Image == "" {
		return errors.New("image は必須です")
	}
	for i, port := range d.ExposedPorts {
		switch {
		case port.LoadBalancerPort != nil && len(port.Host) == 0:
			return fmt.Errorf("exposedPorts[%d]: loadBalancerPort を指定したときは host が 1 件以上必要です", i)
		case port.LoadBalancerPort == nil && len(port.Host) > 0:
			return fmt.Errorf("exposedPorts[%d]: loadBalancerPort を指定しないときは host を指定できません", i)
		}
	}
	return nil
}

// createParams は定義からバージョン作成リクエストへ変換します。
// registryPasswordAction は deploy の文脈から service 層が決定します (Deploy を参照)。
func (d *ApplicationDefinition) createParams(action v1.RegistryPasswordAction) version.CreateParams {
	return version.CreateParams{
		CPU:                    d.CPU,
		Memory:                 d.Memory,
		ScalingMode:            d.ScalingMode,
		FixedScale:             d.FixedScale,
		MinScale:               d.MinScale,
		MaxScale:               d.MaxScale,
		ScaleInThreshold:       d.ScaleInThreshold,
		ScaleOutThreshold:      d.ScaleOutThreshold,
		Image:                  d.Image,
		Cmd:                    d.Cmd,
		RegistryUsername:       d.RegistryUsername,
		RegistryPassword:       d.RegistryPassword,
		RegistryPasswordAction: action,
		ExposedPorts:           d.ExposedPorts,
		EnvVars:                d.Env,
	}
}

// definitionFromVersionDetail はバージョンの詳細を定義と同じ形式に正規化します。
//
// diff の現状側と init の定義復元で使います。cluster と name は呼び出し元が指定します。
// registryPassword は応答でマスクされるため復元しません。secret=true の環境変数の値は
// API が null を返すため、nil のまま引き継がれます。
func definitionFromVersionDetail(detail *version.VersionDetail, clusterName, appName string) *ApplicationDefinition {
	return &ApplicationDefinition{
		Cluster:           clusterName,
		Name:              appName,
		CPU:               detail.CPU,
		Memory:            detail.Memory,
		ScalingMode:       detail.ScalingMode,
		FixedScale:        detail.FixedScale,
		MinScale:          detail.MinScale,
		MaxScale:          detail.MaxScale,
		ScaleInThreshold:  detail.ScaleInThreshold,
		ScaleOutThreshold: detail.ScaleOutThreshold,
		Image:             detail.Image,
		Cmd:               detail.Cmd,
		RegistryUsername:  detail.RegistryUsername,
		ExposedPorts:      detail.ExposedPorts,
		Env:               detail.EnvVars,
	}
}
