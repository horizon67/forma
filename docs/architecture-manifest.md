# Architecture Manifest Proposal

Status: exploratory proposal — not a language decision and not part of the v0 grammar

この文書は、Ruby/Rails、AWS、database、libraryなど、人間が当面選定する実装architectureを
Forma projectでどう扱うかという一案を記録する。規範仕様は
[`v0-primitives.md`](v0-primitives.md)であり、ここに示すfile名、TOML schema、profile分割、keywordは
未決定である。

## 問題

Forma sourceはapplicationの意味をtarget-neutralに記述する。一方、実際にapplicationを生成・運用する
には、当面は人間が次のような判断を行う必要がある。

- runtimeとframework: Ruby、Railsなど
- databaseとpersistence adapter
- cloud providerとcompute、database、object storage
- authorization、job、observabilityなどを実装するlibrary
- version policy、region、deployment topology

これらを`.forma`へ直接書くと、application semanticsが特定のframeworkやcloudへ結び付く。反対に、
generatorの内部へ隠すと、projectの恒久的なarchitecture判断を人間がreviewできない。この間を埋める
versioned inputとして、Architecture Manifestを検討する。

## source of truthの分担

複数の真実を同じ対象について重複させるのではなく、異なる判断を異なるartifactが所有する。

| artifact | 所有する判断 | 編集者 |
| --- | --- | --- |
| `*.forma` | entity、state、action、permission、pageなどのapplication intent | 人間 |
| `forma.arch.toml`（仮） | runtime、framework、provider、adapterなどのarchitecture policy | 人間。当面はAIの提案を承認してもよい |
| `forma.lock`（仮） | 解決済みprofile、generator、direct dependency version、digest | resolverが生成し、人間がreviewする |
| generated artifact | target code、infrastructure definition、target固有lockfile | generator。人間は手編集しない |

Architecture Manifestはapplication semanticsの第二の定義ではない。`.forma`が「何を実現するか」を、
Manifestが「どの実装方針で実現するか」を所有する。

## profileの分割案

一つの巨大なtarget profileへすべてを入れず、少なくとも次を分ける案を検討する。

### Application Profile

Semantic IRを特定のruntimeとframeworkへloweringする。component、transport、persistence adapter、
runtime behavior、target側test harnessなどを所有する。

### Deployment Profile

生成されたapplication artifactを特定のproviderへ配置する。compute、managed database、network、
object storageなどを所有する。secretの値そのものは所有しない。

### Architecture Manifest

projectが使用するApplication ProfileとDeployment Profileを選択し、両者のcapabilityをbindする。
RailsをAWS以外へ配置でき、AWS上でRails以外も動かせるよう、両profileを独立して組み合わせられる形を
目指す。ただし、無制限なprofile compositionはhidden behaviorを増やすため、compatibilityは生成前に
機械的に検査する。

```text
Forma source
    ↓ deterministic front-end
Semantic IR + Conformance Contract
    │
    ├─ Application Profile: Ruby + Rails
    └─ Deployment Profile: AWS
              ↓
Generated Application + Infrastructure
              ↓ build + conformance
Accepted Artifact
```

## Manifestの概念例

次は議論用のTOML sketchであり、schemaの決定ではない。product名やservice名をForma grammarの
keywordにせず、versioned profile/capability identifierとして扱うことを想定している。

```toml
schema = "forma/architecture/v0"

[application]
profile = "ruby-rails"
runtime = "ruby"
framework = "rails"
database = "postgresql"

[deployment]
profile = "aws"
region = "ap-northeast-1"
compute = "ecs"
database = "rds"
object_storage = "s3"

[[adapters]]
capability = "persistence"
implementation = "active-record"

[[adapters]]
capability = "authorization"
implementation = "pundit"
```

## libraryをpackage名だけで指定しない

次のようなflat package listだけでは、dependencyが何のために存在し、どのForma semanticsを実装するかを
読めない。

```toml
packages = ["pundit", "aws-sdk-s3"]
```

可能な限り、direct dependencyをcapabilityとimplementationの対応として宣言する。

```toml
[[adapters]]
capability = "authorization"
implementation = "pundit"

[[adapters]]
capability = "object-storage"
implementation = "aws-sdk-s3"
```

libraryはFormaに存在しないapplication behaviorを暗黙に追加してはならない。たとえばauthorization
ruleは`.forma`の`allow`が所有し、PunditはRails上でそれを実装するadapterにすぎない。observableな
semanticsを追加するlibraryが必要なら、単なるpackageではなくcompiler-visibleなcapabilityまたは
versioned extensionとして設計する。

人間が指定するのはdirect dependencyとversion policyまでとし、transitive dependency、正確なversion、
digestは`forma.lock`または生成targetのlockfileへ解決する案が考えられる。

## 人間選定から自動選定への移行

別々の仕組みを作るのではなく、同じarchitecture axisをどこまで固定するかで段階を表現する。

1. **Pinned**: 人間がRuby、Rails、AWS、serviceまで指定する。
2. **Constrained**: managed infrastructure、relational database、Japan regionなどのrequirementだけを指定する。
3. **Automatic**: requirementとprofile capabilityからgeneratorが候補を選び、人間の承認後にlockする。

将来は次のような要求だけを人間が保守する可能性がある。

```toml
[requirements]
region = "japan"
managed = true
relational_database = true
```

AIがarchitectureを選定する場合も、modelの一時的な回答をそのままbuild入力にはしない。選定結果を
機械可読なplanとして提示し、人間またはpolicyが承認した結果をversioned lockへ固定する。Forma sourceと
conformance contractは選定方法に依存しない。

## build keyとの関係

Architecture Manifest、解決済みprofile、generator設定、lockは生成結果へ影響するため、artifactの
build keyへ含める必要がある。

```text
build key = hash(Semantic IR, Conformance Contract,
                 Architecture Manifest, resolved profiles,
                 generator configuration, architecture lock)
```

architectureを変更してもapplication semanticsが同じなら、同じconformance contractを通過しなければ
ならない。profileが実装できないcapabilityはAI generatorを呼ぶ前にcompatibility errorとする。

## 未決定事項

- ManifestをTOMLにするか、Forma lexer/parserを共有する別grammarにするか。
- Application ProfileとDeployment Profileの責務境界。
- databaseのように両profileへまたがるcapabilityのbinding方法。
- profile、capability、adapter identifierのregistryとversioning。
- direct dependencyのversion constraintと`forma.lock`のschema。
- staging、productionなどenvironment差分の表現。
- availability、latency、cost、data residencyなどnon-functional requirementの置き場所。
- secretの参照方法。secret valueをsourceやlockへ保存しない仕組み。
- AIによるarchitecture proposalの承認、更新、rollback protocol。
- infrastructure conformanceをapplication conformanceと同じcontractへ含めるか、別contractにするか。

## 検証方法

schemaを決める前に、少なくとも次の二例を最後まで記述して比較する。

1. 同じForma applicationをRuby/Rails + AWSへ生成する明示的なPinned構成。
2. 同じForma applicationを異なるframework/providerへ生成する構成、またはrequirementだけを指定する
   Constrained構成。

比較では、次を確認する。

- `.forma`を変更せずarchitectureだけを交換できるか。
- direct libraryの目的をManifestから説明できるか。
- profile incompatibilityをgeneration前に検出できるか。
- lockから同じarchitecture inputsを再現できるか。
- architectureを交換しても同じconformance contractを適用できるか。
- Manifestが巨大なvendor-specific DSLへ成長していないか。

この検証を行うまでは、`forma.arch.toml`、profile分割、Pinned/Constrained/Automaticという名称を
言語またはcompilerの決定事項にしない。
