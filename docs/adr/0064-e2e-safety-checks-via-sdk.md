# ADR 0064: E2E の安全確認と起動待ちを SDK で直接実行する

- Status: Accepted
- Date: 2026-10-27

## Context

Server のライブ E2E では、テスト対象の API 操作以外に安全確認が必要でした。作成前の同名リソースの不在確認、後片付けでの削除前の再照合と削除後の不在確認、起動待ちのポーリングです。

これらの確認を skr CLI の呼び出し (`server find`、`server read`、`disk find`、`disk read`) で実装していた時期がありました。この構成では、確認ごとに実行記録が蓄積し、ページングや応答の JSON 解析も CLI 出力経由で行うため、テストの本質ではない部分が複雑になっていました。安全確認の実行そのものは、disk find の網羅確認 ([ADR 0058](0058-live-e2e-command-coverage.md)) が disk の E2E で担保するため、Server のシナリオから証跡を残す必要はありません。

## Decision

Server のライブ E2E において、安全確認と起動待ちを sacloud SDK (`sacloud-sdk-go`) で直接実行します。証跡は残しません。

- 作成前の同名確認 (Server と Disk)、後片付けの削除前の再照合と削除後の不在確認、孤立ディスクの参照は、SDK の `service/iaas/server` と `service/iaas/disk` を使います。`internal/iaas/client` のクライアント工場で組み立て、skr CLI と同じ認証設定 (プロファイル) を使います。
- 起動待ちのポーリング (サーバー状態の確認と SSH 接続の再試行待ち) も SDK で行い、待機の実行は証跡に残しません。
- テスト対象の操作は skr CLI で実行し、実行記録に説明付きで残します。Server のシナリオでは `server create`、`server read`、`server find --name`、`server boot`、`server update`、`server delete`、`disk delete` を残します。削除などリソースを実際に変更する操作は、実行したことを実行記録に残す経路で行います。
- `server find` と `server read` は作成結果の確認操作として CLI の検証ステップを1つずつ保持します ([ADR 0049](0049-internet-workflow-options.md)、[ADR 0058](0058-live-e2e-command-coverage.md))。
- シナリオの経路はライブ実行で検証します。2026-10-27 のテスト構成の整理で `serverCommands` インターフェースと偽の実装による単体テストは廃止し、実ロジックの単体テスト (解析・変換・識別確認・ページング) だけを残します。

この方針は Server の E2E に適用します。他のリソースの E2E を移行する場合は、この判断を参照しつつ、そのシナリオで証跡が必要な操作を個別に決めます。

## Consequences

- 実行記録にはテスト対象の CLI 操作だけが残り、安全確認によるノイズがなくなります。
- 安全確認の実行は実行記録から確認できません。確認の内容と条件はシナリオのテストコードと設計文書で追跡します。
- E2E テストが `internal/iaas/client` と sacloud SDK のサービスパッケージに依存します。SDK のメジャーアップデート時はテストも合わせて更新します。
- 途中ページの取得に失敗した場合の扱い (不在として扱わない) は、SDK を使う検索でも共通のページング処理で維持します。

## Alternatives considered

- 安全確認も skr CLI で実行し、実行記録に残す構成を維持する案では、確認ごとに実行記録が増え、ページングと JSON 解析のためにテストの本質ではないコードが大きくなります。
- 安全確認を CLI で実行しつつ実行記録から除外する案では、CLI 出力の解析は残る一方で記録の実装だけが複雑になり、扱いが一貫しません。
- Go の SSH ライブラリで起動待ちを含む確認をすべて置き換える案では、安全確認の範囲と SDK 依存の範囲が混ざります。本 ADR は確認の実行経路だけを対象とし、SSH 接続の確認は [ADR 0063](0063-reusable-ssh-and-local-command-recording.md) のとおり外部の ssh クライアントを使い続けます。
