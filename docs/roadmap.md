# Forma Roadmap

Status: living roadmap — non-normative

## 1. 中心仮説と責任境界

Formaの中心仮説は、coding agentへ渡す自然言語promptを、型付き・検査可能・review可能なapplication
specificationへ置き換えられることである。Forma自身がframework別code generatorになることではない。

```text
Forma source
  → Go front-end: format / parse / resolve / type check / semantic check
  → Resolved Intent + Acceptance Facts
  → Generation Request
  → AI coding agent + target repository
  → ordinary application code
  → build / test feedback
```

Forma sourceは**何を作るか**を決める。Implementation Policy Manifestは**何を使って作るか**を決める。
target repositoryは既存code、dependency、architecture、build/test commandという**現在の状態**を与える。
coding agentはこの3つを統合してrepository-nativeな実装を作る。

言語のsyntaxとsemanticsは[`v0-primitives.md`](v0-primitives.md)、agentとの責任境界は
[`agent-generation.md`](agent-generation.md)、実装技術policyの案は
[`implementation-policy-manifest-proposal.md`](implementation-policy-manifest-proposal.md)に記録する。

## 2. 現在地

| 領域 | 状態 | 根拠と残る課題 |
| --- | --- | --- |
| 言語思想 | 方針確定 | AI coding agentを必須の実装主体とし、Formaはpromptより強い入力を作る |
| v0言語仕様 | design draft | 10 primitives、modifier、EBNF、静的検査を定義。未実装項目が残る |
| Go front-end | 部分実装 | Lexer、Parser、AST、Checker、stable identity、Resolved Intent、Source Mapを実装 |
| Acceptance Facts | admin slice実装 | list/detail/editの正常系・拒否系をstable ID付きで導出 |
| Generation Request | B4 current schema実装 | historical `v0alpha1` / `v0alpha2`をbyte identityごと保持し、review diffとversion metadataを持つ`v0alpha4`を検証可能 |
| agent E2E | 初回・incremental実測済み | 既存Go targetを更新し、43/43 facts、2 satisfied policies、1 preferred deviationを確認 |
| incremental update | 最初のprobe完了 | added/changed diffを適用済み。rename、削除、migrationは未検証 |
| Implementation Policy Manifest | experimental `v0alpha1` | required、preferred deviation、forbidden scanを実測済み |
| public Identity | **P1 Stage D完了** | 既存admin targetへIdentityを追加し、81/81 Facts、3 Review Requirementsを検証した |
| automated repair | **P2 first bounded loop完了** | fresh agent processでtest/build failure → repair → 81/81と、intent gap → human handoffを自動実行 |
| Expression以降 | **P3 Next / experimental slice** | self-only Invariantの`<=`まで実装。Changes、Occurrence、Effectは未決定 |
| 旧Go generator/conformance | 凍結prototype | 正式なgenerator/profile architectureにはしない |

管理画面の初回E2Eによって、次の問いにはかなり明確な「はい」が得られた。

> Formaは、AIに渡す前処理として本当に機能するか。

同じtarget repositoryを壊さずに変更する最初のprobeも成功した。Formaは一度きりの詳しいpromptではなく、
applicationを継続的に保守するsourceとして一段強い根拠を得た。public IdentityのStage Dと最初のbounded
repair loopも完了し、次はCRUD/state transitionを越えるapplication semanticsを検証する。

## 3. 優先順位

現在の実施順序を次のように固定する。

| 優先度 | Milestone | 検証する仮説 |
| --- | --- | --- |
| **P0 / first probe completed** | Incremental update + 最小Manifest | Forma差分から既存codeを壊さず更新できるか |
| **P1 / Stage D completed** | Signup/signin + Identity | public user flowに必要な意味をtarget-neutralに記述できるか |
| **P2 / first bounded loop completed** | Automated repair loop | build/test failureから意味を弱めず実装を修正できるか |
| **P3 / Next** | Expression → Changes → Occurrence → Effect | CRUD/state transitionを越えるdomain behaviorを記述できるか |
| **P4** | v0 hardening/release | front-endとschemaを第三者が再現可能なtoolとして完成できるか |

この順序は、実装が簡単なものではなく、**中心仮説を強く否定し得る実験**を先に置く。各Milestoneを
始める前にschemaを完成させず、最小のvertical sliceを実測してから一般化する。

front-endの未実装項目はすべてを先に埋めない。次のprobeを正しく表現・検査するために必要なものから
実装する。

## Milestone 1 — Front-end foundations（継続）

### 目的

coding agentへ渡す前に、application intentの構文・参照・型・静的semanticsを決定的に確定する。

### 実装済みの核

- `forma check`のLexer、Parser、AST、Checker
- stable diagnostic codeとsource span
- stable semantic identityとSource Map
- `forma resolve`によるcanonical Resolved Intent JSON
- admin-flow Acceptance Facts
- `forma request`と`forma verify`の最小slice

### probeに応じて実装する残項目

- `forma fmt`によるcanonical source layout
- inherited constraintの合成
- defaultと`required readonly` producerの検査
- 省略projection、action参照、navigationの完全な解決
- string/regex escape setの仕様どおりの検査
- `forma explain`による暗黙の意味の人間向け表示
- Resolved Intent、Source Map、Acceptance Factsのversioningと互換性方針

### Exit criteria

- 完全例を`forma check`できる
- typo、型不一致、不正遷移、permission不整合をagent実行前に拒否できる
- 同じsourceとfront-end versionからbyte-identicalなResolved Intentを得る
- すべてのresolved nodeをsource declarationへ追跡できる
- sourceの意味がmodelやnetwork inferenceに依存しない
- framework、route、SQL、component、directoryの語彙をResolved Intentへ含めない

決定性を要求するのはここまでの意味解決であり、target application codeのbyte identityではない。

## Milestone 2 — Initial agent generation（admin slice実測済み）

### 目的

固定lowererを作らず、coding agentがrepository contextを読んでForma intentを実装できるか検証する。

### 完了した実験

1. [x] repository規約を持つstandalone Go repositoryを用意した。
2. [x] 管理側のUser list/detail/edit flowをFormaで記述した。
3. [x] Resolved Intent、Source Map、Acceptance Factsを含むGeneration Requestを出力した。
4. [x] coding agentが通常のGo application codeとrepository固有testを実装した。
5. [x] 43/43 Acceptance Factsと既存build/testの成功を確認した。
6. [x] framework、route、HTTP、HTML、storageなどの実装判断をrequestへ持ち込まずに実装できた。
7. [x] fact coverageの集合照合と、distinct test数・最大facts/testの集中度を検査した。

詳細は[`../experiments/admin-agent-e2e`](../experiments/admin-agent-e2e/README.md)に記録する。

### 得られた根拠と限界

- Forma coreへframework別generatorを追加せずapplicationが動いた。
- Acceptance Factsからtarget固有の正常系・否定系testを作れた。
- targetのfile構成、route、submission方式などはagentが独立に決めた。
- 43/43は主要testを直接reviewし、単なるcoverage申告でないことを確認した。
- 初回runの時点ではincremental updateと独立再現性が未検証だった。incrementalはMilestone 3で最初の
  probeを完了した。fresh agent processによるrepairはMilestone 5で再現したが、独立agentによる初回生成や
  実在repositoryへの適用は引き続き未検証である。

このMilestoneを繰り返して二つ目のframework generatorを作ることはしない。

## Milestone 3 — Incremental update + Implementation Policy Manifest（P0 / first probe completed）

### 目的

target codeを破棄・再生成せず、Forma sourceの変更を既存repositoryへ小さく安全な差分として適用する。
同時に、「何を作るか」と「何を使って作るか」を分離したままagentへ渡せるか検証する。

### 実験前に固定するbaseline

- 現在の`app.forma`
- canonical Resolved Intent、Source Map、Generation Request
- 43 Acceptance Factsと対応する12 distinct target tests
- 現在のtarget repository commitとpassing build/test
- target内の手書き・無関係codeとして保持すべきfile

### 最初の変更候補

- `User.nickname` fieldを追加する。
- nicknameをlist、detail、editへ提示し、search対象へ追加する。
- listのlogical page sizeを20から10へ変更する。

field追加で**追加node/fact**を、page size変更で**同じsemantic nodeの期待値変更**を同時に観測する。
最終的な変更内容はexperiment開始時にbaselineとともに固定し、途中で成功しやすい要求へ変えない。

### 最小Implementation Policy Manifest

```yaml
schema: forma/implementation-policy/v0alpha1

policies:
  - id: implementation/server-rendering
    policy: required
    value: html/template

  - id: implementation/persistence
    policy: preferred
    value: database/sql

  - id: implementation/router
    policy: forbidden
    value: github.com/gorilla/mux
```

- `required`はevidence fileの存在とopaque valueの出現を検査する。
- `preferred`を使わない場合はnon-emptyな逸脱理由を要求する。
- `forbidden`は定義済みscopeをscanし、hitを無言で無視しない。
- technology名をForma coreが意味解釈しない。
- 正規化済みManifestをimmutableなGeneration Requestへ埋め込む。

### 最初のprobeで実装・検証する

- before/after Resolved Intentのsemantic diff
- requested changeでadded、changed、unchangedを区別する最小model
- 追加・変更されたAcceptance Factsの集合
- Manifest parser、normalizer、generic policy coverage validator
- existing target repositoryを読むagent instruction
- full regeneration禁止と無関係code保持のreview

最初のprobeへrename、削除、migrationの一般modelまで詰め込まない。追加と変更が成功した後、次の独立した
probeとして扱う。

### 後続のincremental probe

- fieldのrenameと削除をstable identityで区別する
- constraint変更と既存data migrationを扱う
- state value、transition、permissionを変更する
- pageとactionを追加・削除する
- removed factに対応するstaleなtarget code/testを検出する

### Exit criteria

- full regenerationなしでintent差分を既存repositoryへ適用できる
- 無関係な手書きcodeと既存architectureを壊さない
- 変更と無関係な既存Acceptance Factsが引き続き成功する
- 追加・変更されたFactsがtarget固有testで覆われ、stale coverageが残らない
- required、preferred、forbidden policyの各経路を実測する
- semantic changeとagentのimplementation refactorをreview上で区別できる
- update後のbuild/testが成功する

### 最初のprobeの結果

- [x] immutableなcommit、target tree、source blob、request blob、43 Factsをbaselineとして固定した。
- [x] `User.nickname`追加とpage size 20→10をincremental requestへ導出した。
- [x] 8 intent nodesをadded/changed、13 Factsをchanged、30 Factsをunchangedとして分類した。
- [x] full regenerationせず、既存12 testsとpackage構造を保ったtarget差分として実装した。
- [x] 43/43 Facts、2 satisfied policies、1 preferred deviation、root/targetの`go test`と`go vet`を確認した。
- [x] `v0alpha1` baselineを保持し、additiveでない新protocolを`v0alpha2`として分離した。
- [x] verify時にbaseline requestを必須入力とし、canonical digestとsemantic diffを再導出して照合した。
- [x] preferred deviationをreason付きでCLIへ表示し、人間のreviewから隠さないようにした。

最初のprobeについて上のExit criteriaは満たした。Milestone全体には後続のrename、削除、constraint変更、
migration probeが残るが、これらをIdentityより先のblockerにはしない。

## Milestone 4 — Public signup/signin + Identity（P1 / Stage D completed）

### 目的

管理画面CRUDでは現れなかったcurrent principal、credential、session、ownershipの意味を、実装方式から
独立して記述できるか検証する。

### 最初のflow

最初のvertical sliceは
[`email-verified-membership-probe.md`](email-verified-membership-probe.md)へ固定する。
Stage Bのtarget-neutral shapeは
[`identity-semantic-model-proposal.md`](identity-semantic-model-proposal.md)で設計する。

1. visitorがname、email、passwordでsignupし、Pending Userになる。
2. 30分・一度限りのメールURLからverificationし、Activeになる。
3. verificationを再送でき、古いevidenceは無効になる。
4. Active Userだけがsigninし、signoutできる。
5. 未認証principalは保護されたpageを閲覧できない。
6. authenticated userは自分のprofileだけを表示・編集できる。

### 設計・検証する

- domain entityとcurrent principalの関係
- signup、signin、signoutのobservable intent
- ownership predicateとrole permissionの合成
- credentialを通常のentity fieldやdiagnosticへ漏らさない境界
- session方式、hash、cookie、identity providerをagent/repositoryへ委ねる境界
- Identity Intentと正常系・拒否系Acceptance Facts
- verification emailのemissionとdeliveryの分離
- verification expiryを検査するclock boundary
- Credentialを`preserveInput`と`stored: "input"`から除外するfact導出規則
- compiler invariant（secret boundaryとsetup pre/post）、Acceptance Fact、人間review requirement
  （secret handlingとrepository fixture fidelity）の分離
- credential/evidence値をRequestへ入れず、symbolic handleを使う独立したsemantic fixture

候補syntaxを先に固定せず、[`public-membership-proposal.md`](public-membership-proposal.md)の比較例を
Generation Requestへ落とせる最小semantic modelから決める。

Stage Aでは現行v0 subsetを[`../examples/public-membership.forma`](../examples/public-membership.forma)として
実測済みである。公開create form、Pending初期state、email constraintから11 Factsを導出できた。一方、
Credential、Verification、Effect、resend、self authorizationは表現できず、password fieldをsecretとして
区別できないことも確認した。現行Fact規則ではpasswordの再表示と平文相当の保存まで要求してしまうため、
credential-awareな導出が必要である。また、signinやownershipのFactをsignup Factの成功結果へ依存させず、
symbolic handleとsemantic setupから独立実行する方針を固定した。

Stage Dでは、適用済みadmin request（Git blob `5751ecf8`、`v0alpha2`）をbaselineに、実際の`.forma`
sourceからincremental Generation Requestを構築し、[`../experiments/membership-agent-e2e`](../experiments/membership-agent-e2e/README.md)
のtarget repositoryへメール認証付きsignup/signinを実装した。81 Acceptance Facts（admin 43件はunchanged、
38件がすべてadded）と3 Implementation Policies、3 Review Requirementsを`forma verify --baseline`で検証した。
Forma coreはGo/HTTP/HTML用のgeneratorやadapterを持たない。

同じsourceが意味的に十分でも、複数pageへ分散したnavigationと例外結果を人間が読みやすいとは限らない。
現行source、現行sourceからの決定的projection、`flow`をnavigationの正本にする案を
[`membership-flow-notation-probe.md`](membership-flow-notation-probe.md)で比較した。read-only navigation projectionを
`forma project navigation`、case-oriented outcome projectionを`forma project outcomes`として実装した。Identityとadmin
CRUDを同じmodelへ投影し、outcome側はAcceptance Factに明示されたnegative guaranteeだけを`must not`へ分離する。
language syntaxとResolved Intent schemaは増やさず、未宣言のdefault entryを`unspecified`と表示する。一方、現行shapeでは
任意のsurface-only chainを表せないsemantic gapは残る。

この実測で4つの穴が出た。既存applicationへIdentityを足すと標準actionの宛先が曖昧になり、明示`goto`
（[`navigation-destination-proposal.md`](navigation-destination-proposal.md)）で解決した。Factが2件、
到達不能な状態をsetupしていた。`verification-rejected`のconsumed caseは`Pending + consumed`を、
`duplicate-identifier-rejected`は「evidenceを持たない既存registration」を要求していたが、どちらの状態も
atomicなoperationからは生まれない。前者を成功verificationの到達先である`Active`へ、後者をregistrationが
commitする4 recordを揃えた状態へ直し、期待値を絶対数からgrowthへ変え、双方をcompiler invariantで固定した
（Acceptance Facts `v0alpha4`）。targetがemail検証へFormaにない規則を足していたため、宣言された`matches`を
そのまま適用する形へ統一した。

fact→test mapの限界も測れた。`forma verify`はreferenceが解決できるかしか見ないため、名前が対応していても
期待値を観測していないtestを通してしまう。review時に9件が該当し、protocol上の81/81と意味上の81/81が
別物であることが確認された。埋めるにはhuman reviewが要る。

fixtureがFactを観測できているかの確認手段はmutation testであることも分かった。`fixture-fidelity`は
2度差し戻され、2度目は「countが変わらない」という観測がsubmit値と既存値が同じなら上書き実装も通す、
という理由だった。testがpassすることではなく、実装を壊したときに落ちることでしか確かめられない。
feedback artifactの信頼性も同様で、生成失敗時に旧artifactが残ると`forma verify`が古い測定で成功して
しまうため、撤回してから検証し成功時のみatomicに公開する形にした。3件のReview Requirementsは
すべて承認され、Stage Dは完了である。

独立agentによる初回生成の再現性、実在する大規模repositoryへの適用、rename/削除を含むincremental
updateは未検証である。後続のautomated repairは
[`../experiments/membership-automated-repair-loop`](../experiments/membership-automated-repair-loop/README.md)で
fresh agent processを含むbounded orchestrationまで実測した。

Stage B1ではForma syntaxを増やさず、test-only fixtureからIdentity、Identifier、Credential、Registration、
Verification、Authentication、Session、Ownership、page interaction、authenticated/ownership accessを
Resolved Intent `v0.5`へ正規化した。全semantic nodeのstable IDとSource Map `v0.3`の1対1 coverage、closedな
参照検査を実装し、既存admin semanticsも維持した。

Stage B2ではcanonical fixtureからIdentity専用29 Acceptance Factsを完全導出した。Fact-localなsemantic setupは
runtime値を持たないsubject/credential/evidence/session handleで表し、複合例外はclosed caseとして隔離する。
canonical fixtureの27 distinct kindと`FactKindContract` registryの完全一致をfixture testで固定し、汎用validatorは
生成されたkindがcontractを持つことを検査する。operation実行とbefore/after observation、setupが期待結果を
先取りしないpre/post規則はcompiler invariantである。Credentialは`preserveInput`と`stored: input`から除外し、
credential/evidence raw value用schema fieldもstructural testで禁止した。

Stage B3では`secret-redaction`、`secret-storage`、`fixture-fidelity`をstable ID付きReview Requirementsとして
Resolved Intentから決定的に導出する。Generation Request `v0alpha3`はこのartifactと表示対象IDを持ち、validatorが
再導出して完全一致を要求する。これらはFact coverageやfeedbackの`passed`件数へ含めず、`forma verify`が機械検査の
成功後も必ず人間へ表示する。

Stage B4ではGeneration Requestを`v0alpha4`へ上げ、Review Requirement diffとbaselineのSource Map / Review
Requirements versionを追加した。historical `v0alpha1` / `v0alpha2`は専用codecで元のcanonical bytesを保ち、
compiler outputだけをlosslessにupgradeしてdiffする。適用済みadmin `v0alpha2` blobからadmin semanticsを保った
Identity追加requestへ、43既存Factsをunchanged、38 Factsと3 Review Requirementsをaddedとしてpairwise lineageを
検証した。

Stage B5ではpasswordless、external provider、email変更をsyntaxなしのGo fixtureとして比較した。generic Fact validatorは
passwordlessのcredential非依存26/38 Factsを受理し、Fact kindの部分集合対応を実証した。一方、end-to-end builderは
3件を明示的に拒否し、authentication proof、external authority、identifier binding lifecycleが独立した不足axisだと
分かった。詳細は[`identity-variant-probe.md`](identity-variant-probe.md)に記録する。次は対応済みfirst sliceだけを
一意に表す最小surface syntaxであり、未対応方式を通常field/actionへfallbackさせない。

Stage Cでは[`identity-surface-syntax-proposal.md`](identity-surface-syntax-proposal.md)の最小syntaxを実装する。
`proof`をcredentialの別名にせず、local password proofとそのcredential bindingを別semantic nodeとして解決する。
合格条件はsourceからStage B fixtureと同じIdentity semantics、38 Facts、3 Review Requirementsを再導出できることである。

このfirst sliceは実装済みである。`examples/email-verified-membership.forma`をParser / Checkerが受理し、
Authentication Proofとcredentialを別nodeにしたResolved Intent `v0.7`、Source Map `v0.4`へ解決する。canonical
fixtureとの完全一致、38 Identity Facts、3 Review Requirements、未対応proof / lifecycle / owner bindingのnegative testを
固定した。各Identity operationのinteractionもapplication全体でちょうど1件に制限し、Factの付かない追加surfaceを
拒否する。Stage DではこのGeneration Requestを既存admin targetへ適用し、既存43件とIdentity 38件を合わせた
81/81 Facts、3 Review Requirementsを検証した。詳細は
[`../experiments/membership-agent-e2e`](../experiments/membership-agent-e2e/README.md)に記録する。

### Exit criteria

- target repositoryを知らなくてもflowと拒否条件を説明できる
- 本人だけの操作をrole定数へ縮退させず検査できる
- credential valueがResolved Intent、Source Map、diagnosticへ漏れない
- credential/evidenceの値をGeneration Requestへ入れず、各Identity Factを独立実行できる
- agentがrepository標準の安全なidentity実装を選べる
- Identity Factsをtarget固有testへ変換し、正常系と否定系が成功する

## Milestone 5 — Automated repair loop（P2 / first bounded loop completed）

### 目的

一度の生成成功ではなく、build/test failureを観測し、Formaの意味を弱めずに実装を修正するloopを完成する。

```text
Generation Request
  → inspect repository
  → edit
  → build / test / lint
  → structured feedback
  → repair
  → repeat until success or genuine blocker
```

### 実装済みの土台

- stage、status、command、diagnostic、関連intent node、fact/policy coverageを持つ`v0alpha2` feedback型
- required fact IDとcoverageの完全一致、未知・重複・未参照を拒否するvalidator
- immutable requestとfeedback JSONを検査する`forma verify`
- fact ID、repository固有test reference、resultを持つcoverage report

### 残る実装・実験

- [x] compiler errorとrepository build/test failureの明確な分離
- [x] agentが要求を削除・弱化してtestを通すことを禁止するretry policy
- [x] failureが実装bugか、Formaのintent gapかを分類するfeedbackとhuman handoff
- [x] fresh agent processでbuild失敗とtest失敗のrepairを別々に再現
- [ ] 複数attemptを必要とするrepairと、より間接的なfailureでの再現

Source Mapを使ったfailure関連付けと、controlledなfailure → repair → successの最初のprobeは
[`../experiments/membership-repair-loop`](../experiments/membership-repair-loop/README.md)で実測した。

### 最初のprobeの結果

- [x] duplicate registrationが既存credentialを上書きするcontrolled faultをtargetへ入れ、
      `TestDuplicateIdentifierCoversExactAndCanonicalForms`の失敗を観測した。
- [x] 失敗testからFact `fact/identity/UserAccount/operation/register/identifier/duplicate` を求め、
      sourceNodesとSource Mapから5件のrelatedIntentNodesへ結んだ。
- [x] 旧succeeded feedbackを残さず、`status: failed`のGeneration Feedbackをatomicに公開した。
      `forma verify`は成功しなかった。
- [x] app.forma、Generation Request、Manifest、coverage map、既存testのhashはrepair前後で不変。
      成功時feedbackはStage D記録とbyte-identicalに戻った。
- [x] 実装だけを直し、`forma verify --baseline`が81/81、40 distinct tests、3 policies、
      3 Review Requirementsで成功した。

このprobeは同じagentによるcontrolled runである。完全な自動orchestrationや一般的なrepair能力の証明ではない。

### build failureのprobeの結果

test failureに対する上のprobeと対にして、compile failureを
[`../experiments/membership-build-repair-loop`](../experiments/membership-build-repair-loop/README.md)で実測した。

- [x] `signIn`のcredential照合をarity違いにするcontrolled compile errorを1行入れ、
      `internal/web`と`cmd/server`のbuild失敗を観測した。
- [x] `stage: build` / `status: failed`を出し、81 Factsすべてを`not-run`に保った。
      失敗したassertionがないので`failed` Factも`relatedIntentNodes`も作らない。
- [x] 最初の実行でdiagnosticsからGo compiler errorが落ちることを発見した。`go test -json`は
      compiler errorを`ImportPath`付きの`build-output` recordで出すが、generatorのparserが
      package eventしか見ておらず`[build failed]`だけが残っていた。schemaではなく
      experiment側parserの欠落であり、`v0alpha2`のfieldのまま修正した。
- [x] failed feedbackから`policyCoverage`を落とした。`ValidateCompletion`は`succeeded`でない
      feedbackのpolicy coverageを検証しないため、そこへ書いた`satisfied`は誰も検査しない主張になる。
      `factCoverage`の`not-run`に当たる語がpolicy側にないので、schemaを増やさず何も主張しない方を選んだ。
      feedback generatorは共有なのでtest failure側にも効き、上のprobeの
      `generation-feedback.failed.json`も再生成した。stage `test`の実測結果は変わっていない。
- [x] compiler diagnosticだけを根拠に実装1行を戻し、`forma verify --baseline`が81/81へ復帰した。
      成功時`generation-feedback.json`のhashは上のprobeの記録と同一へ戻った。
- [x] build failureをtest failureと誤分類しない、未観測Factを`passed` / `failed`にしない、
      compiler diagnosticを空にしない、をnegative testで固定した。

81 Factsすべてが少なくとも1件のtest referenceを`internal/web`へ持つため、
web packageのcompile errorはcoverage全体を盲目にする。`passed`は0件だった。

### retry integrityのprobeの結果

repairが実装を直さず、testやcoverageやrequestを弱めてgreenにする経路を拒否できるかを
[`../experiments/membership-repair-integrity`](../experiments/membership-repair-integrity/README.md)で実測した。

- [x] faultを残したままreferenced testを弱める / 失敗testを削除してcoverage mapを付け替えると、
      現在のpipelineは`succeeded`を出し`forma verify`が81/81で成功することを確認した。
      duplicate registrationのbugはtargetに残ったままである。
- [x] retry開始前にtrusted側が固定したsnapshotとの比較で、3経路すべてを拒否した。
      比較元はrepositoryの外に置き、agentが返したhashは読まない。
- [x] gateはtest commandより前に走る。go testがgreenでも止まる。
- [x] 保護対象のtest fileはcoverage mapのtest referenceから決定的に導出する。
      target implementationは固定しないので、implementation-only repairは通り81/81へ到達した。
- [x] 拒否時は`status: blocked` / `stage: inspect`をpublishし、fact coverageもpolicy coverageも
      主張しない。schemaは`v0alpha2`のままで足りた。撤回とblocked publishはagent-editableな
      generatorではなくprebuilt guardが所有するので、guardで止まった時点が安全であり、
      拒否後に古いsucceeded feedbackは残らない。guardが非0で終わったらgeneratorを起動しない。

- [x] reviewで2つの迂回路が見つかり、塞いだ。1つはrule packageへのfile追加で、記録済みpathを
      1 byteも変えずにcoverage mapを書き換えられた。snapshotへdirectory一覧を記録し、追加を
      `added`として拒否するようにした。もう1つは、追加fileの`init()`がgateより先に走り、
      gateが読む前にretry baseline自体を貼り直す経路である。これはgateをagentの編集するcodeと
      同じprocessで動かす限り塞げないので、integrity packageだけをimportする
      `cmd/retryguard`をprebuilt binaryとして分離した。

このgateが成立する条件は3つあり、いずれもtrusted orchestratorの責務である。snapshotを
repositoryの外へ置くこと、gateを起動すること、そしてgateをagentの編集するcodeと
process分離すること。3つ目を破ると、bypassが出すsucceeded feedbackは正当なartifactと
byte-identicalになり、artifactからは区別できない。testが意味としてFactを忠実に検査して
いるかも、この機構は証明しない。

### automated independent loopの結果

上の責務を[`../experiments/membership-automated-repair-loop`](../experiments/membership-automated-repair-loop/README.md)
で1つのexperimental orchestratorへまとめた。

- [x] retry開始前に`retryguard`、feedback generator、`forma verify`をrepository外へprebuildした。
      guardだけを固定しても、その後agent-editableなgeneratorやverifierを`go run`すれば同じprocess
      bypassが戻るため、retry後は3 toolsのsourceをcompileしない。
- [x] 初回測定、fresh repair process、guard、再測定、最終verifyをbounded loopとして固定した。
      guardがblockedをpublishした場合はgeneratorを起動せず、attempt上限またはagent非0終了では
      最新のfailed feedbackを人間へ残す。
- [x] 別々の`codex exec --ephemeral` processへtest failureとbuild failureを渡し、どちらも
      implementationだけを1 attemptで修正した。trusted側の最終測定は81/81、40 distinct tests、
      3 policies、3 Review Requirementsで成功し、feedbackは元のartifactへbyte-identicalに戻った。
- [x] repair agentは最終testとverifierを実行せず、合否はagent process終了後にtrusted側が決めた。
- [x] ASCII case foldだけを宣言したimmutable requestに対し、protected testがUnicode case foldを要求する
      controlled intent gapを追加した。fresh agentはcodeやverification inputを変更せずstructured decisionを返した。
- [x] decisionのFact IDとintent nodeをfailed feedback、Generation Request、Source Mapへ照合し、repository全体が
      不変であることとtrusted再測定が同じfailureを返すことを確認してから、観測済みの`stage: test`とcommandを
      保った`status: blocked`のGeneration Feedbackをhuman handoffとして発行した。再測定が成功するdecisionや
      変更を残すdecision、rejected Factがないbuild failureからのdecisionは拒否する。

これはagent/provider APIをForma coreへ追加するものではない。orchestratorはprocess commandだけを
知るexperiment toolingである。2 repairsと1 intent gapは局所的で、実agent runはいずれも1 attemptだった。
同一OS userに対するfilesystem/process isolationも提供しない。最初のbounded loopはExit criteriaを満たしたが、
複数attempt、間接的なfailure、一般的なintent-gap分類は追加の再現性課題として残る。

### Exit criteria

- [x] buildまたはtest failureから自動修正して成功できる
- [x] retry中もrequested factsとpolicyが欠落・弱化しない
- [x] failureがFormaの不足ならcodeで回避せず、人間へintent gapとして返せる
- [x] 合否をagentの自己申告ではなくrepository commandの結果で確認できる

## Milestone 6 — Application semanticsを実例から拡張する（P3）

### 目的

CRUDとstate transitionを越えるdomain behaviorを、statement languageやframework vocabularyへ退行せず
記述する。

### 検討順序

```text
1. pure Expression
2. Derived Value / Invariant / Precondition
3. Changesとatomic post-state
4. Occurrence model
5. Effect binding / delivery contract
```

Effectを先に設計しない。recipientや発生条件にもExpressionが必要であり、ChangesとEffectの境界には
atomic post-stateの意味が必要だからである。

### 現在のprobe

- [`expression-proposal.md`](expression-proposal.md): self-only Invariantの`<=`を最小sliceとして実装
- [`order-approval-proposal.md`](order-approval-proposal.md): 注文承認、在庫引当、通知再送、閾値割れ
- [`examples/orders.forma`](../examples/orders.forma): v0だけで書ける構造・lifecycle・認可を確認

### 選別基準

- 2つ以上の異なるapplication例で同じsemantic needが現れる
- 独立したstable identity、評価時点、failure semanticsを持つ
- 他のprimitiveのmodifierだけでは表せない
- frameworkやdelivery mechanismから独立してAcceptance Factsを導出できる
- control flow、statement順序、暗黙の副作用を持ち込まない

### Exit criteria

- order/inventoryと少なくとももう1例で共通axisを確認する
- actionを実行できる条件とpost-stateをtarget-neutralに表現できる
- 状態を変えないactionとobservable occurrenceを表現できる
- effectのemissionとdeliveryを分離し、stable emission identityを導出できる
- 各追加概念から正常系・拒否系Acceptance Factsを作れる

## Milestone 7 — v0 hardening and release（P4）

### 目的

実験で残った最小言語とfront-end boundaryを、第三者が再現・利用できるreleaseへする。

### 実装する

- normative v0とreference front-endの差分を閉じる
- schema/version compatibilityとmigration policyを固定する
- `forma fmt`、`forma explain`、diagnostic UXを完成する
- compilation unitとproject layoutを明文化する
- installation、editor integration、CI例を用意する
- examplesとgolden/negative corpusを増やす
- security、secret handling、untrusted repository実行境界をreviewする

### Exit criteria

- clean environmentからinstallし、examplesをcheck/resolve/request/verifyできる
- normative specificationと実装の既知の差分がない
- version不一致やunsupported intentを無言で無視しない
- 第三者が少なくとも1つのrepositoryでE2Eを再現できる

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

- 中心仮説を強く否定し得るend-to-end experimentを、広いschema設計より先に行う。
- Forma sourceの可読性を生成速度より優先する。
- AIは実装主体だが、parse、名前解決、型、Forma semanticsは所有しない。
- Resolved Intentは実装shapeではなくapplication intentを運ぶ。
- Acceptance Factsの期待値はFormaが決め、target固有testへの変換はagentが行う。
- repositoryを通常のsourceとして尊重し、既存codeへincrementalに変更する。
- build/test failureは実装修正へ使い、intentを暗黙に弱めない。
- 足りない意味はframework profileではなく、必要ならForma languageへ戻して検討する。
- 実例が必要性を示すまで、新しいprimitiveやmanifest schemaを固定しない。
