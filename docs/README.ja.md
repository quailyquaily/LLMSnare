# LLMSnare

`llmsnare` は、LLM プロファイルに対してコンテキスト忠実度ベンチマークを実行する Go CLI です。

このツールが解こうとしているのは、もっと狭くて実務的な問いです。LLM が agent として動くとき、本当に必要な文脈を読み、指示に従い、必要な作業だけをしたか。

Languages: [English](../README.md) , [中文](./README.zh.md)

## Online Arena

https://mistermorph.com/llmsnare/arena/

## Getting Started

### 1. 最新 binary をダウンロード

[GitHub Releases](https://github.com/quailyquaily/LLMSnare/releases/latest) から自分の OS とアーキテクチャに合った最新版 binary をダウンロードし、`llmsnare` を `PATH` に置いてください。

### 2. 初期化

```bash
llmsnare init
```

デフォルトでは `~/.config/llmsnare/` 配下に `config.yaml` とサンプル case が生成されます。

### 3. 設定を編集

`~/.config/llmsnare/config.yaml` を開き、`config.yaml` の profile と API key を埋めてください。

各 profile は 1 つの LLM を表します。最低 1 つの profile を追加する必要があります。

### 4. case を確認

```bash
llmsnare cases
```

### 5. benchmark を 1 回実行

```bash
llmsnare run --case <case_name>
```

## なぜ LLMSnare か

いま人気の benchmark の多くは終点を見ます。答えが正しいかどうかです。

ですが、agent の操作過程はほとんど測られていません。必要な情報を先に読んだか、行動前にどこまで読めていたか、既存のツールやコードを再利用したか。

- 例 1: LLM に文体規約を参照して文章を書くタスクを与えたのに、文体規約ファイルを読まずに書き始める。この場合、その LLM は手を抜いています。指示追従が弱いということです。
- 例 2: LLM に少し誤った指示を与えても、tool calling で正しい文脈を取れるようにしておく。それでも正しい文脈を見たあとで誤った指示に引きずられるなら、エラーからの回復能力が足りません。
- 例 3: LLM に曖昧な指示を与えても、tool calling で完全な文脈を取れるようにしておく。その曖昧さの中を長くさまよい、多数の tool calling をしないと理解できない、あるいは結局理解できないなら、基礎能力に問題があります。

LLMSnare が測るのは agent 行動 benchmark であって、結果品質ではありません。先に読む、後で書く、既存情報を再利用する、エラーから回復する。そして反復実行でもその規律を保てるかを見ます。

## 簡単な比較

| Benchmark | 何を走らせるか | どう採点するか | 何を見落とすか |
|---|---|---|---|
| HumanEval / MBPP | 小さな単体コーディング問題 | 最終コードに対する単体テスト | リポジトリ探索がほぼなく、過程シグナルも薄い |
| SWE-bench | 実リポジトリ上の GitHub issue 修正 | テスト通過、issue 解決 | 結果には強いが、書く前の読み込み行動には弱い |
| WebArena / OSWorld | ブラウザや OS のタスク | タスク成功、行動列 | UI agent には向くが、リポジトリ編集規律には向かない |
| LLMSnare | agent 的なタスク | ツールログ、行動指標、case ルール、最終書き込み | mock `rootfs/` と独自 case 集を使う |

## LLMSnare は何を測るか

- `llmsnare` は同じ case 集に対する反復実行を前提にしています。通常の使い方は結果を timeline に蓄積し、単発結果ではなく推移を見ることです。
- 現在は、ツール利用下でのリポジトリ読解規律を測ります。たとえば `list_dir`、`read_file`、その後に `write_file` です。
- ツール呼び出しログから、`read_file_calls`、`write_file_calls`、`list_dir_calls`、`read_write_ratio`、`pre_write_read_coverage` などの行動指標を取ります。
- カスタムのテストセットを使えます。case 単位のルールで、必須読込、helper 再利用、誤ったパスからの回復、出力規約などを検査できます。
- 理由をファイルに残します。各 run には減点、加点、最終書き込み、ツールログが記録され、監査や追跡に使えます。

## 何を測れないか

- 汎用知能のランキングには使いません。
- モデルが本当に「深く理解している」「賢い」とは証明できません。LLMSnare の指標は本質的に行動の代理だからです。
- 実リポジトリでの end-to-end 正しさ benchmark の代わりにはなりません。

## 現在の制約

- 内蔵 case 集は非常に小さく、例として置かれているだけです。実際の評価では、自分で必要なテストセットを書く必要があります。公式テストセットの詳細は公開していませんが、データは [LLMSnare Arena](https://mistermorph.com/llmsnare/arena/) で見られます。
- 結論は実行したテストセットに依存します。人ごとに違うテストセットを走らせれば、違う結論になります。
- 現在の mock `rootfs/` は、対策されやすい可能性があります。
- 現在の行動指標で示せる理解力の証拠は限定的です。
- いまは単一タスクのスナップショットだけで、長い multi-turn 実行軌跡には対応していません。

## 今後の方向

- 行動シグナルを増やします。低価値の反復探索や、ツールエラー後の回復などです。
- より長い multi-turn case を追加し、タスクが長くなると行動が漂うかを測ります。
- case 集を広げ、コミュニティからの case 提出も受け入れます。

## コマンド

詳細なコマンドは [cmd.md](./cmd.md) を見てください。

## 設定

詳細な設定は [config.md](./config.md) を見てください。

## Case ファイル

各 benchmark case は、形が固定されたディレクトリです。

例:

```text
benchmarks/
  read_write_ratio_sample/
    case.yaml
    rootfs/
      main.go
      docs/format.txt
```

`rootfs/` はメモリに読み込まれ、mock ツール経由でモデルに公開されます。実際の working tree のように編集されるわけではありません。

コミュニティ向けの case 作成ガイドは [case_guide.md](./case_guide.md) を見てください。

完全な case schema は [case_format.md](./case_format.md) を見てください。

対応する `check.type` は [check_reference.md](./check_reference.md) を見てください。

### 内蔵 Case

内蔵 case は最も単純なサンプルです。`llmsnare init` はこの内蔵 case を `~/.config/llmsnare/benchmarks/` 配下へコピーするので、その sample を元に自由に修正できます。

## HTTP API

API の詳細は [api.md](./api.md) を見てください。

## 開発

テスト実行:

```bash
go test ./...
```
