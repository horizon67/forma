# Agent Generation Model

Status: architectural direction — historical lineage and current Generation Request implemented as `v0alpha4`

Formaのend-to-end実行モデルでは、AI coding agentは任意のgenerator implementationではなく、
application codeを作る主体である。

```text
Forma source
  → parse / check / resolve
  → Resolved Intent + Acceptance Facts + Review Requirements
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

この境界は[`order-invariant-agent-e2e`](../experiments/order-invariant-agent-e2e/)でも維持された。Formaは
278 Acceptance Factsと4 Review Requirementsを持つGeneration Requestを生成し、coding agentはFormaをimportしない
standard-library Go application、保存境界、HTTP surface、52 repository testsを実装した。experiment側の測定processが
実際の`go test -json`結果と明示的coverage mapを照合し、278/278 Factsをfeedbackとして返す。application runtimeで
experiment commandやForma verifierが動く構成ではない。

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

各factは少なくとも次を持つ。現在のkindは
`forma/acceptance-facts/v0alpha8`として実装し、admin flow、Identity専用29 Facts、application entry、
surface-only transition、self-only Invariant、domain action transition/confirmation、experimental Changesを扱う。Invariantはentity単位の成立・違反に加え、
参照fieldを入力に含むform submitごとにauthoritativeな拒否Factを導出する。各Factは他のrequirementを満たした
隔離scenarioとして、解決済みExpression tree、post-state評価、authoritative enforcement、atomic commit結果を
持つ。追加domainのkindは引き続き実例から拡張する。

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

認証済みprincipalや一度限りのevidenceのようにpreconditionの確立が必要なFactでも、別Factの成功結果を
`dependsOn`として参照しない。各Factは新しい隔離されたscenarioから独立実行できるものとし、必要な初期状態は
target-neutralなsemantic setupとして表す。credentialやevidenceは値そのものではなくsymbolic handleで参照し、
具体的な合成test値と確立方法はcoding agentがrepository固有testへ落とす。Forma coreはframework別fixture
adapterを持たない。compilerが導出したclosed setupはFact kindごとのpre/post contractで検査し、expectationを
setup時点で成立させるself-fulfillingな組合せを拒否する。そのsetupをrepository固有testへ変換した実装が
operation・認可・観測経路を迂回していないかはFormaが再計算できないため、Factの`passed`結果へ吸収せず、
人間が確認するstable review requirementとして扱う。
Identityでの具体的なcandidate shapeは
[`identity-semantic-model-proposal.md`](identity-semantic-model-proposal.md)に記録する。

現在のcompilerが`fixture-fidelity`を導出するsubjectはIdentityだけである。
[`order-invariant-agent-e2e`](../experiments/order-invariant-agent-e2e/)では、Identityを持たないsurface access Factsを
pureなrole関数へだけ対応付けるとHTTP認可を迂回できることが分かったため、当時の98 access Factsすべてを実HTTP handler testへ
対応付けた。続くChanges sliceでもこの条件をaccess kindに限定せず、`page/...`をsubjectに持つcurrent 243 Factsすべてが
実HTTP testを参照する回帰条件へ広げた。一般のapplication／surface単位Review Requirementは必要性が確認できたが、
Resolved Intentにapplication root nodeがなく、review完了の範囲もまだ定義していない。既存Identity requirementへ暗黙に
混ぜず、target-neutralなsubjectとsourceNodesを設計してからcompiler-owned requirementへ一般化する。

人間確認が必要な境界は`forma/review-requirements/v0alpha4`としてAcceptance Factsから分離する。Identityごとに
`secret-redaction`、`secret-storage`、`fixture-fidelity`の3件を、Invariantごとに
`concurrent-invariant-enforcement`をstable ID付きで導出する。後者は、同じpredicateを参照するauthoritativeな
mutation境界がconcurrent operationでも違反post-stateをcommitしないことを確認させる。Changes actionごとに
`atomic-changes-enforcement`と、cross-entity writeがある場合の`cross-entity-write-authorization`、relation value readがある場合の
`cross-entity-value-read-authorization`も導出する。instructionはcompiler所有の
固定文であり、agent feedbackへreview coverageや完了statusを追加しない。`forma verify`のexit 0は機械検査の成功だけを
意味し、これらの要件は成功出力にも必ず表示される。

## Generation Request

最小のmachine-readable requestは次の情報を運ぶ。

```text
GenerationRequest
  schemaVersion
  resolvedIntent
  acceptanceFacts
  reviewRequirements
  sourceMap
  implementationPolicy
  requestedChange
    kind: full | incremental
    baseline request identity
    intentChanges
    factChanges
  verification
    feedbackSchema
    requiredFactIds
    displayReviewRequirementIds
    requireTestReference
    rejectUnknownFacts
```

`requestedChange`は初回生成ならapplication全体、更新なら前回から変化したintent nodeとAcceptance Factを
表す。incremental requestはimmutableなprevious requestのcanonical SHA-256、schema version、added/changed
node、unchanged件数を持つ。最初のsliceではremoved nodeを拒否し、rename/delete modelを推測しない。

target repositoryそのものはrequestへ複製せず、agentへworkspaceとして渡す。architecture constraintや
禁止事項は、正規化した`implementationPolicy`としてapplication intentと分離して格納する。最小の
`required`、`preferred`、`forbidden`とcoverage規則は
[`implementation-policy-manifest-proposal.md`](implementation-policy-manifest-proposal.md)に記録する。
incremental requestで新しいManifestを指定しない場合はbaseline requestに埋め込まれたManifestを保持し、
policyを意図せず消さない。

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
  policyCoverage
    policyId
    status: satisfied | deviated | flagged
    evidence | reason | hits
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
forma verify --repository target/ --baseline previous-request.json incremental-request.json generation-feedback.json
```

このcommandはJSON schemaの未知field、request内のfacts/policy改竄、coverage集合の不一致、不正なtest
reference、未成功resultを拒否する。またdistinct test数と1 testあたりの最大fact数を表示し、coverageの
集中を可視化する。Implementation Policyを持つrequestではrepository rootを受け取り、required evidence、
preferred deviation reason、forbidden token scanも検査する。repositoryのtest command自体はagent execution
側が実行する。
forbidden scanにhitがある場合は機械的failureにはしないが、`flagged` policy IDとhit pathをCLIへ表示し、
人間のreviewへ残す。

incremental requestのverifyでは直前のrequestを`--baseline`で必須入力とする。verifierはbaselineをcanonical
JSONへmarshalしたbyte列のSHA-256を照合し、Resolved IntentとAcceptance Factsのdiffも再導出して、requestに
記録されたadded/changed/unchanged集合と一致することを確認する。fileそのもののSHA-256ではないため、末尾改行や
JSONの空白だけを変えてもidentityは変わらない。この検査が保証するのは隣接するrequest間のpairwiseな
lineageであり、repositoryへどのrequestが最後に適用されたかの証明はorchestration layerが所有する。

`AcceptanceFacts.version`はJSON shapeだけでなく、Resolved Intentからfactを導出する規則のversionでもある。
fact kind、ID、input、expected、導出対象を変える場合はこのversionを更新する。同様にResolved Intentと
Source Mapも各versionへ対応する。現在のverifierがrequestのversionをsupportしない場合、canonical比較を
実行せず、matching Forma versionで検証するよう明示的に拒否する。将来過去versionをsupportする場合は、
versionに対応するbuilderへdispatchする。

現在は`generation-request/v0alpha1`とReview Requirements導入前のincremental `v0alpha2`をhistorical artifactとして
読み取り可能に保つ。中間の`v0alpha3`は受理しない。version-dispatchedなbuilderを持たない以上、そのschemaが運ぶAcceptance Factsを現在のbinaryは再導出できず、supportを宣言しても果たせないためである。current交換形式
`v0alpha4`はReview Requirementのincremental diffと、baselineのSource Map / Review Requirements versionを持つ。
historical schemaはIdentityを含まずcanonical Review Requirementsが空の場合だけ受理し、同じschema名のまま
unknown fieldを追加しない。

historical requestのdigestはschema専用codecで元のcanonical byte列から計算する。diff前にcompiler outputだけを
current in-memory shapeへlosslessにupgradeし、requestをcurrent compilerで作り直したりlineageを付け替えたりしない。
`ValidateIncrementalBaseline`も同じupgradeとintent / fact / review diffを再実行する。実際にtargetへ適用された
admin `v0alpha2` Git blob `5751ecf85e9b7be2665aa91854ee5b69798e81a3`から、admin semanticsを保ったまま
Identityを追加する`v0alpha4` requestへのpairwise lineageをtestで固定している。

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

最初のincremental experimentでは、immutableな`v0alpha1` full requestをbaselineとし、`User.nickname`追加と
page size 20→10を`v0alpha2` incremental requestへ導出した。8 intent nodesがadded/changed、13 Factsの
payloadがchanged、30 Factsがunchangedだった。既存targetへfull regenerationなしで適用し、43/43 Facts、
12 distinct tests、2 policyの`satisfied`と1 preferred policyのreason付き`deviated`、root/targetのbuild checksを
確認した。

## 現在のprototypeとの関係

[`../experiments/admin-e2e`](../experiments/admin-e2e/README.md)と
[`../experiments/conformance`](../experiments/conformance/README.md)は、Forma sourceからどの意味を
取り出す必要があるかを調べるための過去のprototypeとして凍結する。Go generator、profile capability、
共通adapter、byte-identical artifactをForma coreの将来architectureとはしない。
