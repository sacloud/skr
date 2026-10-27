# ADR 0063: E2E で再利用する SSH 接続確認とローカルコマンドの実行記録

- Status: Accepted
- Date: 2026-10-27

## Context

Server や GSLB など、一時的な Server の作成を伴うライブ E2E では、SSH 接続の確認が必要でした。旧 runner は runner ごとに `createSSHMaterial` (鍵の生成と cloud-init の user-data の作成) と `sshRunner` (SSH ログインの確認) を複製していました。

E2E では skr CLI 以外の検証 (SSH 接続や DNS 問い合わせ) も行うため、その証跡を実行記録に残す仕組みも runner 固有のものでした。Server の E2E を testing.T ベースに移行する ([ADR 0062](0062-e2e-testing-t-based-scenario.md)) 際に、SSH 系の処理を runner ごとに複製しない構成が必要でした。

## Decision

`test/e2e/internal/helper` パッケージに、Server を作成する E2E で再利用する SSH 系の処理を配置します。

- `helper.WriteSSHMaterial` は ssh-keygen で一時的な鍵ペアを生成し、指定したユーザーでログインするための cloud-init user-data を作成します。公開鍵だけを user-data に記載し、秘密鍵の内容は含めません。sudo を設定すると NOPASSWD の sudo 権限を含めます。秘密鍵と user-data は所有者のみ読める一時ファイルに保存し、テスト終了時に削除します。
- `helper.ConnectSSH` は一時的な鍵でリモートの `hostname` を取得し、ログインできることを確認します。`helper.RunSSHCommand` はリモートコマンドを1回実行し、stdout を返します。
- `helper.RunLocalCommand` は skr 以外のローカルコマンドを実行し、実行結果を実行記録へ保存します。実行記録にはコマンド名 (ベース名)、引数、stdout、stderr、所要時間を記録し、skr CLI の記録と同じ形式にします。

SSH はバッチモードで対話を禁止し、`IdentitiesOnly` で一時的な鍵だけを使い、未知のホスト鍵はテスト専用の known_hosts ファイルに記録します。リモートコマンドは固定された引数として渡され、ローカルシェルを経由しません。

Server の E2E は SSH の確認を含む操作を helper 経由で実行します。2026-10-27 のテスト構成の整理でシナリオの偽の実装は廃止し、シナリオの経路はライブ実行で検証します。

## Consequences

- Server 以外のリソースを扱う E2E (GSLB の health-check Server など) も、鍵の生成と SSH 接続の確認を runner ごとに複製せず再利用できます。
- SSH の接続試行がすべて実行記録に残るため、cloud-init の反映待ちの経過と接続失敗の原因を実行記録から確認できます。
- ssh-keygen と ssh クライアントに依存します。helper の単体テストは ssh-keygen が利用できない環境ではスキップします。
- 一時的な鍵はテストの終了時に削除されますが、リモートの cloud-init で登録されたユーザーは Server の削除とともに消えます。

## Alternatives considered

- runner ごとに SSH 処理を複製する構成を維持する案では、Server 以外の E2E を移行するたびに同じ処理を書き直すことになり、秘密情報の扱いの差異も生まれます。
- SSH 接続の確認を Go の SSH ライブラリ (`golang.org/x/crypto/ssh`) で実装する案では、ホスト鍵の検証と認証の組み立てを独自に管理することになります。外部の ssh クライアントをバッチモードで使う方が、ホスト鍵の記録と接続の挙動が標準的です。
- SSH の実行記録を CLI の記録と別の形式にする案では、実行記録の閲覧が2種類になります。既存の実行記録形式に統一する方が確認が簡単です。
