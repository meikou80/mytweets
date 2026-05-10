# Xポスト検索

お気に入りのユーザーやキーワードに一致するXのポストを公式APIから取得し、SQLiteに保存してローカルで検索します。

## Requirements

* Go 1.25+
* X Developer App の Bearer Token
* SQLite用の追加Cコンパイラは不要です

## Setup

Bearer Tokenを環境変数に設定します。

```sh
export X_BEARER_TOKEN=...
```

Windows PowerShellでは次のように設定します。

```powershell
$env:X_BEARER_TOKEN="..."
```

既存の設定を取り込みたい場合は、`config.example.json` を参考に `config.json` を作成します。
初回アクセス時にSQLiteへ取り込まれ、以後はブラウザ上で保存した設定が使われます。

```json
{
  "users": ["XDevelopers"],
  "keywords": ["生成AI", "Go言語"],
  "keyword_search_mode": "separate",
  "search_endpoint": "recent",
  "lookback_days": 365,
  "language": "ja",
  "exclude_reposts": true,
  "exclude_replies": true,
  "max_pages_per_refresh": 1
}
```

## Usage

```sh
go run .
```

```
Usage of mytweets:
  -a string
        server address (default ":8989")
  -config string
        initial config import path (default "config.json")
  -db string
        database path (default "tweets.db")
```

ブラウザで `http://localhost:8989` を開き、「取得設定」を編集して「設定を保存」を押します。
「Xから取得」を押すと、SQLiteに保存された設定で取得します。
「CSV出力」を押すと、現在の検索欄とソース絞り込みに一致する保存済みポストをCSVでダウンロードします。

`config.json` はDBに設定がまだ保存されていない初回だけ読み込まれます。
一度ブラウザから保存した後は、`config.json` を編集しても自動同期されません。

## Keyword Search Mode

`keyword_search_mode` はキーワード検索の範囲を決めます。

* `separate`: ユーザー投稿取得と、X全体のキーワード検索を別々に実行します。
* `tracked_users`: `users` に書いたユーザー、かつ `keywords` に一致するポストだけを検索します。ユーザーの全投稿は取得しません。

`tracked_users` の場合は、できるだけ `(from:user1 OR from:user2) (keyword1 OR keyword2)` のようにまとめてRecent Searchを呼びます。X APIのクエリ長制限を超える場合だけ複数リクエストに分割します。

## Search Endpoint

`search_endpoint` は検索対象期間を決めます。

* `recent`: `/2/tweets/search/recent` を使います。直近7日間が対象です。
* `all`: `/2/tweets/search/all` を使います。`lookback_days` または `start_time` / `end_time` で期間を指定します。

過去1年分を検索する例:

```json
{
  "users": ["sakamoto_582", "InterviewCat582"],
  "keywords": ["自社開発"],
  "keyword_search_mode": "tracked_users",
  "search_endpoint": "all",
  "lookback_days": 365,
  "language": "ja",
  "exclude_reposts": true,
  "exclude_replies": true,
  "max_pages_per_refresh": 1
}
```

`all` はX APIのFull-Archive Searchなので、利用できるAPIアクセス権限が必要です。

## Notes

* `separate` のユーザー追跡は `GET /2/users/by` と `GET /2/users/{id}/tweets` を使います。
* キーワード追跡は `search_endpoint` に応じて `GET /2/tweets/search/recent` または `GET /2/tweets/search/all` を使います。
* `X_BEARER_TOKEN` が未設定でもサーバーは起動しますが、取得時にエラーになります。
* 設定はSQLiteに保存されます。`config.json` は初回取り込み用の後方互換ファイルです。

## License

MIT

## Author

Yasuhiro Matsumoto (a.k.a. mattn)
