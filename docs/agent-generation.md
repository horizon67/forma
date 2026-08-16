# Agent Generation Model

Status: architectural direction — minimal admin-flow request schema implemented as `v0alpha1`

Formaのend-to-end実行モデルでは、AI coding agentは任意のgenerator implementationではなく、
application codeを作る主体である。

```text
Forma source
  → parse / check / resolve
  → Resolved Intent + Acceptance Facts
  → Generation Request
  → AI coding agent + target repository
  → ordinary application code
  → repository build / test
  → feedback to the agent
```

Formaはcoding agentへ渡す自然言語promptを、型付き・検査可能・review可能な入力へ置き換える。
Forma compilerはapplicationの意味を確定するが、その意味を特定frameworkのfileやAPIへloweringしない。

## 責務境界

設計上の責任主体はGoそのものではなく、決定的なForma compilerである。Goは現在のreference
implementationであり、責務境界を次のように定める。

```text
Forma source
  ↓
Forma compiler
（現在のreference implementationはGo）
  - format
  - parse
  - 名前解決
  - 型検査
  - 状態遷移・認可・制約の意味検査
  - 暗黙事項の解決
  ↓
Resolved Intent + Source Map + Acceptance Facts
  ↓
Generation Request
  ↓
AI coding agent
  - target repositoryを読む
  - frameworkと既存architectureに合わせて実装する
  - Acceptance Factsをrepository固有testへ変換する
  - build / test / lintを実行する
  - failureを受けて実装を修正する
  ↓
通常のapplication code
```

Forma compilerは「何を実装し、何が成立すべきか」を決定する。AI coding agentは「対象repositoryで
どう実装し、どう検査するか」を決定する。

Forma toolchainが所有するもの:

- grammar、parser、formatter
- 名前解決、型検査、静的な意味検査
- source上の省略を展開したtarget-neutralなResolved Intent
- Resolved IntentのnodeとForma sourceを結ぶSource Map
- 実装後に観測すべきtarget-neutralなAcceptance Facts
- coding agentへ渡すmachine-readableなGeneration Request
- build/test failureをintent nodeへ対応付け、agentへ返すfeedback envelope

AI coding agentとtarget repositoryが所有するもの:

- component、route、API、database schema、transactionの具体形
- framework、library、file layout、naming convention
- repositoryの既存architectureへ合わせたintegration
- migrationを含むincrementalなcode変更
- Acceptance Factsをrepository固有のtestへ変換すること
- build、test、lintなど既存toolchainの実行と、failureに基づく修正

Forma coreはframework別generator、capability matrix、共通runtime adapter、target別test harnessを
保有しない。coding agentが実装できない場合は、profile compatibility errorを捏造するのではなく、
repositoryの事実と失敗したcommandをfeedbackとして返す。

## Resolved Intent

Resolved Intentはcompiler内部のlowering用IRではなく、**coding agentが実装すべきapplication intentを
曖昧さなく列挙した出力**である。少なくとも次を含む。

- 解決済みのentity、field、relation、type、constraint
- stateと許可されたtransition
- action、precondition、permission
- page、list、detail、formと、そこに含まれるcapability
- 省略から一意に導出されたprojection、action参照、navigation
- 各nodeのstable semantic identity

含めてよいのは、利用者またはdomainから観測できるapplicationの性質と、sourceから一意に解決された
参照・defaultである。React component、HTTP verb、SQL、directory、package名に加え、loading widget、
relation用dropdown、identityによるsort tie-break、submission tokenのような実現機構は含めない。
既存repositoryがすでにそれらを選んでいる場合、agentはrepositoryから読み取る。

全mutationに共通するauthoritative validationや認可再検査のようなlanguage-level guaranteeは、対策名を
booleanとして各nodeへ繰り返さない。対象nodeを参照するAcceptance Factとして出力する。

## Acceptance Facts

Acceptance Factsはtarget固有test codeでも散文promptでもなく、実装後に成立すべき事実を表す
machine-readableなnodeである。以下は人間向け表示であり、交換形式そのものではない。

```text
- action/User/activate succeeds only from Confirmed
- page/Users is visible to admin
- page/Users/view/list/User searches name and email
- page/Users/view/list/User has a logical page size of 20
- an invalid User transition is rejected without changing the state
```

正常系と否定系の両方を持たせる。agentはこれらを、対象repositoryで通常使われているunit、integration、
request、browser testなどへ変換する。Forma自身はHTTP statusやDOM selectorを標準化しない。

Acceptance Factsは実装の合否をAIの自由判断へ委ねるためのpromptではない。期待する意味はForma
front-endが決定し、agentが決めるのはそのrepositoryでどう観測・検査するかである。

各factは少なくとも次を持つ。管理画面list/detail/edit向けの最小kindは
`forma/acceptance-facts/v0alpha1`として実装し、追加domainのkindは実例から拡張する。

```text
AcceptanceFact
  id               stable fact identity
  kind             target-neutral assertion kind
  subject          Resolved IntentのSemanticID
  input            principal、値、pre-stateなど
  expected         outcome、post-state、visible capabilityなど
  sourceNodes      根拠となるResolved Intent node
```

fact IDはsource位置やtarget repositoryに依存せず、subjectのSemanticID、fact kind、caseから決定的に
導出する。HTTP、DOM、framework、test runnerの語彙を`kind`や`expected`へ含めない。

## Generation Request

最小のmachine-readable requestは次の情報を運ぶ。

```text
GenerationRequest
  schemaVersion
  resolvedIntent
  acceptanceFacts
  sourceMap
  requestedChange
  verification
    feedbackSchema
    requiredFactIds
    requireTestReference
    rejectUnknownFacts
```

`requestedChange`は初回生成ならapplication全体、更新なら前回から変化したintent nodeを表す。
target repositoryそのものはrequestへ複製せず、agentへworkspaceとして渡す。architecture constraintsや
禁止事項が必要なら、repositoryのpolicy fileまたは明示的なuser instructionとして同時に渡す。これらを
genericかつ検証可能なentryとして構造化する案は
[`implementation-policy-manifest-proposal.md`](implementation-policy-manifest-proposal.md)に記録する。

model名、prompt template、tool listをForma language semanticsへ含めない。それらはagent executionの
設定であり、Generation Requestの意味ではない。

Generation RequestはForma orchestration layerがcompilerから受け取り、agent実行中もimmutableな入力として
保持する。完了判定にagentから返されたrequestのcopyを使用してはならない。`requiredFactIds`はagentが作業対象を
確認するための複製であり、coverage検証時の正本ではない。検証側は保持しているResolved IntentからAcceptance
Factsを再導出し、request内のfactsおよび`requiredFactIds`と一致することを先に確認する。

## Feedback loop

agentは最低でも次をForma orchestration layerへ返せる必要がある。

```text
GenerationFeedback
  stage: inspect | edit | build | test
  status: succeeded | failed | blocked
  relatedIntentNodes
  factCoverage
    factId
    testReferences
    result: passed | failed | not-run
  command
  diagnostics
  summary
```

Generation Requestは、各factをrepository固有testへ変換し、そのtestまたはsidecar manifestからfact IDを
参照できるようagentへ要求する。`succeeded`と判定する前にForma orchestration layerは、保持したrequestの
Resolved Intentから再導出したcanonical fact ID集合と`factCoverage`の集合が完全に一致し、未知・重複・
未参照factがなく、すべて`passed`であることを機械的に照合する。この照合はtarget固有adapterではなく
ID集合とtest結果の検査である。

`testReferences`の最小交換形式は`repository/relative/path#test-identifier`とする。空文字、絶対path、正規化
されていないpath、同一fact内の重複参照は拒否する。1つのintegrationまたはE2E testが複数factを同時に検査する
ことは正当なので、異なるfact間で同じtest referenceを共有してよい。

現在のreference CLIでは、orchestration layerが保持したrequestとagentのfeedbackを次のように照合する。

```bash
forma verify request.json generation-feedback.json
```

このcommandはJSON schemaの未知field、request内のfacts/policy改竄、coverage集合の不一致、不正なtest
reference、未成功resultを拒否する。またdistinct test数と1 testあたりの最大fact数を表示し、coverageの
集中を可視化する。repositoryのtest command自体はagent execution側が実行する。

`AcceptanceFacts.version`はJSON shapeだけでなく、Resolved Intentからfactを導出する規則のversionでもある。
fact kind、ID、input、expected、導出対象を変える場合はこのversionを更新する。同様にResolved Intentと
Source Mapも各versionへ対応する。現在のverifierがrequestのversionをsupportしない場合、canonical比較を
実行せず、matching Forma versionで検証するよう明示的に拒否する。将来過去versionをsupportする場合は、
versionに対応するbuilderへdispatchする。

この機構はfactの変換漏れを防ぐが、test内容がfactを忠実に検査していることまで証明しない。その確認には
repositoryのreviewと、将来必要ならtest mutationなど別の検証を使う。

Forma sourceのerrorとrepository実装のfailureを混同しない。前者はcompiler diagnostic、後者はagentの
build/test feedbackである。repository failureからFormaの意味が不足していると判明した場合だけ、
新しいlanguage axisまたはsource変更として人間へ返す。

## Incremental update

target codeは破棄専用artifactではなく、通常のapplication repositoryである。人間やagentが保守してよい。
ただしFormaが所有するapplication intentをtarget codeだけで変更するとdriftするため、意味の変更は
Forma sourceへ反映し、Resolved Intentの差分を次のGeneration Requestとしてagentへ渡す。

目標はbyte-identicalな再生成ではない。目標は、既存の設計と手書きcodeを尊重しながら、intent差分を
小さく安全なrepository変更として適用し、build/testを通すことである。

## 現在のprototypeとの関係

[`../experiments/admin-e2e`](../experiments/admin-e2e/README.md)と
[`../experiments/conformance`](../experiments/conformance/README.md)は、Forma sourceからどの意味を
取り出す必要があるかを調べるための過去のprototypeとして凍結する。Go generator、profile capability、
共通adapter、byte-identical artifactをForma coreの将来architectureとはしない。
