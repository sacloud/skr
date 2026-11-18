# チュートリアルスクリプト実行 harness の設計

- 状態: Phase 1・2 実装済み (Phase 3 以降は未着手)
- 根拠: [ADR 0088](../adr/0088-tutorial-script-execution-harness.md)
- 関連: [ADR 0065](../adr/0065-e2e-command-coverage-baseline.md) (baseline 形式)、[ADR 0062](../adr/0062-e2e-testing-t-based-scenario.md) (E2E の配置)

## 目的

`docs/manual/tutorials/` の `console` ブロックを sakumock に対して bash で実行し、CLI の変更 (フラグ改名、出力形式の変更、フィールド参照の変更) がチュートリアルを壊したときに `make test` で検出する。

## 構成

```
test/tutorials/
├── harness_test.go    # TestMain: バイナリビルド、全チュートリアルの実行
├── extract.go         # console ブロック → コマンド (step) と期待出力の抽出
├── extract_test.go
├── runner.go          # step の bash 実行、状態引き継ぎ、stub 検証、モック起動とイベント注入
├── runner_test.go
├── fixture.go         # fixture の読み込み、値置換、skip/stub 判定、未使用検出
├── fixture_test.go
├── support.go         # kong パース (internal/cli.ParseOnly)、gojq コンパイル
├── compare.go         # 期待出力と実際の出力の構造比較
├── compare_test.go
├── baseline.go        # baseline.txt の読み込みと照合
├── baseline_test.go
├── baseline.txt       # チュートリアルごとの状況とブロック数の下限
└── fixtures/
    ├── zone.json      # implemented チュートリアルの制御 fixture
    └── eventbus.json  # Phase 2: SimpleMQ / EventBus モックとイベント注入の fixture
```

IaaS モックの HTTP ブリッジは `internal/sakumock/iaas/http.go` に置く。通常ビルドでは参照されないが、単体テスト (`http_test.go`) でブリッジと `remoteCaller` の往復を検証する。

## 抽出 (extract)

` ```console ` コードブロックを抽出し、次の規則で step に分割する。

- `$ ` で始まる行は新しいコマンド行 (step の開始)。
- 直前のコマンドが行継続 (`\`) で終わる場合、次の行は先頭のインデントを除去して同じ step に連結する。
- heredoc (`<<EOF` ... `EOF`) の本体と終端行はコマンドの一部として含める (終端行が欠けると bash の `source` が構文エラーになる)。`<<-` の場合は先頭タブを除去して終端を判定する。
- それ以外の `$` で始まらない行は、直前の step の期待出力 (インライン期待出力) とする。コマンドの前に期待出力がある場合は抽出エラーとする。
- ` ```console ` の直後 (空行を挟んでもよい) に置かれた ` ```json ` コードブロックは、直前の `console` ブロックの最後の step に対する期待出力とする。本文を挟んで離れた位置にある JSON 例は関連付けない (誤関連付けを避ける fail-closed な規則)。
- `#` で始まるコメント行は検証対象から除外する。

抽出結果は `(ファイルパス, 開始行番号, コマンドテキスト, 期待出力)` のリスト。報告時にチュートリアルの行番号へ対応付ける。

## 実行 (runner)

### step 実行と状態引き継ぎ

各 step は次の wrapper で `bash -euo pipefail` により実行する。

```bash
set -o errexit -o nounset -o pipefail
set -a
cd "$SKR_HARNESS_CWD"
umask "$SKR_HARNESS_UMASK"
source "$SKR_HARNESS_STEP"   # step のコマンドテキスト
printf '%s' "$(pwd)" > "$SKR_HARNESS_STATE/cwd"
umask > "$SKR_HARNESS_STATE/umask"
env -0 > "$SKR_HARNESS_STATE/env"
```

- wrapper の stdout は step の出力として捕獲し、期待出力の照合に使う。stderr はエラー報告の証跡として保持する。
- `set -a` により、step 内の変数代入も環境変数として `env -0` のダンプに現れる。終了後、`cwd`、`umask`、`env` を読み戻し、次の step の環境に反映する (`SHLVL` と `_` を除く)。2 step 目以降は直前 step の完全な環境を基準に再構築するため、`unset` の反映も引き継がれる。これにより `WORK_DIR=$(mktemp -d)` のようなブロックをまたぐ変数も動作する。
- タイムアウト (既定 60 秒) を超えたら step を失敗とする。
- `$()` のコマンド置換は bash がそのまま評価するため、ID 取得の `$(skr ... --query ...)` も追加機構なしに検証される。

### fixture の適用順序

1. stub の判定 (`stub`) — チュートリアルに書かれたコマンドテキスト (正規化後) が fixture の stub キーと一致した場合、その step は実行せず置換後のテキストに対する構文検証だけを行う。
2. skip の判定 (`skip`) — harness 固定の対話コマンド (`ssh`、`exit` など)、および fixture で指定したコマンドの前綴りに一致した step は実行しない。
3. 値の置換 (`values`) — 実行するコマンドと stub の置換後テキストに対し、`ZONE`、`QUEUE-NAME` などのトークンを境界で置換する。

正規化は空白のトリムと連続空白の圧縮のみ。fixture の項目 (stub キー、skip、values のトークン、mock.events の after) がどの step にも適用されなかった場合は検証を失敗させる (fail-closed)。チュートリアルの編集で対応が外れた項目を検出するためである。

### stub の構文検証

stub で置き換えられた元のコマンドは実行されないため、置換後のテキストに対して構文を検証し、フラグ改名や式の壊れを検出する。トークン化はシェル風の分割 (シングル/ダブルクォートを解釈) で行う。`skr` コマンドは `internal/cli` の `ParseOnly` (kong パースのみ、実行なし) で検証し、`--query` / `--query-raw` 式は gojq コンパイルで検証する。stub 対象は最小限に保つ。

### 値の置換 (values)

- `values` のキーは英数字とハイフンからなる大文字トークンにする。置換は大文字の caps 語 (例: `ZONE`、`UNIQUE-SUFFIX`) の境界で判定し、ハイフンは境界として扱うため `eventbus-switch-UNIQUE-SUFFIX` のような名前内部も置換される。一方、アンダースコアで連結された語 (`ZONE_ID` の `ZONE`) は別の語として置換しない。
- 値トークンが、より長い未定義のプレースホルダーの一部になる場合 (`SWITCH-NAME` の値だけを定義して `TEST-SWITCH-NAME` が出現) は、意図しない名前でリソースを作る恐れがあるためエラーにする (fail-closed)。
- 値に空白を含めたい場合は stub で表現する。

## 期待出力の比較 (compare)

- 期待出力 (インライン行、` ```json ` ブロック) を連結して JSON としてパースでき、実際の出力も JSON としてパースできる場合のみ構造比較する。インライン行の照合は、その step を実行した直後の出力に対して行う。
- 構造比較の規則:
  - object: キーの集合が一致すること。値は再帰的に比較する。
  - array: 長さが一致すること。要素は再帰的に比較する。
  - string / number / bool: 型のみを比較する (値の不一致は許容する)。出力例の値はマスクされているため。
  - null: 期待が null の場合は実際も null を要求する。
- 空配列 `[]` は長さ 0 の配列として厳密に一致を要求する (list 系コマンドの「一覧に残っていないこと」の確認)。
- 期待出力を JSON としてパースできない場合 (プレーンテキストの出力例) は、コマンドの終了コードのみを検証する。
- 実際の出力が JSON としてパースできず、期待出力が JSON の場合は不一致とする。

## mock との接続

### IaaS (`iaas-api`)

`skr_tutorial_harness` ビルドタグ付きでバイナリをビルドする。

- `internal/iaas/client/client.go` に `//go:build !skr_tutorial_harness` を追加する (通常ビルド)。
- `internal/iaas/client/harness_client.go` (`//go:build skr_tutorial_harness`) が、同じシグネチャの `New(trace bool)` を提供する。環境変数 `SKR_TUTORIAL_HARNESS_IAAS_URL` があれば `internal/sakumock/iaas` の HTTP ブリッジに接続する `remoteCaller` を返し、なければ in-process の `sakumockiaas.Server` を返す。どちらも `iaas.APICaller` と適合する。
- モックは runner 側で 1 つだけ起動し、`NewTestHTTPServer` で HTTP に公開する。`skr` は step ごとにプロセスを起動するため、スイッチなどの状態をプロセス間で共有するには HTTP 経由にする必要がある。`SKR_TUTORIAL_HARNESS_IAAS_URL` にブリッジの URL を設定してサブプロセスに伝える。
- モックのシード (ゾーン一覧など) は環境変数 `SKR_TUTORIAL_HARNESS_ZONES` で渡す。fixture の `mock.zones` から runner が設定する。
- HTTP ブリッジはリクエストをそのまま `Do(ctx, method, uri, body)` に変換し、エラーは 400 とメッセージ本文で返す。`remoteCaller` は 2xx 以外を `Do` のエラーに変換するため、in-process の場合と同じエラー文言が CLI に伝わる。
- `internal/cli` の配線は `iaasclient.New` の結果を `<res>api.NewAPI(client)` に渡すだけで、戻り値の具象型に依存しないため、タグ付きビルドでも無変更でコンパイルできる。

### Ogen 系 API

sakumock モジュールのテストサーバーをハーネス内で起動し、`SAKURA_ENDPOINTS_<SERVICE>` 環境変数で接続する (`internal/cli/simplemq_test.go` と同じ方式)。IaaS と異なり通常ビルドのバイナリをそのまま使える。

fixture の `mock.services` で起動するモックサービスを宣言する。`simplemq` を宣言すると `SAKURA_ENDPOINTS_SIMPLE_MQ_QUEUE` と `SAKURA_ENDPOINTS_SIMPLE_MQ_MESSAGE` に同じテスト URL を設定する。`eventbus` は SimpleMQ 宛てのサービスリンクを持つため、SimpleMQ モックの URL とダミー認証情報を渡して `NewTestServerWithServiceLink` で起動し、`SAKURA_ENDPOINTS_EVENTBUS` を設定する。実 API にサービスリンクが向かないよう、`eventbus` だけを宣言した fixture は検証で拒否する。

モックサービスを宣言しても実 API に到達する経路が残る場合は、認証方式に注意する。サービスプリンシパル認証のプロファイルは実環境の認証エンドポイントで JWT を検証するため、harness のプロファイルはダミーのアクセストークン認証 (Basic 認証) で作成する。モックは任意の Basic / Bearer 認証を受理するため、この方式ですべての通信をローカルに留められる。

### イベント注入

実世界ではリソース操作の副作用としてイベントが発生し、EventBus のトリガーが発火する。これを再現するため、fixture の `mock.events` で「どの step の後にどのイベントを注入するか」を宣言する。

- `after` は step コマンドの正規化後テキストと完全一致させる。一致するたびに、runner は EventBus モックの検証用エンドポイント (`POST /_sakumock/events`) へイベントを POST する。
- `source` と `type` はトリガーの照合に、`attributes` は条件の照合に使われる。属性値にも fixture 値の置換が適用される。
- 応答の `Deliveries[].Error` が空でない場合、および配信が 0 件の場合 (トリガーが 1 つも一致しない場合) は、メッセージが SimpleMQ に届いていないことを意味するため、fail-closed でチュートリアルを失敗させる。
- `after` で指定した step がチュートリアルに存在しなくなった場合は未使用 fixture として検出される。


### sakumock 未整備サービス

baseline で `deferred` とし、抽出と baseline 照合のみを行い、実行しない。ライブ API には向けない。

## bootstrap

TestMain で次を行う。

1. `go build -tags skr_tutorial_harness -o <tmp>/skr ./cmd/skr`。`SKR_TUTORIALS_BIN_DIR` にバイナリのディレクトリを設定する。
2. 一時プロファイルディレクトリを作成し、`SAKURA_PROFILE_DIR` を設定。runner が SDK の Profile API でダミーのアクセストークン認証によるプロファイル `tutorials` を作成し、current に設定する。認証情報はすべてダミーで、実環境の値は使わない。サービスプリンシパル認証を使わない理由は「mock との接続」の節を参照。
3. fixture と baseline を読み込み、`docs/manual/tutorials/` の全 `.md` を走査する。

## baseline

`test/tutorials/baseline.txt` の 1 行は次の形式。

```
<tutorial-relative-path> <implemented|deferred> <blocks> [<reason>]
```

- `implemented`: 実行検証の対象。対応する fixture ファイルが必須。
- `deferred`: sakumock 未整備など。理由を第 4 フィールド以降に書く。抽出のみを行い実行しない。
- `<blocks>`: 抽出された `console` ブロック数。実測が下限を下回ったら抽出の腐敗として失敗する。
- baseline に記載のないチュートリアルファイルは失敗。

## 実行タイミング

- `make test` に含める (`go test ./...` から実行される)。`SKR_E2E` ゲートは不要。
- CI は `ubuntu-latest` で `jq` が利用可能。チュートリアル内の `jq` 呼び出しは実環境の `jq` を使う。

## Phase 計画

| Phase | 対象 | 内容 | 状況 |
| --- | --- | --- | --- |
| 1 | `iaas-api/zone.md` | harness 本体、IaaS ビルドタグ接続、zone.md の ID 取得書き換え、baseline 初版 | 完了 |
| 2 | `eventbus.md` | Ogen 系接続 (SimpleMQ / EventBus モック)、`mock.services` と `mock.events`、IaaS モックの HTTP ブリッジによる状態共有、値注入 (`UNIQUE-SUFFIX`、`QUEUE-NAME`、`ZONE`、`STARTS-AT`) | 完了 |
| 3 以降 | IaaS 系チュートリアル | 手動置換プレースホルダーの撤去 (`server.md`、`internet.md`、`gslb-api.md`、`proxylb-api.md`、`networking-suite-api.md`)、sakumockiaas のデータ拡充 (`archive`、`product disk`、`server`、`appliance` など)、baseline の `deferred` 解除 | 未着手 |

## fixture の形式

`test/tutorials/fixtures/<チュートリアルファイル名の拡張子を .json に置換した名前>.json` に置く (`iaas-api/zone.md` なら `zone.json`)。ディレクトリ違いで同名のチュートリアルが存在した場合は失敗する。

```json
{
  "skip": ["実行しない step の正規化後コマンド"],
  "stub": {"置換対象コマンド": "置換後のコマンド"},
  "values": {"PLACEHOLDER": "値"},
  "mock": {
    "zones": ["is1a"],
    "services": ["simplemq", "eventbus"],
    "events": [
      {
        "after": "イベントを注入する step の正規化後コマンド",
        "source": "//eventbus.sakura.ad.jp/eventlog",
        "type": "jp.ad.sakura.eventbus.eventlog.IaaS.request.Switch.normal.created",
        "attributes": {"zone": "ZONE"}
      }
    ]
  }
}
```

- `values` のキーは英数字とハイフンの大文字トークンにする。空白を含むキーは stub で表現する。
- 値の置換は caps 語の境界 (ハイフン、引用符、空白など) で行う。ハイフンで連結された名前の一部 (`eventbus-switch-UNIQUE-SUFFIX` など) も置換対象になる。一方、トークンがより長い未定義のプレースホルダー (`TEST-SWITCH-NAME` に対する `SWITCH-NAME`) の一部になる場合は、意図しない値でリソースを作る恐れがあるためエラーにする (fail-closed)。
- `mock.services` は起動する Ogen 系モックを宣言する。`eventbus` は `simplemq` と同時に宣言する必要がある。
- `mock.events[].attributes` の値にも `values` の置換が適用される。
- チュートリアルから参照されなくなった `values` / `skip` / `stub` / `mock.events` の項目は fail-closed で失敗する。

## 既知の限界

- 構造比較は値を検証しない。値の妥当性はライブ E2E が担保する。
- `--query` 式が「動くが意図と異なる」選択を検出できない。
- stub で置き換えたコマンドの挙動変更は検出できない。
- `date +%s` や `mktemp` など外部コマンドの非決定性は問題にならないが、モックデータとの整合が必要な値は fixture で固定する。
- SimpleMQ モックの strict モード (rotate-api-key が発行したキーでの Bearer 認証検証) は使わず、任意のキーを受理する既定モードで実行する。キーの発行から受信までの対応自体は、発行結果をファイル経由で渡す step の実行と、イベント注入時の配信確認で検証する。
