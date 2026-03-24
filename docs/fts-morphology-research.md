# FTS 形態素解析 リサーチノート

## 課題

SQLite FTS5 の trigram トークナイザには以下の制約がある:

1. 2文字キーワード（「文章」等）が検索不可（trigram = 3文字単位）
2. 長い日本語自然文クエリで全 trigram の AND 条件となり 0 件

## 検討した選択肢

| 方式 | 説明 | 評価 |
|---|---|---|
| (a) Go 側 kagome 前処理 | 分かち書きテキストを FTS5 unicode61 に格納 | **採用** |
| (b) C カスタムトークナイザ | SQLite FTS5 に MeCab ベースの C トークナイザを組み込み | PoC には過剰 |
| (c) クエリ前処理のみ | ingest は変えず、検索時にクエリだけ分かち書き | trigram テーブルとの互換性なし |

## 採用方式: (a) kagome + unicode61

- **ingest 時**: `kagome` (IPA辞書) でチャンクテキストを分かち書きし、`chunk_fts_seg` テーブルに格納
- **検索時**: クエリを同じく分かち書きし、OR 結合で FTS5 MATCH に渡す
- **マージ戦略**: trigram FTS と segmented FTS の両方を検索し、チャンク ID ごとに best score をマージ

### kagome の選定理由

- 純粋 Go 実装（追加の CGO 依存なし）
- IPA 辞書で日本語形態素解析の品質が十分
- `go get` のみでインストール可能

## 結果

| 指標 | Before | After |
|---|---|---|
| Keyword hit rate | 0/12 | **9/12** |
| 「文章」(2文字) FTS | 0件 | 3件 |
| 長い日本語クエリ FTS | 0件 | ヒット |

## 制約・注意点

- `chunk_fts_seg` は `content` テーブルとの連携なし（Go 側で明示 INSERT）
- reindex --fts で両テーブルを再構築可能
- kagome の辞書サイズ分だけバイナリが肥大化（約 50MB）
