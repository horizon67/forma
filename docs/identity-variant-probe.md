# Identity Variant Probe

Status: B5 comparison gate complete — test-only Go fixtures; not valid Forma syntax

## 1. 目的

このprobeは、email + password + verification link用に作ったIdentity modelが、別方式を最初から排除するほど
狭くなっていないかを確認する。対象は次の3例である。

1. local passwordを持たないpasswordless authentication
2. application外のauthorityを使うexternal provider authentication
3. authenticated subjectによる既存email identifierの変更

比較fixtureは[`../internal/compiler/identity_variants_test.go`](../internal/compiler/identity_variants_test.go)に置く。
surface syntaxを先に決めず、当時のResolved Intent `v0.5`、Acceptance Facts `v0alpha2`、Review Requirements
`v0alpha1`へ直接与えた。

## 2. 結果

| fixture | current end-to-end builder | generic Fact validator | 結論 |
| --- | --- | --- | --- |
| passwordless | reject | credential非依存の26/38 Factsを受理 | Fact registryは部分集合を扱えるが、Identity shapeとbuilderがlocal credential前提 |
| external provider | reject | 必要な新Fact kindはcontract未定義としてreject | providerをCredentialへ押し込むのは意味的に誤り |
| email change | reject | 必要な新Fact kindはcontract未定義としてreject | candidate identifierとcommit lifecycleの宣言場所がない |

3 fixtureとも、unsupportedな形を黙って受理したり通常field/actionへ縮退させたりしなかった。この意味でproposal
gateは機能した。一方、3方式を現行schemaで完全に表現できるという結果ではない。

## 3. Passwordless

### Probe

canonical membership fixtureから`proofs`と`credentials`、registration/authenticationのproof/credential参照、
画面のcredential inputを除いた。current validatorは次で拒否した。

```text
first Identity slice requires one identifier, proof, credential, verification, and ownership
```

`BuildAcceptanceFacts`も同じ境界で止まる。現在の`addIdentityFacts`は、identifier、credential、verification、
authentication、ownershipを1個ずつ持つ29-Fact bundleとして実装されているためである。
`BuildReviewRequirements`もinvalid intentを下流へ流さず、同じshape diagnosticで停止する。

一方、credential IDを参照しないFactsだけを残すと、generic `ValidateAcceptanceFacts`は**26/38**を受理した。
これによりB2でfixture固有の「29件ちょうど」「registry全kind出現」を汎用validatorから外した判断は実際に効いた。

除外された12 Factsには2種類ある。

- passwordlessでは不要になるもの: `credential-bound`、`credential-non-projectable`、passwordの
  `secret-input-not-preserved`
- 別のproofで再導出すべきもの: authentication accepted/rejected/ineligible、session termination、ownership、
  registration validation/input

後者まで消してよいわけではない。必要なのはFact削減だけでなく、authentication proofをlocal credentialから
独立させることである。

Stage Cのfirst sliceでは、この合成builderが未実装のまま複数interactionを受理しないよう、各operationのinteractionを
application全体でちょうど1件に制限する。passwordlessなどでFact builderを合成型へ変更するときに、このcardinalityも
同時に再検討する。

passwordlessでもverification evidenceはruntime secretなので、`secret-redaction`と`secret-storage`を消してはならない。
`fixture-fidelity`も同様に残る。将来のbuilderは「CredentialがないからReview Requirementsもない」と推論せず、
verification evidenceと採用したproof nodeからsecret boundaryを導出する必要がある。

### 不足axis

- `IRAuthentication`が参照するproofの種類。local credential、verification evidence、external assertionを区別する。
- registrationのatomic outcomeを固定4要素でなく、採用したproof/verificationから導出する規則。
- monolithicな29-Fact builderではなく、存在するsemantic nodeとoperationからFact群を合成するbuilder。
- passwordless linkをsignup verificationとsignin proofのどちらとして扱うかという明示的なlifecycle。

## 4. External provider

### Probe

providerをlocal-password proof/credentialの位置へ押し込むfixtureを作った。serialization自体は可能だが、
Stage Cのvalidatorは次で拒否する。

```text
proof ... is not the supported local-password proof
```

この拒否は正しい。external providerはUserがapplicationへ提示するlocal secretではなく、外部authorityのassertionと
subject mappingを信頼するauthentication方式である。passwordと同じCredential nodeにすると、secret storageの
Review Requirementやcredential binding Factも誤って導出される。

external assertionやprovider access/refresh tokenをapplicationが扱う場合、そのredaction/storage reviewは必要だが、
local passwordのReview Requirementを流用して意味を偽装してはならない。review sourceは将来のexternal assertion nodeから
導出する。

### 不足axis

- external authority / providerのsemantic identity
- provider subjectとdomain subjectのmapping、初回作成と既存account linkの規則
- authentication開始、return/callback、拒否・取消しというoperation
- providerが返すidentifierのcanonicalizationと衝突時の結果
- assertion成功からsession確立までのatomic boundary
- external authentication用の正常系・state mismatch・unknown mapping・replay拒否Fact kindとpre/post contract

issuer URL、client ID/secret、OAuth/OIDC library、callback routeは実装policyまたはrepository設定であり、Forma coreの
nodeへ入れない。Formaが固定するのはauthorityとmappingをapplication behaviorとしてどう扱うかである。

## 5. Email change

### Probe

authenticated pageから`identity/UserAccount/operation/change-email`を参照するinteractionを追加した。current modelには
operation declarationの所有nodeがないため、validatorは次で拒否した。

```text
interaction ... references missing operation identity/UserAccount/operation/change-email
```

candidate emailを2個目の`IRIdentifier`として偽装するprobeも、first sliceの「identifierは1個」という境界で拒否された。
これは単なる件数制限を外せば解決する話ではない。2個目のcurrent login identifierと、検証前のcandidate bindingは
異なるlifecycleを持つ。

既存`IRVerification`の追加だけでも表現できない。現在のverificationはPending subjectをActiveへ変えるstate actionと、
既存identifier宛てnoticeに固定されている。email変更で必要なのはActiveのままcandidate valueを検証し、成功時だけ
current identifierを置換することである。

email変更のverification evidenceもsecret redaction/storage reviewの対象であり、identifier置換testには
`fixture-fidelity`が必要である。現在の3つのreview kindは再利用できるが、sourceNodesへcandidate verificationと
変更operationを含める導出規則が必要になる。

### 不足axis

- identifier change requestを所有するnamed semantic nodeとinitiate/confirm/resend operation
- current identifierと、まだloginに使えないcandidate identifierの別々のbinding
- candidateに対するunique/canonical-equivalent conflict
- evidenceをsubjectとcandidate identifierの組へ束縛する規則
- verification成功時のatomic identifier置換と、失敗・expiry時にcurrent identifierを維持する規則
- 既存sessionを維持・再認証・失効のどれにするかというpolicy
- identifier変更の正常系、重複、invalid/expired/consumed evidence、at-most-once用Fact kindとcontract

## 6. 採用判断

Stage CのResolved Intent `v0.6`は、`v0.5`のemail + password + verification semanticsを維持しつつ、
local passwordを独立したAuthentication Proof nodeとして明示した。passwordless、external provider、email変更を
曖昧なoptional fieldやCredential kindとして同versionへ追加しない。

次のsurface syntaxでは対応範囲を明示し、未対応方式を通常field/actionへfallbackさせない。将来これらを採用する場合は、
少なくとも次のsemantic axisを先に設計する。

```text
Authentication Proof
  ├── Local Credential
  ├── Verification Evidence
  └── External Assertion

Identifier Lifecycle
  ├── Current Binding
  ├── Candidate Binding
  └── Verified Atomic Replacement

External Authority
  ├── Subject Mapping
  ├── Account Creation / Link
  └── Assertion → Session
```

この比較により、共通化すべき中心は「credential」ではなく**proof**、email変更で追加すべき中心は通常のfield editではなく
**identifier binding lifecycle**だと分かった。これらはsyntaxの別名ではなく、次のResolved Intent versionを必要とする
独立したsemantic axisである。
