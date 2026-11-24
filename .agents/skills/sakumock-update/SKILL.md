---
name: sakumock-update
description: dependabot が提案する sakumock (github.com/sacloud/sakumock) のバージョン更新を評価するときに使用します。新旧バージョンの差分確認、既知の差分が解消されたかの精査、ワークアラウンドの撤去、記録の更新までを行います。
---

# sakumock の更新評価

`github.com/sacloud/sakumock` は dependabot が自動でバージョン更新の PR を提案します。dependabot の PR では、依存を上げたうえでビルドとテストが通ることだけでなく、既知の差分の解消を必ず評価してください。sakumock は開発途上です。skr が早期から使う機能ほど、実 API との差分は残りやすい傾向があります ([ADR 0091](../../../docs/adr/0091-split-sakumock-and-sakumockiaas-gap-tracking.md))。

差分の記録先は `docs/design/sakumock-gaps.md` です。リポジトリ内実装の IaaS モック (sakumockiaas) は dependabot の対象外で、`docs/design/sakumockiaas-gaps.md` で管理するため、この手順の対象ではありません。

## 評価手順

1. **差分の把握**: 更新前後のバージョンを `go mod download` で明示取得し、リリースノートとソースの diff を確認します。クリーンなモジュールキャッシュでは旧バージョンが存在しないため、旧・新の両方を明示的に取得してから比較します。
2. **解消の特定**: `docs/design/sakumock-gaps.md` に記録した差分のうち、upstream issue の番号で更新に含まれる解消を特定します。
3. **動作確認**: 解消が含まれる場合は、CLI 操作テストなど sakumock を起動するテストで実際の動作を確認します。
4. **ワークアラウンドの撤去**: エラーの許容、自前テストサーバーへの置き換え、テスト入力の制限など、差分への対応として入れた扱いを正常系の検証に戻します。単体テスト、チュートリアル harness、ライブ E2E の該当箇所を確認します。
5. **記録の更新**: `docs/design/sakumock-gaps.md` の該当箇所に解消日と確認内容を残し、節見出しのバージョンを更新します。`test/tutorials/baseline.txt` の status や理由が変わる場合は、対応するチュートリアルの本文と対応表も更新します。
6. **検証**: 変更したパッケージのテストを実行し、必要に応じて `go test ./...` を実行します。Markdown を変更した場合は `make lint-text` を実行します。

## 解消する差分が含まれない更新

ビルドとテストの確認、更新内容の記録だけを行い、`docs/design/sakumock-gaps.md` の節見出しのバージョンのみ更新します。

## 新たな差分が見つかった場合

「差分の扱いの分類」(許容、除外、自前モック、未整備) を判断して `docs/design/sakumock-gaps.md` に追記します。どのテストで許容・除外したか (ファイル名と関数名) まで書き、upstream (sacloud/sakumock) への issue 起票の要否を判断します。
