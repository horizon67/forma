# Forma Roadmap

Status: living roadmap — non-normative

Formaの中心仮説は、coding agentへ渡す自然言語promptを、型付き・検査可能・review可能な言語へ
置き換えられることである。Forma自身がframework別code generatorになることではない。

```text
Forma source
  → parse / check / resolve
  → Resolved Intent + Acceptance Facts
  → Generation Request
  → AI coding agent + repository
  → ordinary application code
  → build / test feedback
```

言語のsyntaxとsemanticsは[`v0-primitives.md`](v0-primitives.md)、agentとの境界は
[`agent-generation.md`](agent-generation.md)に記録する。

## 現在地

| 領域 | 状態 | 現在の内容 |
| --- | --- | --- |
| 言語思想 | 方針修正済み | AI coding agentを必須の実行主体とし、Formaはpromptより強い入力を作る |
| v0言語仕様 | design draft | 10 primitives、modifier、EBNF、静的検査を定義 |
| reference front-end | 部分実装 | Lexer、Parser、AST、Checker、stable identity、Resolved Intent、Source Map、admin-flow Acceptance Facts |
| CLI | 最小実装 | `forma check`、`forma resolve`、`forma request`、coverageを検査する`forma verify` |
| Generation Request | 最小slice | `v0alpha1` full request、stable fact ID、immutable requestに対するcoverage集合照合。incremental changeは未実装 |
| agent execution | 初回実測済み | standalone Go repositoryへ管理画面flowを実装し43/43 factsを確認。自動repair loopは未実装 |
| incremental update | 未実装 | intent差分を既存repositoryへ適用する実験が必要 |
| Go admin/conformance | 凍結prototype | 意味抽出のprobeとして保存。正式generator/profile architectureにはしない |

## 1. Forma sourceをparse・checkできる

### 目的

coding agentへ渡す前に、application intentの構文・参照・型・静的semanticsを確定する。

### 実装する

- design draftとParser、AST、Checkerのsurface syntaxを一致させる
- `forma fmt`でcanonicalなsource layoutを作る
- inherited constraintを合成する
- defaultと`required readonly` producerを検査する
- 省略projection、action参照、navigationを一意に解決する
- string/regex escape setを仕様どおり検査する
- diagnosticをstable code、source span、修正案付きで返す
- `forma explain`で暗黙に解決した意味を人間向けに表示する

### Exit criteria

- 完全例を`forma check`できる
- typo、型不一致、不正遷移、permission不整合をagent実行前に拒否できる
- sourceの意味がmodelやnetwork inferenceに依存しない
- 新しい構文を可読性、semantic necessity、target neutralityで評価できる

## 2. Resolved Intentを安定して出力できる

### 目的

Forma sourceの省略と参照を解消し、coding agentが再解釈しなくてよいmachine-readableなapplication
intentを作る。

### 実装する

- [x] 公開概念を`Semantic IR`から`Resolved Intent`へ変更する
- [x] `forma resolve`でcanonical JSONを出力する
- entity、field、constraint、state、action、permission、page capability、navigationを保持する
- semantic nodeへsource位置に依存しないstable identityを与える
- Source Mapを別出力にして、nodeを現在のForma sourceへ戻せるようにする
- sourceの省略から導出したprojection、access、Submit Intentを説明可能にする
- Resolved Intentのversioningと互換性方針を定める
- [x] list/detail/editの正常系・拒否系Acceptance Factsをstable ID付きで出力する

### Exit criteria

- 同じsourceとfront-end versionからbyte-identicalなResolved Intentを得る
- file移動やcomment変更でapplication intentが変化しない
- すべてのresolved nodeをsource declarationへ追跡できる
- framework、route、SQL、component、directoryの語彙を含まない

決定性を要求するのはここまでの意味解決であり、target application codeのbyte identityではない。

## 3. Resolved Intentをcoding agentへ渡して一つのapplicationを実装できる

### 目的

固定lowererを作らず、coding agentが実際のrepository contextを読んでForma intentを実装できるか検証する。

### Generation Request

最小requestは次を含む。

- Resolved Intent
- target-neutralなAcceptance Facts
- Source Map
- 初回生成か変更適用かを表すrequested change
- userまたはrepositoryが持つarchitecture constraintsへの参照

各Acceptance Factはstable IDを持つ構造化nodeとし、散文promptにはしない。agentが作るtestは対応する
fact IDを参照し、requestのfact ID集合とtest coverageの集合を機械的に照合する。

最小のfull requestは`forma request`として実装済みである。
[`../experiments/admin-agent-e2e`](../experiments/admin-agent-e2e/README.md)のlist/detail/edit flowを
golden化し、通常のstandalone Go repositoryへの初回agent実装と43 factsのcoverage照合まで完了した。
incremental requested changeと、独立agentまたは実在repositoryでの再現確認が次の段階である。
次のincremental experimentでは
[`implementation-policy-manifest-proposal.md`](implementation-policy-manifest-proposal.md)の最小policyを
同乗させ、「何を作るか」と「何を使って作るか」を分離したまま検証する。

repositoryのframework、library、file構成はFormaがprofileとして再定義しない。agentが既存codeとpolicyから
読み取り、必要な実装を選ぶ。

### 最初の実験

1. [x] repository規約を持つ小さなstandalone Go repositoryを用意する。
2. [x] 管理側のUser list/detail/edit flowを追加する。
3. [x] Resolved IntentとAcceptance Factsを正本としてagentへ渡す。
4. [x] repository固有のcodeとtestを作り、既存build/testを通す。
5. [x] Formaに存在しない実装判断と、意味の不足を分けて記録する。
6. User signup/signin flowとidentity axisは別probeとして実施する。

### Exit criteria

- Forma coreへframework別generatorを追加せずapplicationが動く
- agentが既存repositoryのarchitectureとconventionを再利用する
- Acceptance Factsからtarget固有の正常系・否定系testを作れる
- 実装上の判断と、Formaに不足した意味を区別して報告できる

## 4. build/test failureをagentへ戻して修正できる

### 目的

一度の生成成功ではなく、失敗を観測して修正するagent loopを完成させる。

```text
Generation Request
  → inspect repository
  → edit
  → build / test / lint
  → structured feedback
  → repair
  → repeat until success or genuine blocker
```

### 実装する

- [x] stage、status、command、diagnostic、関連intent node、fact coverageを持つ`v0alpha1` feedback型
- [x] required fact IDとcoverageの完全一致、未知・重複・未参照を拒否するvalidator
- [x] immutableなrequestとfeedback JSONを検査する`forma verify`
- compiler errorとrepository build/test failureの分離
- Source Mapを使ったForma declarationへの関連付け
- agentが意味を勝手に弱めず、実装を修正するretry policy
- [x] fact ID、repository固有test reference、resultを持つcoverage report

### Exit criteria

- buildまたはtest failureから少なくとも一度自動修正して成功できる
- failureがFormaの不足なら、codeで回避せず人間へintent gapとして返せる
- 合否を「agentがそう思う」ではなく、repository commandの結果で確認できる

## 5. Formaの変更から既存applicationをincrementalに更新できる

### 目的

target codeを毎回破棄せず、通常のrepositoryを安全に進化させる。

### 実験する変更

- fieldの追加・rename・constraint変更
- listのsearch/filter/page size変更
- state valueとtransitionの追加
- permission変更
- pageとactionの追加・削除

### 実装する

- 前回と今回のResolved Intent差分
- source renameと削除を区別できるidentity/change model
- repositoryの既存手書きcodeを保つ編集境界
- migrationやtest更新を含むagent plan
- staleな実装とAcceptance Fact coverageの検出

### Exit criteria

- full regenerationなしでintent差分を既存repositoryへ適用できる
- 無関係な手書きcodeを壊さない
- semantic changeとagentのimplementation refactorをreview上で区別できる
- update後のbuild/testが成功し、追加・変更されたAcceptance Factsを覆う

## v1候補は実例から設計する

v1 syntaxを先に増やさず、agent generation experimentで表現できなかったapplication intentから必要な
semantic axisを抽出する。

現在の候補:

- expression、invariant、derived value、precondition
- action argument、changes、atomic postcondition
- occurrence、effect、notification、schedule、retry
- public identity、credential、ownership
- aggregate、join、inverse relation、cascade rule
- schema/data migration intent
- i18n copyとdesign intent

注文・注文明細・在庫probeは[`order-approval-proposal.md`](order-approval-proposal.md)、式レイヤは
[`expression-proposal.md`](expression-proposal.md)、public identityは
[`public-membership-proposal.md`](public-membership-proposal.md)で検討している。

## 凍結した方向

次をForma coreのroadmapから外す。

- frameworkごとの決定的lowerer
- target profile capability matrix
- Application/Deployment Profile registry
- Formaが保守するprofile conformance adapter
- artifactのbyte-identicalな破棄・再生成
- 二つ目のframework generatorを作ることで中心仮説を検証すること
- content-addressedなgenerated artifact cacheを正しさの境界にすること

[`../experiments/admin-e2e`](../experiments/admin-e2e/README.md)と
[`../experiments/conformance`](../experiments/conformance/README.md)は削除せず、これらの境界がなぜFormaを
code-generator projectへ近づけるかを示す凍結prototypeとして残す。

## Roadmap全体の原則

- Forma sourceの可読性を生成速度より優先する。
- AIは実装主体だが、parse、名前解決、型、Forma semanticsは所有しない。
- Resolved Intentは実装shapeではなくapplication intentを運ぶ。
- Acceptance Factsの期待値はFormaが決め、target固有testへの変換はagentが行う。
- repositoryを通常のsourceとして尊重し、既存codeへincrementalに変更する。
- build/test failureは実装修正へ使い、intentを暗黙に弱めない。
- 足りない意味はframework profileではなく、必要ならForma languageへ戻して検討する。
