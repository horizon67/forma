# Implementation Policy Manifest Proposal

Status: design proposal — schema未確定、incremental experimentで検証する

## 1. 目的

Formaは、applicationの意味と実装技術の選択を別々の入力として扱う。

```text
Forma source
  「何を作るか」
  - entity、field、state
  - page、action、permission
  - validation、navigation
  - Acceptance Facts

Implementation Policy Manifest
  「何を使って、どの方針で作るか」
  - framework、library
  - frontend/backendのarchitecture方針
  - codeを変えるinfrastructure選択
  - required、preferred、forbidden

Target repository
  「現在どうなっているか」
  - 既存codeとdependency
  - directoryとarchitecture
  - build/test command
  - repository policy

          ↓

Generation Request + repository
          ↓
AI coding agent
          ↓
通常のapplication code
```

`.forma`はapplicationとして観測できる意味を所有する。Implementation Policy Manifestは、その意味を
target repositoryで実現するときの技術選択と制約を所有する。coding agentは両方とrepositoryの現状を
読み、具体的なcodeへ統合する。

例えば、次のForma sourceは検索機能を要求するが、検索libraryを指定しない。

```forma
list User {
    search name, email
}
```

Manifestが`ransack`をrequiredとしていれば、agentはRails repositoryでこの検索をRansackにより実装する。
Frontendのremote data取得に`@tanstack/react-query`がrequiredなら、client fetchを必要とする実装でそれを
使用する。検索結果の意味はAcceptance Facts、RansackやTanStack Queryの使用はImplementation Policy
Coverageで別々に検査する。

## 2. なぜrepositoryだけでは足りないか

CLAUDE.md、AGENTS.md、`.cursorrules`など、agent-driven開発では実装方針を伝える文書がすでに使われている。
Manifestは新しい要求を発明するものではなく、この散文を構造化し、指示の欠落を検査可能にするものである。

repositoryの現在状態だけでは、少なくとも次を表現できない。

- **greenfield** — 参照する既存codeやdependencyがまだない。
- **shouldとisの区別** — jQueryとReactが混在していても、「新規codeはReact」は現状から導出できない。
- **forbidden** — dependencyが存在しないことは、「今後も使用禁止」を意味しない。

したがってManifestは、**repositoryが自分の現在状態だけでは語れない、実装上の意思**を置く場所とする。

## 3. 旧target profileへ戻らないための不変則

最重要の設計制約は次である。

> **Forma coreのcodeは、user-definedなkeyまたはopaqueな技術valueによって分岐しない。**

Forma coreはschema自身が定める`required`、`preferred`、`forbidden`の検証規則では分岐する。一方、
`ransack`だからRuby用処理を行う、`backend/search`だから検索capabilityとして扱う、という分岐はしない。

`ransack`、`@tanstack/react-query`、`postgresql`はFormaにとってopaqueな文字列である。Forma coreは、
それらが実在するpackageか、特定capabilityに適するか、どのframeworkで利用できるかを判定しない。

同様に、`backend.search`や`frontend.remoteData`のようなcapability key体系をForma coreが定義しない。
Manifest schemaが固定するのはpolicy entryの共通形だけであり、entryの意味とForma intentの対応はagentが
repository contextとinstructionから判断する。

Forma coreが行ってよいのは次までである。

- schema versionと構造の検査
- stable policy IDの一意性とcanonical order
- policy modeに応じたcoverageの存在検査
- repository相対evidence pathの構文と存在確認
- opaqueなvalue文字列がevidence fileに出現するかという最小確認
- forbidden valueの機械的scan結果の表示

Forma coreは次を行わない。

- library/capability registryの保守
- intentからlibraryを選ぶmapping
- framework compatibility判定
- evidence codeがlibraryを正しく使用しているかの意味解析
- Manifestからframework別artifactを決定的にloweringすること

## 4. source of truthの分担

| 入力 | 所有するもの |
| --- | --- |
| `*.forma` | application behaviorとdomain/UI intent |
| Resolved Intent | 解決済みapplication intent |
| Acceptance Facts | 実装後に成立すべきtarget-neutralなbehavior |
| `forma.implementation.yaml` | repositoryのdesired implementation policy |
| repository code/config | 現在のimplementationとdependency |
| repository policy文書 | Manifest以外のauthoritativeな制約 |
| Generation Request | agentが実際に受け取った上記入力のimmutable snapshot |

Manifestはdependency manifestやlockfileを置き換えない。既存repositoryでは、package versionの正本は
Gemfile、package.json、lockfileなど既存ecosystemのfileに置く。Manifestは「新規検索ではRansackを使う」
のようなdesired policyだけを追加する。greenfieldでversion選択そのものが意思なら、その場合だけversion
constraintをpolicyとして記録する。

## 5. 名前と配置

概念名は**Implementation Policy Manifest**、人間が編集するfile名は次を候補とする。

```text
forma.implementation.yaml
```

target repositoryの方針を語るため、target repository内へ置く。YAMLは人間向けauthoring formatであり、
Forma orchestration layerはschema検査後にcanonical JSONへ正規化する。

過去にはApplication/Deployment Profile、capability matrix、framework lowererをForma coreが所有する方式を
検討したが、intentから技術への対応表をcoreへ持ち込み、framework別generator projectへ近づくため棄却した。
本書はそのprofile方式の再導入ではない。Forma coreはpolicyの技術valueを解釈せず、coding agentへ渡す
opaqueなrequired/preferred/forbidden policyの構造とcoverageだけを扱う。棄却した方向は
[`roadmap.md`](roadmap.md)の「凍結した方向」にも記録する。

## 6. 最小authoring model

最初からbackend/frontend/infrastructureの固定key体系を作らない。次のようなgeneric entry列から始める。

```yaml
schema: forma/implementation-policy/v0alpha1

policies:
  - id: implementation/backend/search
    policy: required
    value: ransack
    instruction: Server-side list searchにはRansackを使う

  - id: implementation/frontend/remote-data
    policy: required
    value: "@tanstack/react-query"
    instruction: Client-side remote data取得にはTanStack Queryを使う

  - id: implementation/date-library
    policy: forbidden
    value: moment
    instruction: 新規codeへMoment.jsを追加しない

conventions:
  - 既存のservice objectとrequest specの配置を維持する
```

`id`、`policy`、`value`は検証対象である。`instruction`はagentがopaqueなvalueをどの場面へ適用するか
理解するための説明であり、Forma coreは解釈しない。

`conventions`は意図的に助言的な散文として分離する。policy IDやcoverageを持たず、機械検証されない。
検証可能な指示を`conventions`へ置くとManifest全体がunchecked promptへ戻るため、required/preferred/
forbiddenとして合否に関わるものは`policies`へ置く。

このYAML shapeはproposalであり、incremental experimentの結果を受けてから固定する。

## 7. policy mode

### required

agentは指定valueを使用し、`satisfied`とevidenceを返す必要がある。実装不能、別のauthoritative policyとの
衝突、依頼scopeを超える変更が必要な場合は黙って代替せず、`conflict`または`blocked`として返す。

### preferred

agentは可能なら指定valueを使用する。使用しない場合は`deviated`と具体的なreasonを必須にする。
理由のない欠落はsuccessにしない。`preferred`はoptionalの別名ではない。

### forbidden

agentは指定valueを新規実装へ導入しない。初期schemaではopaque文字列scanを行い、出現を即座に
semantic failureとはせずflagとして表示する。Manifest自身、immutable request、feedback、`.git`、build
cacheなどはscan対象から除外する。documentやlockfileへの出現などfalse positiveがあるため、最終判断は
reviewへ残す。scan hitがなければ`satisfied`、hitがあれば`flagged`と出現pathをcoverageへ返す。

## 8. repositoryとの差分とconflict

Manifestはdesired state、repositoryはcurrent stateである。したがって、両者の差は自動的なconflictでは
ない。例えばManifestが`ransack required`でrepositoryが`pg_search`を使用している場合、Ransackへ変更する
ことが依頼された作業かもしれない。

次の場合はagentがconflictとして報告する。

- AGENTS.mdなど別のauthoritative policyが反対の技術をrequired/forbiddenとしている。
- policyを満たす変更がrequested changeのscopeを大きく超える。
- repositoryのruntimeやarchitecture上、policyを満たせない。
- Manifest自体が古く、現在の明示的な人間の意思と矛盾している可能性がある。

複数のauthoritative policy間に暗黙の優先順位を作らない。現在の明示的なuser overrideがGeneration Requestへ
記録されていない限り、agentは一方を黙って選ばず人間へ返す。

## 9. Generation Requestへの格納

orchestration layerはYAMLを検査・canonicalizeし、正規化済みManifestをGeneration Requestへ埋め込む。

```text
GenerationRequest
  resolvedIntent
  acceptanceFacts
  sourceMap
  implementationPolicy
    schema
    policies
    conventions
  requestedChange
  verification
```

agentへ渡した後にrepository上のYAMLが変更されても、完了判定はorchestration layerがimmutableに保持した
request内のsnapshotに対して行う。現在fileを読み直して判定根拠にしない。これはAcceptance Factsと
`requiredFactIds`で採用したtrust boundaryと同じである。

Manifestの正規化はGeneration Request作成処理の責務だが、application semanticsではない。Resolved Intent
やAcceptance Factsへpolicyを混ぜない。同じForma sourceを別Manifestで実装しても、application behaviorの
期待は同一である。

## 10. Implementation Policy Feedback

Acceptance Fact Coverageとは別に、target固有のImplementation Policy Coverageを返す。

```json
{
  "policyCoverage": [
    {
      "policyId": "implementation/backend/search",
      "status": "satisfied",
      "evidence": [
        "Gemfile",
        "app/models/user.rb"
      ]
    },
    {
      "policyId": "implementation/frontend/remote-data",
      "status": "deviated",
      "reason": "この画面はSSRのみでclient fetchが発生しない"
    }
  ]
}
```

最小verify規則は次とする。

| policy | successに必要なcoverage |
| --- | --- |
| required | `satisfied`、1件以上のevidence、evidence fileの存在、少なくとも1 file内のopaque value出現 |
| preferred | requiredと同じ`satisfied`、またはnon-empty reason付き`deviated` |
| forbidden | `satisfied`、または出現path付き`flagged`。flagは機械的failureにせず人間が確認 |

未知policy ID、重複coverage、required欠落、preferredの無言欠落、repository外pathは拒否する。evidence pathは
repository rootから解決し、root外へescapeできないようにする。

## 11. Acceptance Factsとの非対称

| 検証 | 保証できること | 保証しないこと |
| --- | --- | --- |
| Acceptance Fact Coverage | target固有testが実行され、期待behaviorがpassedと報告された | testがfactを忠実に検査することの証明 |
| Implementation Policy Coverage | evidence fileが存在し、opaque valueが現れ、逸脱・衝突が無言でない | libraryを適切な場所・方法で使用したことの意味解析 |

Implementation evidenceは実行testより大幅に弱い。Forma verifyが提供するのは、完全な出まかせを減らす
機械的な床である。policyの適用が妥当かはcode reviewで確認する。この限界をAcceptance Factsと同じ保証として
表示してはならない。

## 12. 採録基準と非目標

policy entryは、**その指定によりagentが書くcodeまたはconfigが変わるか**を採録基準とする。

載せる候補:

- framework、library、runtime
- API/client data取得、state managementなどのimplementation方針
- databaseとmigration方式
- queue、cache、storageなどcode integrationを伴うinfrastructure
- 禁止dependencyや新規codeの移行方針

原則として載せないもの:

- application behaviorやdomain rule — `.forma`へ書く
- repositoryから一意に読める現在状態の無意味な複製
- application codeを変えないdeployment上の分類
- secretやcredentialの値
- Forma coreが理解すべきlibrary/capability registry
- framework別generator、adapter、capability matrix

`deployment: aws`のようなentryは、それがIaC、SDK、storage、runtime configなどagentの変更対象を実際に
変える場合だけ採用する。単なる運用上のラベルならManifestへ入れない。

## 13. 最初のincremental probe

schemaを先に完成させず、[`../experiments/admin-agent-e2e`](../experiments/admin-agent-e2e/README.md)の
既存Go targetへのincremental changeに最小Manifestを同乗させる。

```yaml
schema: forma/implementation-policy/v0alpha1

policies:
  - id: implementation/server-rendering
    policy: required
    value: html/template
    instruction: Server-rendered UIはhtml/templateで維持する

  - id: implementation/persistence
    policy: preferred
    value: database/sql
    instruction: 永続化にはdatabase/sqlを優先する

  - id: implementation/router
    policy: forbidden
    value: github.com/gorilla/mux
    instruction: 標準net/http routerを維持する

conventions:
  - cmd/serverとinternal package境界を維持する
```

agentが既存の`html/template`実装を維持し、controlled experimentではin-memory storeを残し、
Gorilla Muxを導入しなかった場合、対応するFeedbackは次の形になる。

```json
{
  "policyCoverage": [
    {
      "policyId": "implementation/server-rendering",
      "status": "satisfied",
      "evidence": [
        "internal/web/server.go"
      ]
    },
    {
      "policyId": "implementation/persistence",
      "status": "deviated",
      "reason": "このcontrolled experimentではin-memory storeを維持した"
    },
    {
      "policyId": "implementation/router",
      "status": "satisfied"
    }
  ]
}
```

この例では、required policyはevidence file内の`html/template`出現、preferred policyはnon-emptyな逸脱理由、
forbidden policyは除外規則適用後のscan hitが0件であることにより、それぞれ最小verify条件を満たす。

このprobeでは次を確認する。

1. requiredが実際に使用され、evidence file存在とopaque value出現を検査できる。
2. preferredを意図的に`deviated`とし、reasonの必須経路を検査できる。
3. forbidden scanのhit/除外/flag表示を検査できる。
4. 正規化済みManifestがimmutableなGeneration Requestへ格納される。
5. Manifestのkey/valueをForma coreが意味解釈せず、genericなcoverageだけを検証できる。
6. `.forma`のincremental changeとimplementation policyを同時にagentへ渡し、既存codeとtestを保てる。

## 14. 残る問い

- policy `value`とgrep対象tokenを同一にするか、明示的な`evidenceToken`を許すか。
- forbidden scanのdefault scopeと、repository固有excludeをどう表すか。
- policyを特定SemanticIDへ任意で関連付けるか。関連付ける場合もForma coreは意味を解釈しない。
- greenfieldでrepository rootとbuild/test commandをどこまでManifestに持たせるか。
- version変更時に過去Manifest verifierをどこまで保持するか。
- `conventions`をGeneration Requestへ埋め込む場合、uncheckedな助言であることをUIでどう示すか。

これらは最初のprobe結果から決める。Implementation Policy ManifestをForma grammarのprimitiveにはしない。
