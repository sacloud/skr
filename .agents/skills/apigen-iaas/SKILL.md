---
name: apigen-iaas
description: cmd/apigen-iaas を使って IaaS API コマンドの設定を更新し、生成コードを再生成・検証するときに使用します。
---

# apigen-iaas による IaaS API コマンド生成

`cmd/apigen-iaas` を使って低レベル IaaS API コマンドを追加・変更するときに、このスキルを使用してください。IaaS 以外の Ogen 系 API には共通入口 `cmd/apigen-api` を使います。対象サービスの仕様調査やコマンド設計には [`generate-skr-api-command`](../generate-skr-api-command/SKILL.md) も適用します。生成器の機能追加では、生成器自身の実装・テストと、設定・生成結果をまとめて更新します。

## 基本方針

- 生成対象の SDK 操作、入力規則、ヘルプ、ユーザー向け挙動は人が決め、設定に明記します。SDK のメソッドや型を列挙しただけでコマンドを公開しないでください。
- SDK の公開インターフェースとモデルを確認して設定し、サービス仕様は SDK ドキュメントおよび `manual.sakura.ad.jp` で裏付けます。不明な必須値やフィールドの意味を推測しないでください。SDK のリクエスト型に値が存在するだけでは API 入力と判断せず、SDK の変換処理と API 用リクエスト型を確認します。SDK が検索・変換に使う補助入力で API に送信されない値は、API フィールドとして生成フラグにしません。
- 生成ファイルは成果物です。直接編集せず、設定または生成器を修正して再生成します。
- SDK 固有の入力変換、秘密情報の扱い、通常と異なる出力・失敗処理など、生成器が責任を持たない処理は手書きコードに残します。無理に汎用化しないでください。
- E2E テストやチュートリアルで `--request` を使いたくなった場合は、先に既存オプションと限定的な CLI 拡張を検討します。必要な入力、現状の制約、拡張案と影響、JSON を残す理由を整理し、入力経路と拡張の可否をユーザーに確認してから実装・執筆してください。秘密情報や複雑な構成を無理にフラグ化せず、JSON 入力自体を検証するテストは維持します。

- 生成したコマンドを含め、公開するすべての末端サブコマンドにはライブ E2E テストを必須とします。チュートリアルの有無にかかわらず正常応答を確認し、コマンドツリーとシナリオの対応をテストします。複数のシナリオに分けても構いません。未実行や API エラーを成功扱いにせず、延期は利用者と合意して追跡 Issue を作成します。
- 後片付けも変更操作として扱い、削除・無効化・切断・設定復元の直前に現在値を読み直します。保持したリソース ID・所属と今回設定した値を照合し、`defer` やキャンセル後でも省略しません。不在なら変更せず終了し、不一致・不正応答・照会失敗なら変更を拒否します。設定の別 ID への置換やキャンセル直後の変更でも変更 API を呼ばない照合を、後片付けの実装として必須にします。詳細は [`generate-skr-api-command`](../generate-skr-api-command/SKILL.md) の後片付け要件に従います。
- 配置と命名は [`AGENTS.md` の E2E 規則](../../../AGENTS.md#e2e-の配置と命名)に従います。E2E シナリオは testing.T ベースの `scenario_<snake_case>_test.go` にし、共有の helper (`test/e2e/internal/helper`) でビルド済みの skr を実行します。`SKR_E2E` が未設定ならテストをスキップします。実行記録の識別名は `helper.New` で指定し、`config` は `c0nfig` に置き換えます。

## 許容する CLI 拡張

[ADR 0049](../../../docs/adr/0049-internet-workflow-options.md) の次の範囲は再承認なしで実装・維持し、E2E とチュートリアルで利用します。他の配列や操作へ自動的に一般化しません。

- Server Create の `--nic upstream=SWITCH_ID` は正の整数のスイッチ ID を受け付けます。既存の `shared` と `disconnected` も維持します。
- Internet・Server・Disk の Find は `--name` による単一条件の部分一致検索に対応します。生成設定は `name: "name"`、`field: "Names"`、`type: "string"`、`singleton: true` とし、値を分割せず1要素の `Names` に変換します。未指定と明示入力を区別し、複数名は JSON に残します。
- Internet Delete の `--force` と `--fail-if-not-found` は、API 本文のフィールドではなく削除の動作オプションとして公開します。`--force` は IPv6 の無効化と `NextHop` のある追加サブネットの削除を伴います。

この範囲でも JSON 入力を維持し、個別オプションとの併用・マージを拒否します。

## 作業手順

1. リポジトリルートから [`api-command-generation.md`](../../../docs/design/api-command-generation.md)、[ADR 0016](../../../docs/adr/0016-generate-low-level-api-commands.md)、`cmd/apigen-iaas`、`internal/apigen` と対象の設定 JSON、生成コードを確認します。関連するコマンド登録・手書きコード・テストも読みます。
2. 対象 SDK のバージョン、公開 API メソッド、リクエスト／レスポンス型、SDK の検証動作を確認します。各リクエスト値について、API 用リクエストへの変換を追い、実際に送信される API フィールドと SDK の補助入力を区別します。コマンド名、フラグ、必須値、例、サービス固有の制約は型から推測せず、根拠を確認して設定します。
3. 既存のリソース設定を最小限変更します。設定のルートには `package`、`resource`、コマンド／API／factory／runtime の型名、`imports`、`operations` を指定します。操作ごとに API メソッド、必要なリクエスト／レスポンス型、ヘルプ、リクエスト説明、必要なフラグを明記します。
4. フラグは API に送信されるリクエストフィールドと対応させます。生成器が扱うスカラー型は `string`、`bool`、`int`、`int64` です。SDK が API 用リクエストの構築前に検索・変換などに使う補助入力は、別途ユーザー向け操作として設計しない限り API フィールドのフラグにしません。IaaS API のトップレベル `Tags` 配列に限り、`name: "tags"`、`field: "Tags"`、`type: "[]string"`、`separator: ","` の組み合わせで、カンマ区切りの `--tags` フラグを生成できます。SDK の `Tags` 型に変換する場合は `conversion: "types.Tags"` を指定します。更新時に明示指定の有無を保持する必要がある任意値では `pointer: true` を使い、`Tags` がポインター型の場合も同様に設定します。必要な型変換は `conversion` に指定します。その他の配列、map、union、nullable 値、秘密情報、複雑な入力は JSON リクエストに残します。JSON 入力では `Tags` 配列を引き続き使えますが、`--request` と個別フラグの併用・マージは認めません。詳細は [ADR 0035](../../../docs/adr/0035-iaas-tags-flag.md) を参照してください。
5. SDK 呼び出し前に追加検証が必要な場合は `request_validator` に関数名を指定し、生成コマンドと同じ package に検証関数を実装します。生成コードはその関数を直接呼び出すため、関数名や引数型の不一致をビルド時に検出できます。`handwritten: true` の操作も、同じ package の検証関数を手書きハンドラーから直接呼び出します。Server Create のような明示的な反復入力には、[ADR 0042](../../../docs/adr/0042-generated-request-transform.md) の `repeated_flags` と `request_transform` を使えます。フラグ名、コマンドフィールド名、ヘルプを設定し、Runtime に生成する型付き callback を CLI 側から接続します。変換はフラグ経路のスカラー代入後、リクエスト検証前に実行し、JSON 経路には適用しません。入力の解析と固有制約はドメイン側で実装・テストし、秘密情報や任意の配列を自動的にフラグ化しないでください。IaaS の検索操作では単一の対象ゾーンを必須とし、`Zone` に `all` が渡された場合はフラグ経路と JSON 経路の両方で API 呼び出し前に拒否します。特殊な操作は `handwritten: true` とし、そのハンドラーは手書きで実装します。
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
8. IaaS リソースコマンドの追加・変更では、原則として `test/e2e/<resource>/` に再実行可能なライブ E2E シナリオテストを追加します。対象サービスが利用できる場合は `is1b` を基本の実行先にします。`SKR_E2E` を必須にし（未設定ならスキップ）、ランダムな識別名、既存リソースへの変更拒否、作成したリソースだけを確認して削除する後始末、削除後の不在確認を実装します。実行記録は helper が Git の追跡対象外の `e2e-output/` に保存します。環境・認証・明示的な承認がそろう場合は、ライブ E2E も実行します。実行できない場合もシナリオテストは追加し、未実施の理由を伝えます。
9. 新しいユーザー向け IaaS コマンドには `docs/manual/tutorials/iaas-api/` に利用者向けチュートリアルを作成し、`docs/manual/README.md` から参照できるようにします。[`generate-skr-api-manual`](../generate-skr-api-manual/SKILL.md) を適用し、認証、前提条件、実行順、ID の受け渡し、入力ファイル、出力、注意事項、クリーンアップを説明します。ライブ検証の実施日、個別実行の結果、検証で作ったリソースの一時的な状態は書かず、再現可能な操作手順と確認済みの恒常的な仕様を記載します。
10. 変更した Go ファイルを `gofmt` し、再生成後に生成コードが設定と一致すること、2 回の生成結果が安定していること、対象パッケージ・CLI・ライブ E2E の各テスト、`git diff --check` が成功することを確認します。Markdown を追加・変更した場合は `make lint-text` も実行します。

## 設定と生成物の境界

[ADR 0047](../../../docs/adr/0047-minimize-cli-request-body-validation.md) に従い、本文の必須項目、値の範囲、サービス上の相関制約を CLI で重複検証しません。SDK と API に委ね、エラーを利用者へ返します。CLI の必須オプション、相互排他、ファイル・JSON・独自構文、秘密情報の入力制約は検証します。例外は `validate_zone` と、SDK が無視する Archive の未知 OSType です。EventBus の Provider 補完など、検証ではない入力処理は残します。

- 設定 JSON はコマンドの公開範囲とユーザー向け説明の根拠です。SDK の型だけでは決まらないフラグ、必須条件、ヘルプ、検証関数を明示します。
- 単一ゾーンを対象とする生成操作には `validate_zone: true` を指定します。共通検証はリクエスト構築後、`request_validator` の前に空文字列と `all` を拒否します。リソース固有の validator でゾーン検証を重複させず、手書きハンドラーでは `zones.ValidateSingleZone` を直接呼び出します。詳細は [ADR 0046](../../../docs/adr/0046-common-iaas-zone-validation.md) を参照してください。
- `request_type` がある操作は `--request` と個別フラグを併用させず、JSON とフラグの経路を混ぜません。個別フラグの必須性はフラグ経路だけに適用します。
- `response_type` がない操作は成功時に出力しません。通常と異なる SDK 操作やユーザー向け挙動を隠さないよう、設定と生成コードを対応させます。
- `Runtime` callback と SDK API factory は CLI 側で設定します。生成されたコマンド群には `New...Commands(runtime, factory)` constructor からまとめて渡し、setter による段階的な初期化は使いません。リクエスト検証関数は生成 package に置き、`request_validator` で生成コードから直接呼び出します。文字列キーによる振り分けや CLI の Runtime 検証 callback 接続は使いません。生成 package と `package main` の非公開処理を直接結び付けず、既存の callback パターンに従います。
- 生成器が新しい設定項目を扱う変更では、設定の型、検証、テンプレート、決定性・エラー経路のテスト、設計文書を一緒に見直します。既存の生成物だけを更新して生成器のテストを省略しないでください。
