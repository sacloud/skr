# ADR 0045: API コマンドを constructor で初期化する

- Status: Accepted
- Date: 2026-10-15

## Context

生成 API コマンド群は `SetRuntime` と `SetFactory` を別々に呼び出して初期化していました。コマンド群を構築した後に依存を段階的に設定するため、初期化漏れや半端な状態を作れるうえ、CLI の登録コードも2つの呼び出しに分かれていました。

`Runtime` は JSON デコード、出力の検証・書き込み、操作固有の入力変換に必要です。SDK API factory も認証済みクライアントと SDK 操作を生成するために必要であり、いずれも生成 package が CLI の非公開処理へ直接依存せず受け取る必要があります。

## Decision

生成器は `New<CommandType>(runtime, factory) <CommandType>` を生成します。constructor は全操作のコマンドに両方の依存を設定して、初期化済みのコマンド群を返します。生成 API コマンド群から `SetRuntime` と `SetFactory` をなくし、CLI とテストはコマンド群を constructor で作成します。

手書きの Container Registry コマンド群も同じ `NewCommands(runtime, factory)` 形式にします。AppRun Dedicated は SDK API factory を個別に持たず `Runtime.NewClient` を利用するため、`NewCommands(runtime)` を使います。

## Consequences

- CLI 初期化は Runtime と factory を一か所で渡し、依存注入の呼び忘れを防ぎます。
- テストが API factory を差し替える場合も、新しいコマンド群を constructor で構築します。
- Runtime と API factory の分離、および SDK クライアント生成の遅延は維持されます。
- constructor に渡す値が不完全な場合、既存どおりコマンド実行時に不足依存のエラーを返します。CLI とテストは必要な依存を明示して構築してください。

## Alternatives considered

- `SetRuntime` と `SetFactory` を維持する案: 呼び出し順序と部分初期化を残し、CLI 初期化コードも分割されたままになるため採用しません。
- `Configure` メソッドに統合する案: 部分初期化は防げますが、初期化専用の追加 API を1つ設けるより constructor の方が構築時の依存を明確にできます。
- Runtime や factory を生成 package 内で暗黙に構築する案: CLI の trace 設定や JSON／出力処理、テスト用 API 差し替えとの境界を曖昧にするため採用しません。
