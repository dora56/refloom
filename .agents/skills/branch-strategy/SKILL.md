---
name: branch-strategy
description: Refloom のブランチ戦略・PR ワークフロー・リリースフローの定義
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
5. docs-only 変更は code checks スキップ (ci.yml の paths-filter)
6. マージ後ブランチ削除

## ブランチ保護ルール (main)

- PR 必須 (直 push 禁止、admin 含む)
- `ci-status` ステータスチェック必須
- Force push 禁止

## リリースフロー

1. CHANGELOG.md の `[Unreleased]` → `[X.Y.Z] - YYYY-MM-DD` に変換
2. コミット + PR → マージ
3. `git tag vX.Y.Z` → push
4. release.yml 自動実行: dist ビルド → GitHub Release → E2E テスト
5. homebrew-tap の Formula 更新 (SHA256 差し替え)

## 実験・検証

- git worktree を使用すること
- `feature/*` ブランチで作業、main への直コミット禁止
