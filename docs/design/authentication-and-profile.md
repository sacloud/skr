# 認証とプロファイルの設計

- Status: Proposed

## 目的

プロファイルファイルを直接編集せず、profile 作成・編集コマンドの対話フローから認証情報を設定します。認証情報は profile の一部として扱い、保存形式を意識する必要がない形にします。

認証設定フローの方針は [ADR 0028](../adr/0028-use-service-principal-key-for-authentication-setup.md) に従います。サービスプリンシパルキーを標準の認証方式とし、CLI から API キーを登録するフローは提供しません。利用者はコントロールパネルで用意したサービスプリンシパルキーを profile に登録します。

## 認証情報

profile v1 の `credentials` にある次のサービスプリンシパル項目を、標準の認証設定フローの対象とします。

- `service_principal_id`
- `service_principal_key_kid`
- `private_key` または `private_key_path`

サービスプリンシパルキーの発行・管理方法は[さくらのクラウド サービスプリンシパル](https://manual.sakura.ad.jp/cloud/controlpanel/service-principal.html)を参照してください。profile v1 は[共通 profile ガイド](https://github.com/sacloud/guide/tree/main/profile)に記載されています。

現行 CLI は sacloud-sdk-go v0.3.0 の `saclient.ProfileOp` で profile を読み書きします。保存する属性名と形式は、この SDK が実際に読み込める形式に合わせます。profile v0 形式を使う場合のサービスプリンシパル属性は `ServicePrincipalID`、`ServicePrincipalKeyID`、`PrivateKeyPEMPath` です。profile v1 の snake_case 属性や YAML 形式を SDK が読み込めることを確認せずに書き出してはいけません。

API キーの `access_token` と `access_token_secret` は profile v1 の形式に存在しますが、`skr` はそれらを対話的に入力・保存する設定フローを提供しません。既存の API キーを使う利用者は、sacloud-sdk-go がサポートする形式に従って profile ファイルを手動編集します。この決定は既存 profile や SDK の API キー認証を無効化するものではありません。SimpleMQ のキュー単位 API キーは共通認証情報ではなく、[SimpleMQ コマンドの設計](simplemq-api.md)に従って個別に扱います。

## 設定コマンド

サービスプリンシパルとサービスプリンシパルキーは、[コントロールパネルの手順](https://manual.sakura.ad.jp/cloud/controlpanel/service-principal.html)で事前に作成します。秘密鍵に対応する公開鍵を登録してキーを作成し、コントロールパネルで確認できるサービスプリンシパル ID と KID を控えます。`skr config create` または `skr config edit` で、その認証情報を profile に設定します。

`skr config create [name]` で新しい profile を作成し、`skr config edit [name]` で既存 profile を編集します。既存 usacloud の `config create/edit` と同様に、対話利用とフラグによる非対話利用の両方を提供します。専用の `auth login` / `auth setup` コマンドは追加せず、認証情報を profile の設定項目として扱います。

`create` の名前を省略した場合は `default` を使い、同名 profile があればエラーにして既存内容を上書きしません。作成した profile を現在の profile にするかは `--use` で明示します。`--use` を省略した場合は現在の profile を変更しません。`edit` の名前を省略した場合は現在の profile を対象とし、名前を指定した場合はその profile を編集します。`edit` も既定では現在の profile 選択を変更せず、切り替えを希望する場合に `--use` を指定します。ヘルプでは、コントロールパネルで作成したサービスプリンシパルキーをローカル profile に設定する操作であることを明記します。

対話フローは次の順序にします。

1. `create` では profile 名を確認します。`edit` では対象 profile を確認し、既存の CLI／SDK 設定を保持します。
2. サービスプリンシパル ID を入力します。
3. キーの KID を入力します。
4. 対応する秘密鍵 PEM ファイルのパスを入力します。秘密鍵の内容は端末入力させず、profile にも複製しません。
5. PEM をローカルで読み取り、秘密鍵として利用できることを検証します。`edit` では項目ごとに現在値を示して変更を確認し、入力を省略した項目は変更しません。秘密鍵の内容は表示せず、ファイルパスも既定では伏せます。認証項目をまとめて安全に profile へ書き込みます。
6. `create` では `--use` が指定された場合だけ、作成した profile を現在の profile に設定します。`edit` で現在の profile 以外を編集した場合、対話利用では完了後に切り替えを確認し、非対話利用では `--use` が指定された場合だけ切り替えます。
7. 成功時は profile 名と保存先だけを表示し、ID、KID、秘密鍵や認証トークンは表示しません。

対話利用では、編集対象に既存値がある場合、変更するかを項目ごとに確認します。ID と KID は利用者が変更内容を判断できるよう既存値を表示できますが、秘密鍵の内容は表示しません。秘密鍵ファイルのパスも既定では伏せ、現在の鍵を維持するか、新しいファイルを指定するかを確認します。入力を省略した項目は変更しません。usacloud の `config edit` と同様に、秘密鍵ファイルの存在、通常ファイルであること、アクセス権を確認し、問題があれば警告します。アクセス権が広い場合に続行を許可するか、権限修正を必須にするかは実装時に決定します。

対話を使わない環境向けには、profile 名を位置引数で受け取ります。認証情報は `--service-principal-id`、`--service-principal-key-id`、`--private-key-file` で指定できます。CLI の属性名は現行 SDK の `ServicePrincipalKeyID` に合わせます。秘密鍵そのものを引数で受け取る `--private-key` は設けません。ID と KID は秘密鍵ではありませんが、プロセス一覧やシェル履歴に残る可能性があるため、対話利用を標準として案内します。

`create` では3項目をすべて指定する必要があります。`edit` では1項目以上の指定を必須とし、指定した項目だけ更新して他の認証情報と CLI／SDK 設定を保持します。`--use` は対話・非対話のどちらでも profile 選択を切り替えます。

`config create` は `--use` が指定されたときだけ profile を現在の profile に設定します。`config edit` は現在の profile を編集する場合は選択を変えず、それ以外では対話利用時に切り替えを確認します。非対話時は `--use` が指定された場合だけ切り替えます。`config show` は従来の JSON 出力を維持しながら、アクセストークン、アクセストークンシークレット、秘密鍵の内容とパスを伏せて表示します。API キー項目が既にある profile にサービスプリンシパルを設定する場合は、既存値を無断で削除せず、profile v1 の優先規則に従います。

既定の出力形式や zone などの任意設定は、初回の profile 作成・編集フローでは尋ねません。既存値は編集時に保持します。設定時は秘密鍵ファイルのローカル検証まで行い、リモート API 呼び出しを成功条件にしません。完了表示は profile の作成・更新を示し、リモート認証済みと誤解させないようにします。

## 実装時の確認事項

- 採用する sacloud-sdk-go のバージョンが、サービスプリンシパル認証に必要な profile 属性と秘密鍵ファイル参照をサポートすることを確認します。
- 現行 SDK の `ProfileOp` が扱う保存形式と属性名を確認し、対応しない形式は CLI から書き出しません。profile v1 形式を使う場合は、SDK がその形式を読み込めることを先に確認します。
- 認証秘密情報を含む profile の権限、作成・更新時の安全な書き込み、表示時のマスキングを確認します。
- profile 書き込みの失敗時に不完全な認証情報を残さない方法と、profile を current にするタイミングを定義します。
- コントロールパネルで登録する公開鍵と profile が参照する秘密鍵の対応を、ローカル検証で確認できる範囲を調査します。
- ローカルの sakumock 等で、認証設定後の profile 選択と SDK クライアント生成をテストします。実際の認証情報をテストやログに出力しません。

## 参考にした既存の CLI フロー

usacloud の `config edit` は、対話モードでは項目単位で既存値を示し、変更の要否を確認します。非対話モードではフラグで値を指定でき、profile の選択切り替えには `--use` を使います。skr ではこの操作形を参考にしつつ、サービスプリンシパルキーを標準とし、API キー、zone、既定出力形式を今回の対話対象から除きます。秘密情報の表示やファイル権限の扱いは skr の安全要件に合わせます。
