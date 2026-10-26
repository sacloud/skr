# ADR 0060: E2E runner の共通ライブラリ化とシナリオ分離

- Status: Superseded by [ADR 0061](0061-e2e-testing-t-based-scenario.md)
- Date: 2026-10-26

## Context

`test/e2e/container-registry-api/main.go` や `test/e2e/c0nfig/main.go` では、skr バイナリの実行と証跡記録、一時ディレクトリの後片付け、乱数サフィックスの生成といったボイラープレートと、実際のシナリオが同一ファイルに混在していました。実際のシナリオとは、どのコマンドをどの順序で実行し、何を確認するのかというテストの流れです。このため、シナリオファイルを開いてもテストの流れが把握しにくく、共有処理も runner ごとに重複していました。

各 runner にはすでに `test/e2e/internal/evidence` という共有パッケージがありますが、CLI 実行そのもの (`cliRunner`) は runner ごとに手書きされており、シグネチャやインターフェースも揃っていませんでした。

## Decision

- 共有ライブラリを `test/e2e/internal/e2e` パッケージに配置します。既存の `test/e2e/internal/evidence` と同じ `test/e2e/internal/` 配下とし、E2E 配下でのみ参照可能な内部パッケージとします。
- 共有ライブラリは次の機構を提供します。
  - `Runner`: skr バイナリの実行と `evidence.Recorder` への記録を担います。`Call` (トップレベルコマンド) と `CallSubcommand` (`WithPrefix` で設定した接頭辞付きサブコマンド) を提供し、`--request @path` のリクエストファイル内容も記録します。
  - `WriteRequest`: `@path` 形式で CLI に渡すリクエストファイルを 0o600 のパーミッションで作成します。
  - `ValidateExecutable`: skr バイナリの実行可否確認と絶対パス化を行います。
  - `TempDir`: プライベートな一時ディレクトリの作成と後片付け関数を返します。
  - `RandomSuffix`: リソース名の衝突回避用サフィックスを生成します。
  - `RecordResult`: runner の終了時に evidence の結果を書き込みます。後片付けの失敗も終了コード経由で結果に反映します。
  - `SignalContext`: SIGINT と SIGTERM を検知してキャンセルされる context を返します。
- 実行されるシナリオの流れは `scenario_<snake_case>.go` に分離します。シナリオファイルの先頭コメントにステップの一覧を記述し、パッと見てどういうシナリオが実行されるか分かるようにします。
- シナリオから CLI 呼び出しするための抽象インターフェース (`Call` / `CallSubcommand`) はシナリオパッケージ側で定義し、単体テストでは fake を注入します。共有 `Runner` がこのインターフェースを実装します。
- 対象は今回 `test/e2e/container-registry-api` と `test/e2e/c0nfig` に限定します。他の既存 runner は一括で移行しません。移行しない runner の挙動は変更しません。
- [ADR 0058](0058-live-e2e-command-coverage.md) の配置・命名ルールと [ADR 0059](0059-e2e-evidence-call-site-descriptions.md) の証跡記録の方針は変更しません。ステップ単位の説明は引き続き呼び出し元で渡します。
- 共有パッケージの単体テストは同じディレクトリに配置し、Runner の記録・接頭辞・環境変数・タイムアウト、リクエストファイルの作成、結果記録の失敗反映などを検証します。
- `tmp/` 配下の evidence はライブ実行時に生成される Git 無視の証跡であり、生のコマンド出力を含むため、textlint の対象外にします。

## Consequences

- scenario ファイルだけを読めば、実行されるステップと確認内容が分かります。ボイラープレートの変更は共有ライブラリに集約されます。
- 新しい runner では CLI 実行と証跡記録を再実装する必要がなくなります。既存 runner を移行する場合も、scenario ファイルの切り出しと main.go の書き換えだけで済みます。
- 共有インターフェースと runner の構造が統一されるため、runner を横断した検証や保守がしやすくなります。一方で、共有パッケージの変更は複数の runner に影響するため、単体テストの維持が必要です。
- 今回移行しない runner は `cliRunner` の手書き実装を暫定的に維持します。移行時に個別の設計根拠があれば、その挙動を維持したまま段階的に置き換えます。

## Alternatives considered

- 共有ライブラリを `test/internal/` に置く案は採用しません。共有する機構がすべて E2E runner 用であり、既存の `test/e2e/internal/evidence` と分かれるためです。`test/e2e/internal/` 配下に集めることで、E2E 以外から参照されることも防ぎます。
- runner ごとに現状の重複実装を維持する案は採用しません。CLI 実行と証跡記録の重複が増え続け、シナリオの可読性も向上しないためです。
- シナリオを JSON や YAML などの宣言的データにする案は採用しません。後片付けや冪等確認の分岐、待ち合わせなど手続き的な処理が中心であり、宣言化すると構文の設計と検証コストが増えるためです。
