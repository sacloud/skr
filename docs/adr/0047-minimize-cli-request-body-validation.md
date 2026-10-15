# ADR 0047: CLI のリクエスト本文検証を必要最低限にする

- Status: Accepted
- Date: 2026-10-15

## Context

IaaS コマンドは、リクエストの必須項目や値の範囲、項目間の制約を CLI でも検証していました。SDK のサービス操作が行う検証や API の判定と重複すると、仕様への追従が必要になり、CLI 独自の制限で有効な入力を拒否する可能性があります。

現在利用する SDK では、各サービス操作がリクエストの `Validate` を呼び出します。Server の builder はプランの検索と NoWait の併用制約も検証します。一方、Archive の OSType は検索条件に変換する補助入力で、未知の値は検索条件へ反映されません。Names と Tags は、OSType から構築した条件の対応項目を上書きします。

## Decision

- CLI はオプションの指定有無、入力経路の相互排他、ファイルの読み込み、JSON のデコード、秘密情報の入力制約、CLI 独自構文を検証します。
- リクエスト本文の必須項目、値の範囲、サービス上の相関制約は原則として SDK と API に委ねます。CLI から `request.Validate()` を重複して呼び出しません。
- [ADR 0046](0046-common-iaas-zone-validation.md) の `validate_zone` は CLI の共通仕様として維持します。Archive の未知 OSType は、SDK が無視する補助入力の例外として CLI で拒否します。
- IaaS の ID、Name、DiskPlanID、Connection、SizeGB、CPU、MemoryGB の検証を CLI から削除します。NoWait と BootAfterCreate の相関チェック、OriginalExpressionHash の必須チェック、変更項目のない更新の拒否も削除します。SDK と API のエラーは利用者へ返します。
- Archive の OSType と Names／Tags の併用禁止は削除し、検索条件の上書きという既存 SDK の動作をそのまま公開します。
- Server の `--disk`／`--nic`、PacketFilter の `--expressions`、user-data の入力処理は維持します。これらは CLI が提供する入力形式の検証です。
- EventBus の `Set*Provider` は検証ではなく入力補完なので維持します。`request_validator` 機構も、必要最低限の検証と既存の補完処理のために残します。

## Consequences

リクエスト本文の仕様を CLI で重複実装する範囲が減ります。フラグ経路の必須指定と `--request` の相互排他は変わりません。JSON 経路では Zone と未知 OSType を除く本文の意味を CLI で判定せず、SDK へ渡します。

エラーの発生箇所と文言は変わる場合があります。SDK の検証にはクライアント生成後に実行されるものや、検索などの通信を伴うものがあります。変更項目のない更新も SDK へ渡るため、CLI が通信前に拒否するとは保証しません。OriginalExpressionHash の空値は API の判定に委ね、従来と同じエラーになるとは保証しません。

例外を追加する場合は、CLI 固有の制約、入力の変換、または下位層で適切に拒否されない補助入力である理由を記録します。SDK のフィールドが存在するだけでは、CLI に検証を追加する理由にはしません。

## Alternatives considered

- 本文の検証を CLI で網羅する方法は、SDK と API の仕様を重複管理し、独自の制限を増やすため採用しません。
- 検証をすべて削除する方法は、入力経路の安全性や未知 OSType の無視を防げないため採用しません。

## References

- [SDK の Server Create](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/server#Service.CreateWithContext)
- [SDK の Server builder](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/server/builder#Builder.Validate)
- [SDK の Archive 検索条件変換](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/archive#FindRequest.ToRequestParameter)
- [SDK の PacketFilter Update](https://pkg.go.dev/github.com/sacloud/sacloud-sdk-go@v0.3.0/service/iaas/packetfilter#Service.UpdateWithContext)
