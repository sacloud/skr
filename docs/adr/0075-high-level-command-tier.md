# ADR 0075: 高レベルコマンドを skr <domain> 名前空間に導入する

- Status: Accepted
- Date: 2026-11-10

## 背景

[ADR 0005](0005-low-level-and-high-level-command-tiers.md) は、SDK 操作を 1 対 1 で公開する低レベル `<domain>-api` と、skr 内の service layer を使う高レベル `<domain>` の階層を提案しましたが、高レベルの実装はまだ存在しません。[ADR 0074](0074-api-command-name-structure.md) は、CommonServiceItem 操作を `--provider-class` で 1 組のコマンドに統合する一方、「リソースごとに使いやすい操作の形が必要な場合は、高レベルコマンドで提供します」としています。

EventBus では [#214](https://github.com/sacloud/skr/issues/214) のとおり、`eventbus-api` の入出力に CommonServiceItem が直接現れ、利用者は `Settings` の JSON を種別 (実行設定・スケジュール・トリガー) ごとに組み立てる必要があります。採用中の SDK (`sacloud-sdk-go` v0.5.0) は [`api/eventbus`](https://github.com/sacloud/sacloud-sdk-go/tree/v0.5.0/api/eventbus) に種別ごとの操作とヘルパーを提供しますが、EventBus の service 層はありません。SDK 側の追加を待たず、必要な作業単位の処理を skr 内に設計します。

高レベルの実装は初めてであるため、この機会に階層全体の規則を決定します。

## 決定

- 高レベルコマンドの名前空間は `skr <domain>` とします。低レベル `<domain>-api` と対になります。EventBus は `skr eventbus` です。[ADR 0005](0005-low-level-and-high-level-command-tiers.md) に例示した高レベル名 `event` は、本 ADR で `eventbus` に改定します。
- 高レベルドメインのパッケージは `internal/<domain>` とし、service 層と CLI コマンドを同じパッケージに置きます。EventBus では SDK の service 層の構成 ([service/iaas/swytch](https://github.com/sacloud/sacloud-sdk-go/tree/v0.5.0/service/iaas/swytch) など) を参考に、`Service` 構造体、操作ごとのリクエスト型、SDK 応答モデルの戻り値を設けます。service 層のファイルは `sacloud-sdk-go` と標準ライブラリのみに依存し、kong、CLI の Runtime、出力処理には依存しません。将来 SDK に `service/eventbus` が追加された場合は、公開 API と挙動を照合して採用を判断します。ファイル移動だけで移植できることや、SDK 側での採用は前提にしません。切り替え時の確認事項は [EventBus 高レベルコマンドの設計](../design/eventbus.md) に記録します。
- 高レベルコマンドに `--request` は設けません。生 JSON、`null` 指定、未対応フィールドなどの完全な入力経路は低レベル `<domain>-api` が担います。高レベル層は公式マニュアルと SDK の仕様で確認できた入力だけをフラグ化します。
- service 層は SDK の応答モデルをそのまま返します。利用者向け出力への射影は CLI の出力処理が担います。
- service 層は入力形式の変換と作業単位の整合性を扱い、サービス仕様の検証を網羅的に再実装しません。[ADR 0047](0047-minimize-cli-request-body-validation.md) に従い、追加する検証は入力変換や安全な更新に必要な理由をドメインの設計文書へ記録します。
- 秘密情報は引数・ヘルプ・出力・ログに含めず、保護したファイルまたは標準入力から読み込みます。
- 高レベルコマンドは [API コマンド生成器](../design/api-command-generation.md) の対象外で、手書きします。

## 影響

- 高レベルドメインごとに service 層の設計と手書き実装が必要になります。生成器による自動化は得られません。
- service 層の CLI 非依存は、単独での検証と将来の SDK 採用を妨げないための規律として維持します。同じパッケージへの配置だけでは依存が分離されないため、service 層から CLI 側の型やヘルパーを参照しないこともレビューします。service 層の単体テストは CLI 入出力に依存しません。
- 低レベル層と高レベル層は、コマンド、ヘルプ、E2E、チュートリアルをそれぞれ持ちます。両層の対応はドメインの設計文書で記録します。

## 検討した選択肢

- `skr event` 名前空間 (ADR 0005 の例): 短い一方、ドメイン名 (`eventbus`) との対応が崩れ、他ドメインへ展開できる規則にならないため採用しません。
- 高レベルコマンドにも `--request` を併設する方式: JSON 経路が二重になり、階層の責務が曖昧になるため採用しません。
- service 層を作らず CLI から SDK の api 層を直接呼ぶ方式: 更新時の読み取りとマージのような作業単位のロジックが CLI に漏れ、SDK 移植の単位がなくなります。採用しません。
- service 層を独立パッケージ (`internal/eventbus/service` など) に分離する方式: 参照時の名前が曖昧になり、この規模では分割の利益が小さいため採用しません。service 層のファイルを CLI コマンドのファイルから分離する規律で移植性を確保します。
