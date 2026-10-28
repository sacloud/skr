# ADR 0065: E2E コマンド網羅を baseline で管理する

- Status: Proposed
- Date: 2026-10-28

## Context

[ADR 0058](0058-live-e2e-command-coverage.md) で公開するすべての末端サブコマンドにライブ E2E が必須とされた。網羅状況は `docs/design/live-e2e-coverage.md` の対応表を手動で更新してきたが、手動の帳簿は実態からずれる。実際に iam-api の runner 対象範囲の集計が末端コマンド数と合わず、runner が実行していない `iam-api group update` が帳簿から漏れていた。コマンドの追加や改名を apigen が行ったときに、網羅漏れを機械的に検出する仕組みが必要である。

## Decision

末端コマンドごとの E2E 状況を baseline ファイルで管理し、網羅テストで強制する。

- `test/e2e/command-coverage/baseline.txt` に、末端コマンドごとに 1 行で状況を記録する。ステータスは `live`(ライブ検証済み)、`implemented`(実装済み・ライブ未実施)、`deferred`(延期、追跡 Issue の参照が必須)、`todo`(未整備) のいずれか。
- `internal/cli` の `LeafCommandPaths` が kong の CLI モデルから末端コマンド一覧を独立に列挙する。`--help` のテキスト解析は使わない。
- `TestBaselineCoversAllLeafCommands` が、末端コマンドが baseline にすべて登録されていること、baseline が実在するコマンドだけを参照していることを検証する。ライブ環境変数が不要なため CI で常時実行される。
- `TestReportEvidenceCoverage` が、実行記録 (`test/e2e/*/e2e-output/*.md`) から実行された末端コマンドを抽出し、baseline が `live` でないコマンドが実行記録に現れた場合に失敗する。実行記録は共有 helper が生成する新形式だけを対象とし、旧 runner 形式の記録は取得しない。旧 runner は [ADR 0062](0062-e2e-testing-t-based-scenario.md) への移行予定であり、移行後に実行記録が新形式へ置き換わる。
- `make e2e-coverage` で状況の報告と網羅検証を実行する。

baseline.txt の初期値は、設計文書と `PLAN.md` に記録済みのライブ検証結果と、追跡 Issue のある延期コマンドを突合して作成した。

## Consequences

- apigen が追加した末端コマンドは baseline への登録がないため網羅テストが失敗し、E2E の整備漏れを追加時に検出できる。
- baseline は手動で更新する記録である。ライブ検証や延期の判断が変わったら baseline と設計文書を同じ変更で更新する。
- コマンドパスの網羅は実行の網羅を表すが、応答内容の検証までは表さない。応答の照合は各シナリオのテストコードが担う。
- 旧 runner 形式の記録は取得しないため、移行前の runner の実行は報告に現れない。移行時に baseline の状況を見直す。

## Alternatives considered

- `--help` の出力を正規表現で解析して末端コマンドを列挙する案。kong のモデルを直接辿るほうが正確で、ヘルプの文言変更に影響されないため不採用とした。
- `GOCOVERDIR` のビルドカバレッジで実行経路を計測する案。ライブ E2E 全体の実行が必要で、コマンド網羅の管理としては過剰なため不採用とした。
- 旧 runner 形式の記録 (`tmp/` 配下の JSON) も解析して集計する案。旧形式は廃止予定であり、移行時に新形式へ置き換わるため不採用とした。
