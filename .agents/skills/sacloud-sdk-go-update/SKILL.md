---
name: sacloud-sdk-go-update
description: sacloud-sdk-go を新しいバージョンへ更新するとき、API 定義 (ogen 定義) の変更を確認して skr 側の追従作業を判断・実施するために使用します。
---

# sacloud-sdk-go の更新

`github.com/sacloud/sacloud-sdk-go` を新しいバージョンへ更新するときに、このスキルを使用してください。dependabot の対象からは外れています ([ADR 0097](../../../docs/adr/0097-exclude-sacloud-sdk-go-from-dependabot.md))。依存の自動更新は、API 定義の変更を skr 側へ追従させる判断を経ないためです。更新は必ずこの手順で、人が変更内容を検討しながら行います。

## SDK の層と skr への影響

`sacloud-sdk-go` は次の層から構成されます。変更の影響は層ごとに異なるため、まずどこが変わったのかを区別します。

- **ogen 定義と生成コード**: `api/<domain>/openapi/openapi.yaml` と ogen が生成する Go コード。API 定義の変更がここに現れます。skr の生成コマンドと手書きコードが直接使う型に影響します。
- **手書きラッパー**: `api/<domain>/*.go` の `DestinationOp`、`NFSOp` など。生成コードを包む SDK 側の利便層です。型定義を変えずに nil 補完などの挙動を追加することがあります (例: v0.7.1 の simple-notification `Tags`)。
- **SDK 内部ツール**: `tools/*` など。skr からは依存しないため影響しません。

## 作業手順

### 1. 変更内容の把握

1. リリースノートと CHANGELOG で変更の列挙を確認します。
2. 列挙された変更 PR の diff を実際に読みます。リリースノートの記述だけでは、どの層の変更か、skr への影響があるかを判断できません。
3. モジュールキャッシュで新旧バージョンの ogen 定義の差分を確認します。更新対象のドメインすべてを確認するか、diff ツールでディレクトリごとに比較します。

4. ogen 定義の差分では、スキーマの追加・削除、型の変化、required の変化、nullable の変化を区別して読みます。これが、生成コードの再生成とユーザー向け出力の判断の根拠になります。

新旧バージョンの openapi.yaml の比較は、たとえば次のとおりです。`go mod download` は go.mod を変更せずにモジュールキャッシュへ取得するため、旧・新の両バージョンを明示的に取得してから比較します。クリーンなモジュールキャッシュでは旧バージョンが存在しないため、新バージョンだけ取得して比較すると diff が失敗します。

```console
go mod download github.com/sacloud/sacloud-sdk-go@vX.Y.Z
go mod download github.com/sacloud/sacloud-sdk-go@vX.Y.Z+1
diff "$(go env GOMODCACHE)/github.com/sacloud/sacloud-sdk-go@vX.Y.Z/api/<domain>/openapi/openapi.yaml" "$(go env GOMODCACHE)/github.com/sacloud/sacloud-sdk-go@vX.Y.Z+1/api/<domain>/openapi/openapi.yaml"
```

### 2. skr 側の影響の分類

**ogen 定義が無変更の場合** (手書きラッパーや内部ツールのみの変更):

1. 生成コード、設定、ドキュメントの変更は原則不要です。
2. skr が使う操作を確認します。変更された手書きラッパーを経由している場合は、挙動の変化 (nil 補完、デフォルト値) と skr の補完処理が二重になっていないかを確認します。二重でも結果が同じなら変更は不要です。

**ogen 定義に変更がある場合**:

1. 対象ドメインを skr が使っているかを確認します。`internal/<domain>api/`、`internal/iaas/<resource>api/`、`service/`、`internal/sakumock*`、`test/e2e/` で SDK の型を検索します。
2. 型定義の変化 (フィールド名、nil 許容、列挙型、型の置き換え) がある場合、手書きコードで SDK の型を直接参照する箇所を追従させます。
3. 生成コードの再生成の要否を判断します。必要なら `make generate-iaas-api`、`make generate-api`、`make generate-wiring` を実行します。生成コードは直接編集しません。
4. ユーザー向け挙動の変化を確認します。JSON 出力のフィールド名や型が変わる場合は、ヘルプ文言、チュートリアル、baseline テスト (`internal/cli` の `TestRequestCommandCoverageBaseline`、`test/tutorials/baseline.txt`) の見直しが必要です。
5. 追従の設計判断 (未指定値の扱い、出力の互換性、スキーマの省略値) が必要な場合は、ユーザーと確認してから進め、判断を ADR または設計文書に記録します。

### 3. 更新と検証

1. `go get github.com/sacloud/sacloud-sdk-go@vX.Y.Z && go mod tidy` を実行します。
2. `service/` 配下の独立モジュール (`service/apprun-dedicated-service-go`、`service/eventbus-service-go`) も SDK を直接参照します。[ADR 0097](../../../docs/adr/0097-exclude-sacloud-sdk-go-from-dependabot.md) と同じ理由でこれらも dependabot の対象からは外れていないため、ルートの更新だけでは追従しません。`includes/go/common.mk` の `SUBMODULES` に列挙された各モジュールでも、SDK の更新と `go mod tidy` を行います。
3. 手順 2 で必要と判断した場合、生成コードを再生成します。
4. `go build ./...` でビルドを確認します。
5. `go test ./...` を実行します。変更のあったドメインは重点的に、必要なら `-race` も実行します。サブモジュールを含めた全体の検証は `make test` で行えます。
6. `make lint-go` を実行します。`make test` と `make lint-go` は `SUBMODULES` の各モジュールにも実行されるため、ルートモジュール単独の実行より広い範囲を検証できます。Markdown を変更した場合は `make lint-text` も実行します。
7. ユーザー向け挙動が変わる場合は、実際のコマンド出力とヘルプ・チュートリアルを照合し、該当コマンドのライブ E2E の要否をユーザーと確認します。実行する場合は [`generate-skr-api-command`](../generate-skr-api-command/SKILL.md) と [`apigen-iaas`](../apigen-iaas/SKILL.md) の E2E 要件に従います。
8. 生成コードを再生成した場合は、baseline テストの更新漏れを確認します。

### 4. 記録と PR

1. PR の説明には、少なくとも次を明記します。レビューで影響判断を追跡できるようにするためです。
   - 更新元と更新先のバージョン。
   - ogen 定義の変更有無 (無変更ならその確認結果)。
   - skr 側の影響と、追従・不要の判断根拠。
   - 検証結果 (テスト、lint、ライブ E2E の実施有無)。
2. ogen 定義の変更に伴うユーザー向け挙動の変更は、ADR または設計文書に記録します。ogen 定義が無変更で挙動変更がない場合、ADR は不要です。
3. `PLAN.md` に実装状況を記録している範囲に影響する場合は、`PLAN.md` も更新します。

## 更新の確認タイミング

dependabot が提案しなくなるため、SDK の更新は SDK 側の開発に合わせて進めます。`sacloud-sdk-go` は自社がメンテナンスする依存です。SDK 側に機能追加や API 定義の変更を加えた場合、skr 側の追従をこの手順で行います。更新しない判断 (現行バージョンで十分、追従コストが高い) も、PR や Issue に理由を残します。
