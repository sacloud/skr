# ADR 0088: チュートリアルスクリプトの実行検証

- Status: Accepted
- Date: 2026-12-01

## Context

`docs/manual/tutorials/` 配下のチュートリアルには、利用者がそのままコピーして実行する `console` ブロックが約 380 個ある。CLI のフラグ改名、出力形式の変更、`--query` で参照するフィールド名の変更などが起きても、チュートリアルはビルドもテストも通らず、静かに壊れ続ける。

既存の検証は次のとおりで、いずれもスクリプトの中身を検証しない。

- リンクチェッカー (`.agents/skills/generate-skr-api-manual/scripts/check_tutorial_links.py`) はリンクのみ。
- `test/e2e/command-coverage/baseline.txt` とライブ E2E ([ADR 0058](0058-live-e2e-command-coverage.md)、[ADR 0062](0062-e2e-testing-t-based-scenario.md)) は、チュートリアルに書いたコマンドをその引数の形では検証しない。
- `make docs-site` は Markdown の変換のみ。

チュートリアルの多くは、[プロジェクト方針](../../AGENTS.md) に従い ID を `--query-raw` とコマンド置換で取得する実行可能な手順になっている。一方、初期に書かれた IaaS 系のチュートリアルには、作成結果の ID を読者に手動で `SERVER_ID=SERVER_ID` のように設定させる箇所が残っている。これはハーネスに値のキャプチャ機構を持たせるより、チュートリアル自体を実行可能な形に直す方が正しい。手動置換は利用者にとっての手順の曖昧さでもあり、AGENTS.md の既存ルールに反する。

## Decision

チュートリアルの `console` ブロックを抽出し、sakumock に対して bash で実行する harness を `test/tutorials/` に導入する。`make test` で常時実行し、`SKR_E2E` によるライブ実行ゲートを持たない。

### 抽出と実行

- `console` ブロック内の `$` で始まる行をコマンド行 (step の開始) とし、それ以外の行を直前の step の期待出力として抽出する。行継続 (`\`) と heredoc (`<<EOF`) はコマンドテキストに連結し、コメント行は除外する。
- コマンドは step 単位で `bash -euo pipefail` により実行する。タイムアウト付きとし、失敗時はチュートリアルのファイル名と行番号を報告する。
- `pwd`、`umask`、環境変数は step 間でハーネスが引き継ぐ。step 内の変数代入は `set -a` により環境変数としてダンプされるため、`WORK_DIR=$(mktemp -d)` のようなブロックをまたぐ変数も動作する。
- 期待出力の照合は、コマンド直後のインライン行と、直後の ` ```json ` コードブロックに対して行う。JSON はキー名・型・配列長の構造比較とし、スカラー値は型のみを比較する (チュートリアルの出力例はマスクされた値を含むため)。空配列 `[]` は厳密に一致を要求する。JSON として解釈できない行は照合対象外とする。

### チュートリアルの実行可能性

- ID は `$(skr ... --query-raw ...)` のコマンド置換で取得する。手動置換のプレースホルダー (`SERVER_ID=SERVER_ID` など) はチュートリアルから撤去する。これは AGENTS.md の既存ルールの適用であり、ハーネスのための新しい約束ではない。
- `ZONE`、`QUEUE-NAME`、`UNIQUE-SUFFIX` など読者が選ぶ値は、fixture が値を供給してトークン置換する。

### 制御の sidecar fixture

- 制御は `test/tutorials/fixtures/<チュートリアル>.json` に置き、ドキュメント内に制御用コメントを埋め込まない。
- fixture は正規化したコマンドテキストをキーにする。コマンドを編集して対応が外れたら検証を失敗させ、使われなくなった fixture の項目もエラーにする (fail-closed)。
- fixture で stub に差し替えたコマンドは実行されないため、stub 対象のコマンドに対しては kong によるフラグ解析と gojq による式のコンパイルで構文検証を行う。
- `ssh`、`exit` など対話的なコマンドは harness の固定リストで skip し、fixture で個別に追加できる。

### mock との接続

- IaaS (`iaas-api`) は `skr_tutorial_harness` ビルドタグ付きバイナリを使う。タグ付きビルドでは `internal/iaas/client` の `New` が `internal/sakumock/iaas` のサーバーを in-process で返し、モックデータは環境変数経由でシードする。通常ビルドには影響しない。
- Ogen 系 API (`eventbus`、`simplemq-api` など) は sakumock モジュールのテストサーバーをハーネス内で起動し、`SAKURA_ENDPOINTS_<SERVICE>` 環境変数で接続する。既存の sakumock テストと同じ方式である。
- sakumock が用意されていないサービスのチュートリアルは、baseline で `deferred` とし実行しない。ライブ API には決して向けない ([AGENTS.md](../../AGENTS.md) の安全性規約)。

### baseline

- `test/tutorials/baseline.txt` にチュートリアルごとの状況 (`implemented` または `deferred` と理由) と、抽出されたブロック数の下限を `<path> <status> <blocks> [<理由>]` の形式で記録する。baseline で状況を固定する考え方は [ADR 0065](0065-e2e-command-coverage-baseline.md) に倣う。
- baseline に記載のないチュートリアルファイルが存在したら検証を失敗させ、新規チュートリアルの登録漏れを検出する。
- チュートリアルの書き換えは baseline の `deferred` を解除しながら段階的に進める。

## Consequences

- CLI の変更がチュートリアルのスクリプトを壊した場合、`make test` が失敗する。フラグ改名や出力形式の変更時にチュートリアルの更新漏れが即座に検出される。
- チュートリアルを執筆・更新する際は、実行可能な形 (コマンド置換による ID 取得) を維持する必要がある。
- 構造比較は値を検証しないため、出力例の値の妥当性は保証しない。検出対象はフィールド名・型・構造の齟齬である。
- IaaS のチュートリアルを `implemented` にするには、対象リソースの sakumock データ (`archive`、`product disk` など) の拡充が必要になる。これはコマンドの E2E でも必要になる作業である。
- `--query` / `--query-raw` の式の typo は gojq の評価エラーとして検出されるが、意図と異なる選択 (別の要素を取る) は検出できない。
- stub で置き換えたコマンドの挙動変更は検出できない。stub の範囲は最小限に保ち、モックの拡充を優先する。

## Alternatives considered

- 静的な構文検証のみ (コマンドの kong パース、gojq コンパイル)。実行検証を整備するなら、フラグの有無しか確認できない静的検証の価値は小さく、保守コストに見合わない。構文検証は stub で実行されないコマンドへの適用に限定する。
- コマンド単位で skr をリプレイする方式。シェルの接着部 (変数の受け渡し、heredoc、`jq` による加工、コマンド置換) を検証できず、チュートリアルの主要な壊れ方を検出できない。
- 制御をドキュメント内の HTML コメントで表現する方式。公開する利用者向け文書に検証用の注記が混入し、fixture をキーとする fail-closed な対応確認もできない。将来どうしても必要になった場合に限り、限定的に再検討する。
- sakumock が未整備のサービスをライブ API に向けて実行する方式。クラウドリソースを作成・変更するため安全性規約に反する。`deferred` として実行しない。
- チュートリアルを検証用に書き換えず、値のキャプチャをハーネスが担う方式。`SERVER_ID=SERVER_ID` のような手動置換を永続化することになり、AGENTS.md の方針に反する。チュートリアルを実行可能な形に直す方が利用者にとっても良い。
