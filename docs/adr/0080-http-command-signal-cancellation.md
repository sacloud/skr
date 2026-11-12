# ADR 0080: HTTP コマンドのシグナルによるキャンセル伝播と期限の方針

- Status: Accepted
- Date: 2026-11-12

## Context

`skr http` はリクエストを `http.NewRequestWithContext(context.Background(), ...)` で作成していたため、CLI プロセスが SIGINT や SIGTERM を受けても、実行中の HTTP リクエストへキャンセルが伝播しなかった。CLI 自体もシグナルハンドリングを持たず、SIGINT は Go ランタイムの既定動作でプロセスを即時終了させていた ([issue #87](https://github.com/sacloud/skr/issues/87))。

リクエストの期限は sacloud-sdk-go が管理する。調査時点の v0.5.0 では、`saclient.Client` は `http.Client.Timeout` に次の優先順位で値を設定する。

1. `SetWith` によるランタイム設定 (skr は使用しない)
2. コマンドライン引数 (skr は使用しない)
3. 環境変数 `SAKURA_API_REQUEST_TIMEOUT`、次に `SAKURACLOUD_API_REQUEST_TIMEOUT` (秒)
4. プロファイル設定の `api_request_timeout` (秒)。プロファイルに `0` が明記されている場合は `0` が採用され、タイムアウトが無効になる
5. SDK 既定の 300 秒

`http.Client.Timeout` は 1 回の試行の接続、送信、応答受信、本文の読み込みまでに適用される。リトライは `retryablehttp` が各試行で `http.Client.Do` を呼び出すため、期限は試行ごとに適用され、リトライ全体の期限は存在しない。リトライ待ちもリクエストの context を見るため、キャンセルされると待ち時間を待たずに中断する。

## Decision

- `skr http` は実行中に SIGINT と SIGTERM を受け、`signal.NotifyContext` で作成した context を HTTP リクエストへ伝播する。リクエストの送信、応答の受信、本文の読み込み、リトライの待ち時間はキャンセルで中断され、`(interrupted by signal)` を含むエラーとして報告する。
- シグナル待ちは `skr http` の実行中に限る。`Run` の戻りまでに `signal.Stop` 相当を解除し、他のコマンド実行中の SIGINT は引き続き Go ランタイムの既定動作 (プロセスの即時終了) で処理する。
- リトライ全体を含む全体期限は CLI 側で設けない。固定の全体タイムアウトは SDK の設定 (環境変数・プロファイル) を無条件に上書きするため、必要になった場合は設定経路と既定値を含めて別途設計する。

## Consequences

- 利用者は Ctrl+C で長時間のリクエストを中断でき、中断はエラーとして区別できる。
- リトライを含む実行全体の所要時間は、SDK のタイムアウト設定 (`SAKURA_API_REQUEST_TIMEOUT`、プロファイルの `api_request_timeout`) とリトライ設定 (`SAKURA_RETRY_MAX`、`SAKURA_RETRY_WAIT_MIN`、`SAKURA_RETRY_WAIT_MAX`) で利用者が制御する。SDK v0.5.0 の既定は、リクエストタイムアウト 300 秒、リトライ待ち 1〜64 秒、リトライ回数 10 回である。
- 他の API コマンド (SDK を直接呼び出すコマンド) へのキャンセル伝播は未対応で、[PLAN.md](../../PLAN.md) に改善候補として記録する。

## Alternatives considered

- 全コマンドへの伝播を同時に行う: kong の context バインドと、生成コード・手書きハンドラーを含む多数の SDK 呼び出し箇所の変更が必要で、この issue の範囲を超える。基盤の検討を含めて後続の作業に分割する。
- CLI 全体の固定タイムアウトを設ける: SDK の設定を上書きし、長時間の正当なリクエストを妨げる。issue が無条件の上書きを禁止しているため採用しない。
- キャンセルせず SIGINT の即時終了を維持する: プロセスは終了するが、キャンセルの伝播とエラー報告は行えず、遅いレスポンスやリトライ中の中断をテストで確認できない。
