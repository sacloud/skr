---
name: apigen-iaas
description: cmd/apigen-iaas を使って IaaS API コマンドの設定を更新し、生成コードを再生成・検証するときに使用します。
---

# apigen-iaas による IaaS API コマンド生成

`cmd/apigen-iaas` を使って低レベル IaaS API コマンドを追加・変更するときに、このスキルを使用してください。IaaS 以外の Ogen 系 API には共通入口 `cmd/apigen-api` を使います。対象サービスの仕様調査やコマンド設計には [`generate-skr-api-command`](../generate-skr-api-command/SKILL.md) も適用します。生成器の機能追加では、生成器自身の実装・テストと、設定・生成結果をまとめて更新します。

## 基本方針

- 生成対象の SDK 操作、入力規則、ヘルプ、ユーザー向け挙動は人が決め、設定に明記します。SDK のメソッドや型を列挙しただけでコマンドを公開しないでください。
- SDK の公開インターフェースとモデルを確認して設定し、サービス仕様は SDK ドキュメントおよび `manual.sakura.ad.jp` で裏付けます。不明な必須値やフィールドの意味を推測しないでください。
- 生成ファイルは成果物です。直接編集せず、設定または生成器を修正して再生成します。
- SDK 固有の入力変換、秘密情報の扱い、通常と異なる出力・失敗処理など、生成器が責任を持たない処理は手書きコードに残します。無理に汎用化しないでください。

## 作業手順

1. リポジトリルートから [`api-command-generation.md`](../../../docs/design/api-command-generation.md)、[ADR 0016](../../../docs/adr/0016-generate-low-level-api-commands.md)、`cmd/apigen-iaas`、`internal/apigen` と対象の設定 JSON、生成コードを確認します。関連するコマンド登録・手書きコード・テストも読みます。
2. 対象 SDK のバージョン、公開 API メソッド、リクエスト／レスポンス型、SDK の検証動作を確認します。コマンド名、フラグ、必須値、例、サービス固有の制約は型から推測せず、根拠を確認して設定します。
3. 既存のリソース設定を最小限変更します。設定のルートには `package`、`resource`、コマンド／API／factory／runtime の型名、`imports`、`operations` を指定します。操作ごとに API メソッド、必要なリクエスト／レスポンス型、ヘルプ、リクエスト説明、必要なフラグを明記します。
4. フラグは SDK リクエストのフィールドと対応させます。生成器が扱う型は `string`、`bool`、`int`、`int64` です。明示指定の有無を保持する必要がある任意値では `pointer: true` を使い、必要な型変換は `conversion` に指定します。配列、map、union、nullable 値、秘密情報、複雑な入力は JSON リクエストに残します。
5. SDK 呼び出し前に追加検証が必要な場合は `request_validator` を指定し、検証関数を手書きコードに実装します。IaaS の全ゾーン検索は `zone_search` を使い、検索条件の各ゾーンへの適用、結果集約、失敗時の部分出力抑止を確認します。特殊な操作は `handwritten: true` とし、そのハンドラーは手書きで実装します。
6. リポジトリの設定と出力先を明示して生成します。Switch の設定と出力先は次のとおりです。

   ```console
   make generate-iaas-api API_CONFIG=api/commands/iaas-switch.json API_OUTPUT=internal/iaas/switchapi/switch_api_generated.go
   ```

   `API_CONFIG` と `API_OUTPUT` はどちらも必須です。新しいリソースでは既存の設定ファイル配置・命名規則に合わせ、出力先ディレクトリが存在することを確認します。生成後に、設定ファイルと生成ファイルの差分だけでなく、CLI 登録と手書き runtime／factory の接続も確認します。
7. 生成器のロジックやテンプレートを変更した場合は、`internal/apigen` の対象テストを追加・更新し、少なくとも次を実行します。

   ```console
   go test ./internal/apigen ./cmd/apigen-iaas
   ```

   さらに生成対象パッケージと CLI の対象テストを実行します。ルートコマンド登録を変更した場合は、適用可能な範囲で CLI 全体も検証します。対象外・アクセス制限のあるファイルを回避するため全パッケージを探索せず、許可された対象パッケージを明示します。
8. IaaS リソースコマンドの追加・変更では、原則として `test/e2e/<resource>/` に再実行可能なライブ E2E runner とシナリオテストを追加します。対象サービスが利用できる場合は `is1b` を基本の実行先にします。確認用フラグを必須にし、ランダムな識別名、既存リソースへの変更拒否、作成したリソースだけを確認して削除する後始末、削除後の不在確認を実装します。実行記録には共通の private evidence 機構を使い、Git の追跡対象外に保存します。環境・認証・明示的な承認がそろう場合は、ビルドした CLI でライブ E2E も実行します。実行できない場合も runner とそのテストは追加し、未実施の理由を伝えます。
9. 新しいユーザー向け IaaS コマンドには `docs/manual/tutorials/iaas-api/` に利用者向けチュートリアルを作成し、`docs/manual/README.md` から参照できるようにします。[`generate-skr-api-manual`](../generate-skr-api-manual/SKILL.md) を適用し、認証、前提条件、実行順、ID の受け渡し、入力ファイル、出力、注意事項、クリーンアップを説明します。ライブ検証の実施日、個別実行の結果、検証で作ったリソースの一時的な状態は書かず、再現可能な操作手順と確認済みの恒常的な仕様を記載します。
10. 変更した Go ファイルを `gofmt` し、再生成後に生成コードが設定と一致すること、2 回の生成結果が安定していること、対象パッケージ・CLI・ライブ E2E の各テスト、`git diff --check` が成功することを確認します。Markdown を追加・変更した場合は `make lint-text` も実行します。

## 設定と生成物の境界

- 設定 JSON はコマンドの公開範囲とユーザー向け説明の根拠です。SDK の型だけでは決まらないフラグ、必須条件、ヘルプ、検証関数を明示します。
- `request_type` がある操作は `--request` と個別フラグを併用させず、JSON とフラグの経路を混ぜません。個別フラグの必須性はフラグ経路だけに適用します。
- `response_type` がない操作は成功時に出力しません。通常と異なる SDK 操作やユーザー向け挙動を隠さないよう、設定と生成コードを対応させます。
- `Runtime` callback、SDK API factory、IaaS Zone API factory は CLI 側で設定します。生成 package と `package main` の非公開処理を直接結び付けず、既存の callback パターンに従います。
- 生成器が新しい設定項目を扱う変更では、設定の型、検証、テンプレート、決定性・エラー経路のテスト、設計文書を一緒に見直します。既存の生成物だけを更新して生成器のテストを省略しないでください。
