# GO RUNCARNATION

![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white)
![GitHub License](https://img.shields.io/github/license/quadrans-muralis/runcarnation?style=flat-square&color=blue)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey?style=flat-square)
![Build Status](https://img.shields.io/github/actions/workflow/status/quadrans-muralis/runcarnation/ci.yml?branch=main&style=flat-square&logo=github)
![Last Commit](https://img.shields.io/github/last-commit/quadrans-muralis/runcarnation?style=flat-square&color=blue)

**「動かしっぱなしのアプリが、リモートの更新を検知した瞬間に生まれ変わる。」**

GO RUNCARNATION は、Gitリポジトリの状態を監視し、GitHub 上に新しいコミットが来たら自動で **pull → 再ビルド → 再起動** までやってくれるCLIツールです。名前の由来は `run`（実行）と `reincarnation`（生まれ変わり）。開発サーバーやBotなど「常に最新のコードで動かしっぱなしにしたいプロセス」を、`git pull` して手動で再起動する手間から解放します。

対話型のプロンプトからも、CIスクリプトなどに組み込みやすい非対話モード（コマンドライン引数）からも使えます。

---

## 目次

- [特徴](#特徴)
- [必要環境](#必要環境)
- [インストール](#インストール)
- [使い方](#使い方)
  - [対話モード](#対話モード)
  - [非対話モード（CLI引数）](#非対話モードcli引数)
- [コマンド一覧](#コマンド一覧)
- [`runcarnation` の動作フロー](#runcarnation-の動作フロー)
- [ビルド済みバイナリの保存場所](#ビルド済みバイナリの保存場所)
- [言語の切り替え](#言語の切り替え)
- [ライセンス](#ライセンス)

---

## 特徴

- **ローカル ⇔ GitHub の状態確認**
  ブランチの差分（ahead / behind）、作業ツリーの汚れ、リモートURL、diffの統計をコマンド一つでひと目で確認できます。

- **`run` — 一度だけ最新化して実行**
  起動前に GitHub と同期し（必要なら pull）、そのままアプリケーションを実行します。

- **`runcarnation` — 監視しながら常駐運用**
  アプリを実行しつつリモートを定期監視し、新しいコミットを検知したら **Kill → pull → 再ビルド → 再起動** を自動で繰り返します。開発中のサーバープロセスや常駐Botに最適です。

- **`go build` ベースのクリーンな実行**
  `go run .` ではなく、一時バイナリをその都度ビルドしてから直接起動・Killするため、プロセスの管理がシンプルで確実です。

- **ビルド済みバイナリの永続保存＆一括掃除**
  OSごとの標準設定ディレクトリに保存され、`demolition` コマンドでいつでもまとめて削除できます。

- **多言語対応**
  英語 / 日本語 / 中国語をコマンド一つで切り替え可能です。

- **短縮コマンド（エイリアス）**
  長いコマンド名は4文字以内の短縮形でも同じ動作をします（例: `check` → `chk`）。

- **対話モード／非対話モードの両対応**
  引数なしで起動すれば対話型プロンプト、引数付きで起動すればワンショット実行と、用途に応じて使い分けられます。

---

## 必要環境

このツール自体はコンパイル済みバイナリとして動きますが、**実行時に以下の外部コマンドを呼び出します**。事前にインストールし、PATHを通しておいてください。

| コマンド | 用途 |
| --- | --- |
| `git` | 状態確認・fetch・pull・diff など、ほぼ全機能で使用 |
| `go` | `run` / `runcarnation` 実行時のビルド（`go build`）に使用 |

また、対象ディレクトリは以下を満たしている必要があります。

- Gitリポジトリであること（`git init` 済み）
- `origin` リモートが設定されていること
- `run` / `runcarnation` を使う場合は、ビルド対象のディレクトリ直下に `main` パッケージを含む Go プロジェクトであること

> **Note:** ブランチ名の取得に `git branch --show-current` を使用しているため、Git 2.22 以降が必要です。

---

## インストール

### ソースからビルド

```bash
git clone https://github.com/yourname/go-runcarnation.git
cd go-runcarnation
go build -o go-runcarnation .
```

### リリース済みバイナリを使う

GitHub Releases ページから対象OS向けのアーカイブをダウンロードし、展開してPATHの通った場所に置いてください。

---

## 使い方

### 対話モード

引数なしで起動すると、対話型のプロンプト `GO RUNCARNATION>` が表示されます。コマンドを入力してEnterしてください。

```bash
./go-runcarnation
```

```
GO RUNCARNATION> help
```

`check` / `status` / `remote` / `fetch` / `diff` / `run` / `runcarnation` を実行すると、続けて対象リポジトリのパスを聞かれます。

```
GO RUNCARNATION> check
Repository path: /path/to/your/repo
```

### 非対話モード（CLI引数）

スクリプトやCIから呼び出したい場合は、サブコマンドと `--path` を直接引数で渡せます。プロンプトの入力待ちは発生しません。

```bash
./go-runcarnation check --path=/path/to/your/repo
./go-runcarnation run --path=/path/to/your/repo
```

`--lang` で表示言語もその場で指定できます。

```bash
./go-runcarnation check --path=/path/to/your/repo --lang=jp
```

`help` / `demolition` / `language` のようにリポジトリパスを必要としないコマンドは `--path` なしで実行できます。

```bash
./go-runcarnation demolition
```

---

## コマンド一覧

| コマンド | 短縮形 | 説明 |
| --- | --- | --- |
| `check` | `chk` | リポジトリをスキャンしてGitHubと比較（ahead / behind 判定） |
| `status` | `stat` | ローカルの作業ツリーを確認（未コミットの変更を検出） |
| `remote` | `rmt` | `origin` の接続先URLを表示 |
| `fetch` | `fch` | GitHubから最新情報を取得（pullはしない） |
| `diff` | - | ローカルとGitHubの差分（`--stat`）を表示 |
| `ignored` | `ign` | `.gitignore` で無視されているファイルを絶対パスで一覧表示 |
| `changed` | `chg` | 未コミットの変更があるファイルを絶対パスで一覧表示 |
| `run` | - | GitHubと同期（必要ならpull）してから、アプリケーションを1回だけ実行 |
| `runcarnation` | `rcn` | アプリを実行しつつGitHubを監視し、更新があれば自動で再起動を繰り返す |
| `demolition` | `demo` | 保存されているビルド済みバイナリを一括削除 |
| `language` | `lang` | インターフェースの言語を切り替え（en / jp / cn） |
| `help` | - | コマンド一覧を表示 |
| `exit` | - | 終了 |

---

## `runcarnation` の動作フロー

```
1. fetch → ローカルがbehindならgit pull
2. go build で一時バイナリを生成
3. バイナリを起動
4. 実行中、5秒おき（pollInterval）にGitHubを監視
     ├─ 新しいコミットを検知 → プロセスをKill → 1. に戻って再pull・再ビルド・再起動
     └─ アプリが自分で終了（正常終了・クラッシュ問わず）→ そこでループ終了
```

つまり「リモートの更新による再起動」と「アプリ自身の終了」は明確に区別されており、**アプリが自ら終了した場合はそこで打ち切られ**、勝手に再実行されることはありません。

---

## ビルド済みバイナリの保存場所

`run` / `runcarnation` で作られる一時バイナリは、OSの標準設定ディレクトリ配下に保存されます。

| OS | 保存先 |
| --- | --- |
| Windows | `%APPDATA%\GO_RUNCARNATION\runcarnation` |
| macOS | `~/Library/Application Support/GO_RUNCARNATION/runcarnation` |
| Linux | `$XDG_CONFIG_HOME/GO_RUNCARNATION/runcarnation`（通常 `~/.config/...`） |

通常は使用後に自動削除されますが、クラッシュ時などに残ってしまったファイルは `demolition` コマンドでまとめて掃除できます。

```
GO RUNCARNATION> demolition
```

実行ログは同様にOS標準の設定ディレクトリ配下（`GO_RUNCARNATION/log`）に保存され、14日を過ぎた古いログは次回起動時に自動で削除されます。

---

## 言語の切り替え

```
GO RUNCARNATION> language
  en. English
  jp. 日本語
  cn. 中文

Select language: jp
言語を日本語に変更しました。
```

---

## ライセンス

LICENCEファイルを参照
