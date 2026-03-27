---
name: branch-strategy
description: Refloom のブランチ戦略・PR ワークフロー・リリースフロー・バージョニング・オートマージの定義
---

# branch-strategy

## ブランチ命名規則

| ブランチ | 用途 | マージ先 |
|---------|------|---------|
| `main` | リリースブランチ (保護) | — |
| `feature/<name>` | 新機能 | main (PR) |
| `fix/<name>` | バグ修正 | main (PR) |
| `docs/<name>` | ドキュメントのみ | main (PR) |

## PR ワークフロー

1. `feature/*` or `fix/*` ブランチを作成
2. 実装 + `make ci` パス確認
3. `gh pr create` で PR 作成
4. CI (`ci-status` ジョブ) が全パスで必須
5. docs-only / CI 設定のみの変更は code checks スキップ (paths-filter)
6. マージ後ブランチ削除

## PR 分類と CI 挙動

| PR 種別 | paths-filter | CI ジョブ | ci-status |
|---------|-------------|----------|-----------|
| コード変更 (*.go, *.py, go.mod 等) | `code=true` | フル実行 (lint+test+build) | 全パスで PASS |
| CI/ワークフロー設定のみ | `code=false` | スキップ | 即 PASS |
| ドキュメントのみ (*.md) | `code=false` | スキップ | 即 PASS |

## ブランチ保護ルール (main)

- PR 必須 (直 push 禁止、admin 含む)
- `ci-status` ステータスチェック必須
- Force push 禁止

## バージョニング (SemVer)

v0.x 期間中の判断基準:

| 変更種別 | バージョン | 例 |
|---------|----------|-----|
| 破壊的変更 (DB スキーマ、CLI フラグ削除、config 変更) | MINOR (0.x.0) | v0.3.0 |
| 新機能 (コマンド追加、検索改善、新フォーマット対応) | MINOR (0.x.0) | v0.3.0 |
| バグ修正、パフォーマンス改善、リファクタリング | PATCH (0.x.y) | v0.2.1 |
| 依存更新、CI 改善、ドキュメント修正 | PATCH (0.x.y) | v0.2.1 |

v1.0.0 の条件: 6 ヶ月以上の安定運用 + DB スキーマ安定 + CLI 安定 + 100 冊スケールテスト

## リリースフロー

1. CHANGELOG.md の `[Unreleased]` → `[X.Y.Z] - YYYY-MM-DD` に変換
2. コミット + PR → マージ
3. `git tag vX.Y.Z` → push (タグはブランチ保護の対象外)
4. release.yml 自動実行: dist ビルド → GitHub Release → E2E テスト
5. homebrew-tap の Formula 更新 (SHA256 差し替え)

## オートマージポリシー

| PR 種別 | 対応 |
|---------|------|
| Dependabot patch/minor | `ci-status` パス後に自動マージ |
| Dependabot major | 手動レビュー必須 |
| 手動作成の PR | 手動マージ (レビュー推奨) |

## 実験・検証

- git worktree を使用すること
- `feature/*` ブランチで作業、main への直コミット禁止
