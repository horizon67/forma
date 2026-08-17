# Identity Semantic Model Proposal

Status: Stage B implementation — B1–B4 complete; B5 comparison gate next; not valid Forma syntax

## 1. 目的

このproposalは、[`email-verified-membership-probe.md`](email-verified-membership-probe.md)で固定した
メール認証付き会員登録flowを、coding agentへ渡せるResolved Intent、Acceptance Facts、Review Requirementsへ
落とす最小modelを定義する。

対象は次の意味である。

```text
User.email identifier
  + password credential
  + Pending registration
  + 30分・一度限りのemail verification
  + verification resendとrotation
  + Active Userだけのsignin
  + current sessionのsignout
  + selfだけのprofile access
```

ここではForma surface syntax、password hash、token format、session transport、route、framework APIを決めない。
先に「何を実装し、何を観測すべきか」だけをtarget非依存に固定する。

## 2. このproposalで決めること

- Identity、Identifier、Credential、Verification、Authentication、Session、OwnershipのResolved Intent node。
- domain operationとpage上のinteraction referenceの分離。
- state、action、page、fieldなど既存nodeとの参照関係。
- credential/evidence値を持たないFact-localなsemantic setup。
- Stage Aで固定した29 Acceptance FactsのID、kind、導出元。
- Acceptance Factへ混ぜない3つのReview Requirement。
- stable semantic ID、Source Map、canonical ordering、schema version境界。
- 旧admin requestをbaselineにしたincremental E2Eへ進むためのversion migration境界。

次は決めない。

- `identity`をForma primitiveとして数えるか。
- `register`、`signin`などのsurface syntax。
- passwordless、external provider、email変更の最終shape。
- 汎用Occurrence / Effect modelとの統合方法。

## 3. 設計原則

### 3.1 Identityは専用semanticとして閉じる

credential、verification、principal、sessionの安全性を、自由なaction bodyやEffectの組合せへ委ねない。
Identityに固有のclosed vocabularyとして検査し、coding agentが安全上重要な意味を推測する余地を残さない。

一方、verification noticeのdurable emissionは後続の汎用Effect modelと合流できるよう独立nodeにする。最初の
sliceではIdentity専用nodeとして保持し、未決定のEffect syntaxを先取りしない。

### 3.2 domain operationとUI interactionを分ける

`resend`などのdomain operationはIdentityの下へ一度だけ宣言し、pageはinteraction referenceから参照する。
同じoperationを複数画面から起動しても意味が重複しない。navigation、入力提示、feedbackはpage側のinteractionが
所有する。

```text
identity/UserAccount/operation/resend
  ↑ referenced by
page/CheckEmail/identity/resend/UserAccount
```

これはv0でdomain actionをtop levelに置き、viewがaction referenceを持つ設計と同じである。

### 3.3 secret valueをcompiler outputへ入れない

Resolved Intentはcredentialのkindとpolicyを持つが、password value、hash、verification tokenを保持しない。
Acceptance Factsも値ではなくsubject-scopedなsymbolic handleを使う。

```text
subject/alice/credential/primary
subject/alice/evidence/current
```

handleは値を取得するkeyではない。repository固有test内で「同じ合成test credential/evidenceを再参照する」という
同一性だけを表す。

### 3.4 Factは互いに依存しない

Factは`dependsOn`を持たない。各Factは新しい隔離scenarioから単独・任意順・反復可能に実行する。必要な
preconditionはFact自身のsemantic setupへ含める。setup失敗はそのFactの失敗であり、別Factの結果を伝播しない。

### 3.5 機械検査と人間reviewを混ぜない

FormaがResolved Intentから再計算できる29件はAcceptance Factsとする。compilerが導出するclosed setupは、
Fact kindごとのprecondition/postcondition規則で自己充足していないことまで機械検査する。secret storage、runtime
redaction、repository固有testへ変換した後のfixture fidelityは実装を読まなければ判定できないためReview
Requirementsとする。agentの自己申告でこれらを`passed`へ変換してはならない。

## 4. schema version境界

採用時は既存schemaへunknown fieldを黙って追加しない。候補versionは次とする。

| schema | 前version | 採用version | 理由 |
| --- | --- | --- | --- |
| Resolved Intent | `forma/resolved-intent/v0.4` | `v0.5` | Identity node、page interaction、access predicateを追加 |
| Source Map | `forma/source-map/v0.2` | `v0.3` | Identity node kindと`v0.5` intentへ対応 |
| Acceptance Facts | `forma/acceptance-facts/v0alpha1` | `v0alpha2` | semantic setupとIdentity fact payloadを追加 |
| Review Requirements | なし | `forma/review-requirements/v0alpha1` | 人間review対象を独立交換形式にする |
| Generation Request | historical `v0alpha1` / `v0alpha2`、previous `v0alpha3` | `v0alpha4` | Review Requirement diffとbaseline version metadataを追加 |
| Generation Feedback | `forma/generation-feedback/v0alpha2` | 維持 | agentがReview Requirementを完了報告するfieldを追加しない |

この表の名前はproposal上の候補であり、implementationとgoldenを同時に更新した時点で規範になる。
Resolved Intent `v0.5`とSource Map `v0.3`はB1、Acceptance Facts `v0alpha2`はB2、Review Requirements
`v0alpha1`とGeneration Request `v0alpha3`はB3で採用した。B4では既存`v0alpha3`へunknown fieldを足さず、
Review Requirement diffとbaseline version metadataを持つGeneration Request `v0alpha4`へ上げた。

## 5. Resolved Intentのtop-level shape

`ResolvedIntent`へ`identities`を、`IRPage`へ`identityInteractions`を追加する。

```text
ResolvedIntent
  version
  roles
  types
  entities
  actions
  identities[]
  pages[]
    identityInteractions[]
```

Identityはentityを置き換えない。`User`のname、email、statusは引き続きentityが所有し、Identityはそのentityを
authenticated principalへ結び付ける意味を所有する。

## 6. Identity node

候補のGo shapeを示す。これはparser ASTやsurface syntaxではない。

```go
type IRIdentity struct {
    ID              SemanticID
    Name            string
    Subject         SemanticID
    Identifiers     []IRIdentifier
    Credentials     []IRCredential
    Registration    IRRegistration
    Verifications   []IRVerification
    Authentication  IRAuthentication
    Ownership       []IROwnership
}
```

最初のsliceではIdentity 1件、Identifier 1件、Credential 1件、Verification 1件だけを許可する。fieldをsliceにする
のは複数identifierやpasswordlessを今すぐ実装するためではなく、後続比較例でtop-level schemaを壊さず検証する
ためである。checkerは未対応の個数・組合せをcompile errorにする。

### 6.1 Identifier

```go
type IRIdentifier struct {
    ID               SemanticID
    Name             string
    Field            SemanticID
    Canonicalization []IRCanonicalizationStep
}

type IRCanonicalizationStep struct {
    Kind string
}
```

最初のemail identifierは次を意味する。

```json
{
  "id": "identity/UserAccount/identifier/email",
  "name": "email",
  "field": "entity/User/field/email",
  "canonicalization": [
    { "kind": "trim-unicode-white-space" },
    { "kind": "ascii-case-fold" }
  ]
}
```

canonicalizationは記載順に適用する。`trim-unicode-white-space`はUnicode `White_Space` propertyに含まれる
code pointだけを両端から除く。`ascii-case-fold`はASCII `A-Z`だけを`a-z`へ変換する。保存表現は指定しない。

### 6.2 Credential

```go
type IRCredential struct {
    ID          SemanticID
    Name        string
    Kind        string
    InputPolicy IRCredentialInputPolicy
}

type IRCredentialInputPolicy struct {
    PreserveWhitespace bool
    Length             IRLengthConstraint
}

type IRLengthConstraint struct {
    Min  int
    Max  int
    Unit string
}
```

最初のcredentialは次になる。

```json
{
  "id": "identity/UserAccount/credential/password",
  "name": "password",
  "kind": "password",
  "inputPolicy": {
    "preserveWhitespace": true,
    "length": { "min": 12, "max": 128, "unit": "unicode-scalar-value" }
  }
}
```

このnodeにはvalue、hash、salt、algorithm、cost、storage fieldを置かない。Credentialは`IRField`ではなく、
list、detail、form field、search、filter、sort、labelの解決対象にもならない。

### 6.3 Registration operation

```go
type IRRegistration struct {
    ID                        SemanticID
    Identifier                SemanticID
    Credential                SemanticID
    Attributes                []SemanticID
    InitialState              IRStateValueRef
    Verification              SemanticID
    AtomicOutcome             []string
    ExistingIdentifierOutcome string
}

type IRStateValueRef struct {
    State SemanticID
    Value string
}
```

```json
{
  "id": "identity/UserAccount/operation/register",
  "identifier": "identity/UserAccount/identifier/email",
  "credential": "identity/UserAccount/credential/password",
  "attributes": ["entity/User/field/name"],
  "initialState": { "state": "entity/User/state/status", "value": "Pending" },
  "verification": "identity/UserAccount/verification/email",
  "atomicOutcome": [
    "credential-binding",
    "notice-emission-record",
    "subject",
    "verification-evidence"
  ],
  "existingIdentifierOutcome": "reject-and-guide-resend"
}
```

`atomicOutcome`は処理順ではなく、全部成立するか全部成立しないかという集合である。external providerへのdeliveryは
含めない。配列はcanonical JSONでは辞書順にする。

### 6.4 Verification、resend、notice

```go
type IRVerification struct {
    ID               SemanticID
    Kind             string
    Subject          SemanticID
    VerifyOperation  SemanticID
    ResendOperation  SemanticID
    EligibleState    IRStateValueRef
    SuccessAction    SemanticID
    Evidence         IRVerificationEvidence
    Notice           IRVerificationNotice
    ResendDisclosure string
}

type IRVerificationEvidence struct {
    Kind                 string
    Lifetime             IRDuration
    ValidBoundary        string
    MaxUses              int
    Rotation             string
}

type IRDuration struct {
    Amount int
    Unit   string
}

type IRVerificationNotice struct {
    ID              SemanticID
    Channel         string
    Recipient       SemanticID
    Emission        string
    DeliveryFailure string
}
```

最初の値は次である。

```json
{
  "id": "identity/UserAccount/verification/email",
  "kind": "opaque-email-link",
  "subject": "entity/User",
  "verifyOperation": "identity/UserAccount/operation/verify",
  "resendOperation": "identity/UserAccount/operation/resend",
  "eligibleState": { "state": "entity/User/state/status", "value": "Pending" },
  "successAction": "action/User/activate",
  "evidence": {
    "kind": "opaque",
    "lifetime": { "amount": 30, "unit": "minute" },
    "validBoundary": "now-before-issued-plus-lifetime",
    "maxUses": 1,
    "rotation": "invalidate-prior-unconsumed"
  },
  "notice": {
    "id": "identity/UserAccount/verification/email/notice",
    "channel": "email",
    "recipient": "identity/UserAccount/identifier/email",
    "emission": "durable-record-required",
    "deliveryFailure": "subject-remains-pending-and-retryable"
  },
  "resendDisclosure": "uniform-for-pending-active-and-unknown"
}
```

`successAction`は既存action nodeを参照する。checkerはそのactionが`Pending -> Active`であることを確認する。
`now-before-issued-plus-lifetime`は`now < issuedAt + 30 minutes`であり、同時刻はexpiredである。

`verifyOperation`と`resendOperation`は独立したsemantic IDを持つ。resendはstate transitionを要求しないため、
現行`IRAction`へ偽の`Pending -> Pending`を作らない。

### 6.5 AuthenticationとSession

```go
type IRAuthentication struct {
    ID                SemanticID
    SignInOperation   SemanticID
    SignOutOperation  SemanticID
    Identifier        SemanticID
    Credential        SemanticID
    EligibleState     IRStateValueRef
    FailureDisclosure string
    Session           IRSession
}

type IRSession struct {
    ID               SemanticID
    PrincipalSubject SemanticID
    SignOutScope     string
}
```

```json
{
  "id": "identity/UserAccount/authentication",
  "signInOperation": "identity/UserAccount/operation/signin",
  "signOutOperation": "identity/UserAccount/operation/signout",
  "identifier": "identity/UserAccount/identifier/email",
  "credential": "identity/UserAccount/credential/password",
  "eligibleState": { "state": "entity/User/state/status", "value": "Active" },
  "failureDisclosure": "generic",
  "session": {
    "id": "identity/UserAccount/session/current",
    "principalSubject": "entity/User",
    "signOutScope": "current-session"
  }
}
```

Session nodeはcookie、header、server store、JWTなどを指定しない。`current-session`はsignoutが現在の
authenticated principalを成立させているsessionだけを終了する意味である。

### 6.6 Ownership

```go
type IROwnership struct {
    ID        SemanticID
    Identity  SemanticID
    Resource  SemanticID
    Relation  string
}
```

```json
{
  "id": "identity/UserAccount/ownership/self",
  "identity": "identity/UserAccount",
  "resource": "entity/User",
  "relation": "principal-subject-equals-resource-identity"
}
```

最初のsliceはIdentityのsubject entity自身への`self`だけを扱う。`owns relation`など一般化したownershipは後続
probeへ残す。

## 7. page上のIdentity interaction

domain operationとpageを結ぶ参照nodeを追加する。

canonical membership fixtureは既存entity/page semanticsとして次も持つ。

- `User`はname、nickname、email fieldとstatus stateを持つ。nicknameはoptionalである。
- `page/Profile`は`page/Profile/parameter`でUserをbindingし、name、nickname、email、statusを表示する。
- `page/ProfileEdit`は`page/ProfileEdit/parameter`でUserをbindingし、name、nicknameだけを編集する。
- ProfileとProfileEditの両方へauthenticated + self ownershipを適用する。

Identity modelはこれらのdetail/form projectionを複製せず、既存page/view nodeを参照する。

```go
type IRIdentityInteraction struct {
    ID           SemanticID
    Operation    SemanticID
    Inputs       []IRIdentityInputRef
    Access       IRAccess
    Success      IRNavigationIntent
    Continuation *IRNavigationIntent
    Feedback     []string
}

type IRIdentityInputRef struct {
    Kind string
    Node SemanticID
}
```

`Kind`は最初のsliceで`field | identifier | credential | evidence`のclosed setとする。入力値は持たず、解決済み
nodeだけを参照する。

| interaction ID | operation | inputs | success / continuation |
| --- | --- | --- | --- |
| `page/SignUp/identity/register/UserAccount` | `operation/register` | name、email identifier、password credential | CheckEmail |
| `page/CheckEmail/identity/resend/UserAccount` | `operation/resend` | email identifier | same-context |
| `page/VerifyEmail/identity/verify/UserAccount` | `operation/verify` | verification evidence | RegistrationComplete / SignIn |
| `page/SignIn/identity/signin/UserAccount` | `operation/signin` | email identifier、password credential | Profile |
| `page/Profile/identity/signout/UserAccount` | `operation/signout` | なし | SignIn |

`Continuation`はverification成功後の完了surfaceからSignInへ進めることを表す。HTML linkやredirect方式は
指定しない。

SignUp、CheckEmail、VerifyEmail、RegistrationComplete、SignInはanonymous principalから到達可能とする。
Profileは次節のauthenticated + ownership accessを要求する。

## 8. Access predicateの拡張

現在の`IRAccess`はall-of clauseとrole any-ofを分けている。この構造を維持し、requirement kindをclosed setへ
拡張する。

```go
type IRAccessRequirement struct {
    Source          SemanticID
    Kind            string
    AnyOf           []string
    Identity        SemanticID
    Ownership       SemanticID
    ResourceBinding SemanticID
}
```

kindごとのpayloadは次だけを許す。

| kind | payload | 意味 |
| --- | --- | --- |
| `roles` | `anyOf` | 列挙roleのどれか |
| `authenticated` | `identity` | 指定Identityのcurrent principalが存在 |
| `ownership` | `ownership` + `resourceBinding` | predicateを指定page parameterへ適用して満たす |

```json
{
  "id": "page/Profile/access",
  "allOf": [
    {
      "source": "page/Profile",
      "kind": "authenticated",
      "identity": "identity/UserAccount"
    },
    {
      "source": "page/Profile",
      "kind": "ownership",
      "ownership": "identity/UserAccount/ownership/self",
      "resourceBinding": "page/Profile/parameter"
    }
  ]
}
```

ProfileEditでは`resourceBinding`が`page/ProfileEdit/parameter`になる。checkerはparameterのentityがownershipの
`resource`と一致することを検査する。これによりprincipal subjectと「現在表示・編集しようとしているUser」を
比較できる。

これにより将来も`(admin OR editor) AND authenticated AND owner`をflattenせず保持できる。

## 9. stable semantic ID

最初のsliceで固定するIDを示す。

| node | ID |
| --- | --- |
| Identity | `identity/UserAccount` |
| Identifier | `identity/UserAccount/identifier/email` |
| Credential | `identity/UserAccount/credential/password` |
| Registration | `identity/UserAccount/operation/register` |
| Verification | `identity/UserAccount/verification/email` |
| Verification notice | `identity/UserAccount/verification/email/notice` |
| Verify operation | `identity/UserAccount/operation/verify` |
| Resend operation | `identity/UserAccount/operation/resend` |
| Authentication | `identity/UserAccount/authentication` |
| Signin operation | `identity/UserAccount/operation/signin` |
| Signout operation | `identity/UserAccount/operation/signout` |
| Session | `identity/UserAccount/session/current` |
| Ownership | `identity/UserAccount/ownership/self` |
| Page interaction | `page/<Page>/identity/<operation>/<Identity>` |

IDはsource file、source position、target repositoryに依存しない。宣言の移動はIDを変えず、Identityまたは
operationのrenameはIDを変える。rename migrationの一般modelはまだないため、Stage Cの最初のsliceではrenameを
拒否またはremove + addとして明示する。

## 10. Source Map

Identity nodeも既存nodeと同じくSource Mapへ1対1で載せる。候補kindは次である。

```text
identity
identity-identifier
identity-credential
identity-registration
identity-verification
identity-verification-notice
identity-operation
identity-authentication
identity-session
identity-ownership
identity-interaction
```

複数のresolved nodeが将来同じsource clauseから導出される場合、異なるnode IDが同じspanを参照してよい。
Source Mapはruntime valueを持たず、credential/evidence value用fieldを追加しない。

## 11. semantic validation

Stage Cでsyntaxを受理する前に、Stage Bのchecker相当で少なくとも次を検査する。

### Reference integrity

- Identity subjectは存在するentityである。
- Identifier fieldはsubjectのrequired、non-collection、unique fieldである。
- Identifierのunique比較はraw field値ではなく、宣言されたcanonicalization適用後の値に対して行う。
- Registration attributesはsubjectのfieldで、Identifier fieldとCredentialを重複して含めない。
- Initial/eligible stateはsubjectのstateと閉じたvalueを参照する。
- Verification success actionはsubject entityのactionで、`Pending -> Active`と一致する。
- interactionのoperation、input、page、navigationは存在するnodeを参照する。
- Ownership resourceは最初のsliceではIdentity subjectと同じentityである。
- ownership requirementのresource bindingはpage parameterであり、そのentityはOwnership resourceと一致する。

### Closed combinations

- 最初のsliceはemail identifier + password credential + opaque email linkだけを受理する。
- passwordはlength unit `unicode-scalar-value`、12以上128以下、whitespace preservationを要求する。
- evidenceは30 minutes、`now < issuedAt + lifetime`、maxUses 1、resend時rotationを要求する。
- signin eligible stateは`Active`、verification eligible stateは`Pending`である。
- failure disclosureとresend disclosureはStage Aで固定したclosed valueである。
- 未対応の複数credential、passwordless、external providerはagentへ渡さずdiagnosticにする。

### Secret boundary

- Credentialを`IRField`やprojectionへ解決しない。
- Credential/evidence schemaにruntime value、hash、token、storage fieldを追加しない。
- interaction inputはnode refだけを持ち、input valueを持たない。
- Identifier canonicalizationとCredential input policyを混同しない。passwordへemail trim/case-foldを適用しない。

## 12. Acceptance Factのsemantic setup

`AcceptanceFact`へoptionalな`setup`とIdentity用payloadを追加する。setupはFact内へinlineし、別Factや共有fixtureを
参照しない。

```go
type FactSetup struct {
    Subjects []FactSubjectSetup
    Evidence []FactEvidenceSetup
    Sessions []FactSessionSetup
    Clock    *FactClockSetup
    Delivery *FactDeliverySetup
}

type FactSubjectSetup struct {
    Handle      string
    Identity    SemanticID
    State       *IRStateValueRef
    Credentials []FactCredentialBindingSetup
}

type FactCredentialBindingSetup struct {
    Handle     string
    Credential SemanticID
    Condition  string
}

type FactEvidenceSetup struct {
    Handle       string
    Verification SemanticID
    Subject      string
    Condition    string
}

type FactSessionSetup struct {
    Handle    string
    Session   SemanticID
    Subject   string
    Condition string
}

type FactClockSetup struct {
    Evidence string
    Relation string
}

type FactDeliverySetup struct {
    Notice    SemanticID
    Condition string
}
```

`Condition`はclosed setである。

| setup | condition |
| --- | --- |
| Credential binding | `satisfies-policy` |
| Evidence | `issued`、`consumed`、`superseded` |
| Session | `active`、`terminated` |
| Clock | `before-expiry`、`at-expiry`、`after-expiry` |
| Delivery | `succeeds`、`fails` |

expiredはtokenへ`expired` flagを直接注入せず、issued evidenceとclock relationから作る。rotation後の古いevidenceは
`superseded`で表す。

### Setup / expectation non-self-fulfillment invariant

compilerはFact kindごとにrequired execution/preconditionとforbidden setup/inputを定義する。単純にsetupと
expectationの値が等しいかだけでは判定しない。拒否Factでは「state不変」が正しいexpectationなので、pre-stateと
post-stateが同じこと自体は違反ではないためである。

canonical membership fixtureの29 Factsに現れる27 kindすべてへcontractを定義する。完全一致はこのfixtureの
regression testで固定する。一般のprogramが27 kindの部分集合だけを生成することは正常であり、汎用validatorは
生成された各kindにcontractが存在することだけを要求する。`navigation`、`access-allowed`、`access-denied`は
既存admin Factでも使うため、Identity payloadを持つ場合だけ次のIdentity contractを適用する。

| Fact kind | required execution / setup | 禁止するsetup / input |
| --- | --- | --- |
| `access-allowed` | 指定principalで対象interaction/pageへのaccessを実行 | access結果の直接注入。anonymous caseではsession setup |
| `identity-inputs` | interactionを観測し、解決済みinput node集合と比較 | setupなし。UI/input結果をfixtureとして注入できない |
| `credential-non-projectable` | Resolved Intentと生成artifactのprojection集合を観測 | setupなし。Credentialをfield/view fixtureとして追加できない |
| `registration-validation-rejected` | 各invalid caseを1回dispatchし、before/afterを観測 | subject、binding、evidence、emissionの作成結果 |
| `secret-input-not-preserved` | invalid submit後の再提示surfaceを観測 | credential valueまたはcredentialを保持済みとするsetup |
| `duplicate-identifier-rejected` | exact/canonical-equivalent subjectをsetupし、registerを1回dispatch | 2人目のsubject、置換binding、新evidence/emissionを結果として事前追加 |
| `registration-created` | 同identifierのsubjectが存在しないfresh scenarioでregisterを1回dispatch | 作成対象のPending subject、credential binding、verification、notice emission |
| `credential-bound` | fresh scenarioでregisterを1回dispatchし、後続authenticationから観測 | 対象subjectのcredential binding |
| `verification-issued` | fresh scenarioでregisterを1回dispatch | 対象subjectのverification evidence |
| `notice-emitted` | fresh scenarioでregisterを1回dispatchし、emission countのbefore/afterを観測 | 対象registration由来のnotice emission |
| `navigation` | source interactionを1回dispatchし、結果navigationを観測 | destinationをcurrent surfaceまたはoperation resultとして事前注入 |
| `operation-at-most-once` | 同じlogical dispatchを2回以上実行し、mutation/emission deltaを観測 | dispatch count 0または1。期待する追加mutation/emissionの事前作成 |
| `verification-accepted` | Pending subject、issued evidence、before-expiryでverifyを1回dispatch | Active subject、consumed evidence |
| `verification-consumed` | issued evidenceでverifyを1回dispatch | consumed evidence |
| `verification-rejected` | invalid/expired/consumed各caseでverifyを1回dispatchし、state before/afterを観測 | operation実行なしでPending stateだけを期待結果に使うsetup |
| `verification-resent` | Pending subjectと既存issued evidence 1件をsetupし、resendを1回dispatchして1→2のtotal countと+1 deltaを観測 | resend結果として生じる新evidence、新emission |
| `verification-rotated` | Pending subjectと以前のissued evidenceでresendを1回dispatch | 以前のevidenceがsuperseded |
| `enumeration-safe-outcome` | unknown/Active/Pendingの3隔離caseでresendを各1回dispatchし、結果を比較 | 1 caseの結果を3 caseへ複製、またはuser-visible outcomeの直接注入 |
| `authentication-ineligible-state` | Pending subject + matching bindingでsigninを1回dispatchし、session before/afterを観測 | active session、またはsignin実行なしのsession absenceだけの観測 |
| `authentication-accepted` | Active subject、matching credential bindingでsigninを1回dispatch | 同subjectのactive session |
| `authentication-rejected` | unknown identifier/non-matching credentialを各1回dispatchし、結果を比較 | active session、generic failureの直接注入、または片方のcase省略 |
| `session-terminated` | 同subjectのactive sessionでsignoutを1回dispatch | terminated session |
| `access-denied` | anonymous principalでprotected surfaceへのaccessを実行 | denial結果の直接注入、または対象resourceを存在させず404相当で代用 |
| `ownership-allowed` | aliceのactive sessionとalice resource bindingでaccess/editを実行 | allow結果やauthorization predicateの直接注入 |
| `ownership-denied` | aliceのactive sessionとbob resource bindingでaccess/editを実行 | denial結果、常時false authorization、またはresource absence |
| `verification-expiry-boundary` | issued evidenceからbefore/at/afterの3隔離clock caseを作りverifyを各1回dispatch | `expired` flag、同じconsumed evidenceのcase間共有、clock case省略 |
| `delivery-failure-separated` | delivery behavior `fails`でregistration/emission後のbefore/afterを観測 | Active subject、delivery済みnotice、failure後stateの直接注入 |

rejection、access-denied、no-change Factでは、setupが否定結果を表すだけでtestを成立させないよう、operation/inputと
pre-state/post-state observationをpayloadへ必須にする。たとえば`verification-rejected`はinvalid/expired/consumed
inputでverify operationを実行し、stateが変わらないことを観測する必要がある。単にPending subjectをsetupしただけの
Factはcanonical validationで拒否する。

このvalidationを`ValidateFactSetup`相当のcompiler invariantとし、全Fact builder testから通す。個別規則の
`expired` flag禁止はこの一般則の一例である。

実装ではIdentity Fact kindから`FactKindContract`へのregistryを持ち、次をstructural testで固定する。

- canonical 29 Factsに現れるdistinct kind集合とregistry key集合が完全一致する。
- contract未定義のkind、Factから到達しない余分なcontract、重複kindを拒否する。
- 新しいIdentity Fact kindを追加しただけではtestが通らず、required executionとforbidden setup/inputのcontract追加を
  必須にする。
- `operation-at-most-once`は`dispatches >= 2`を必須inputとし、単発dispatchをcanonical validationで拒否する。

### Handle rules

- handleはFact内だけで一意であり、Fact間のidentityを持たない。
- subject handleは`subject/<alias>`、credential/evidence/sessionはそのsubject配下にscopeする。
- handleにemail、password、token、hash、session IDなどのruntime/test valueを入れない。
- credential inputはbinding handleを参照し、`matching`または`non-matching` relationを指定する。
- unknown identifierは値ではなく`input/identifier/unknown` handleで表す。
- canonical-equivalent identifierは`input/identifier/canonical-equivalent` relationで表す。

repository固有testはこれらのhandleへ合成値を割り当てる。Forma coreは値生成器、identity library adapter、
fixture runtimeを持たない。

Identity Factでauthenticated principalを表すため、既存`FactPrincipal`も値ではなくsetup handleを参照できるように
する。

```go
type FactPrincipal struct {
    Kind     string
    Roles    []string
    Identity SemanticID
    Subject  string
    Session  string
}
```

`anonymous`ではIdentity、Subject、Sessionを空にし、`authenticated`では3つすべてを要求する。SubjectとSessionは
同じFactのsetup内に存在しなければならない。

## 13. Identity Fact payload

既存`FactInput` / `FactExpectation`へ無関係なfieldを増やし続けず、Identity専用のclosed payloadを入れる。

```go
type FactInput struct {
    // existing admin-flow fields
    Identity *IdentityFactInput
}

type FactExpectation struct {
    // existing admin-flow expectations
    Identity *IdentityFactExpectation
}
```

`IdentityFactInput`が持てるもの:

- operationまたはinteraction node。
- identifier、credential、evidenceのsymbolic handle参照。
- invalid input case、dispatch count、clock relation、delivery result。
- mutation、subject、evidence、emission、sessionのbefore observation。countやstateのdeltaはsetupではなく、
  operation実行前後の観測として表す。

`IdentityFactExpectation`が持てるもの:

- subject countとstate。
- credential bindingの成立／不成立。
- evidenceのpost-operation total countと追加delta（`count` / `added`）、condition、rotation。
- durable notice emissionのpost-operation total countと追加delta（`count` / `added`）。
- sessionの開始／終了。
- user-visible disclosure class。
- page navigation。
- preserve可能なdomain fieldと、preserveしてはならないCredential node。

payload kindはclosed discriminated unionとして検査し、自由な式、命令列、target-specific assertionを入れない。

## 14. 29 Acceptance Facts

Stage Aの番号と1対1に対応するcandidate IDを固定する。表のsetupは値でなく前節のsemantic setupである。

| # | Fact ID | kind | 主なsetup / expected |
| --- | --- | --- | --- |
| 1 | `fact/page/SignUp/identity/register/UserAccount/access/allowed/anonymous` | `access-allowed` | anonymousが表示・submit可能 |
| 2 | `fact/page/SignUp/identity/register/UserAccount/inputs` | `identity-inputs` | name、email identifier、password credential |
| 3 | `fact/identity/UserAccount/credential/password/non-projectable` | `credential-non-projectable` | field/list/detail/search/filter/sort/labelへ現れない |
| 4 | `fact/identity/UserAccount/operation/register/validation/rejected` | `registration-validation-rejected` | invalid name/email/passwordの各caseでsubject/binding/emissionを作らない |
| 5 | `fact/page/SignUp/identity/register/UserAccount/validation/preserve-input` | `secret-input-not-preserved` | name/emailは保持可、Credential nodeは除外 |
| 6 | `fact/identity/UserAccount/operation/register/identifier/duplicate` | `duplicate-identifier-rejected` | exact/canonical equivalentで新規subject等を作らずresend案内 |
| 7 | `fact/identity/UserAccount/operation/register/subject/created` | `registration-created` | User 1件、status Pending |
| 8 | `fact/identity/UserAccount/operation/register/credential/bound` | `credential-bound` | field保存でなくidentity binding、signinから観測可能 |
| 9 | `fact/identity/UserAccount/operation/register/verification/issued` | `verification-issued` | 30分・一度限りのevidence 1件 |
| 10 | `fact/identity/UserAccount/operation/register/notice/emitted` | `notice-emitted` | durable Verification Email emission 1件 |
| 11 | `fact/page/SignUp/identity/register/UserAccount/navigation` | `navigation` | CheckEmailへ進む |
| 12 | `fact/identity/UserAccount/operation/register/at-most-once` | `operation-at-most-once` | 同じdispatch 2回でもsubject/emissionは1件 |
| 13 | `fact/identity/UserAccount/operation/verify/accepted` | `verification-accepted` | Pending + issued + before-expiryでActive |
| 14 | `fact/identity/UserAccount/operation/verify/evidence/consumed` | `verification-consumed` | 成功後evidenceを再利用不可 |
| 15 | `fact/identity/UserAccount/operation/verify/evidence/rejected` | `verification-rejected` | invalid/expired/consumedでstate不変 |
| 16 | `fact/page/VerifyEmail/identity/verify/UserAccount/navigation` | `navigation` | RegistrationCompleteを表示しSignInへ進める |
| 17 | `fact/identity/UserAccount/operation/resend/accepted` | `verification-resent` | Pending + 既存issued evidence 1件から、state不変のままevidence/emissionを各1件追加 |
| 18 | `fact/identity/UserAccount/operation/resend/evidence/rotated` | `verification-rotated` | 以前のunused evidenceをsupersededへ |
| 19 | `fact/page/CheckEmail/identity/resend/UserAccount/disclosure/uniform` | `enumeration-safe-outcome` | unknown/Active/Pendingで同じuser-visible outcome |
| 20 | `fact/identity/UserAccount/operation/resend/at-most-once` | `operation-at-most-once` | 同じdispatch 2回でもemission 1件 |
| 21 | `fact/identity/UserAccount/operation/signin/state/ineligible` | `authentication-ineligible-state` | Pending + matching credentialでもsessionなし |
| 22 | `fact/identity/UserAccount/operation/signin/accepted` | `authentication-accepted` | Active + matching credentialでsession開始 |
| 23 | `fact/identity/UserAccount/operation/signin/rejected/generic` | `authentication-rejected` | unknown identifier/non-matching credentialで同じgeneric failure |
| 24 | `fact/identity/UserAccount/operation/signout/session/terminated` | `session-terminated` | signout後、以前のsessionでprotected access不可 |
| 25 | `fact/identity/UserAccount/ownership/self/access/denied/anonymous` | `access-denied` | anonymousへprofile dataを開示しない |
| 26 | `fact/identity/UserAccount/ownership/self/access/allowed/self` | `ownership-allowed` | alice sessionはaliceの表示・編集可 |
| 27 | `fact/identity/UserAccount/ownership/self/access/denied/other-subject` | `ownership-denied` | alice sessionはbobの表示・編集不可 |
| 28 | `fact/identity/UserAccount/operation/verify/expiry/boundary` | `verification-expiry-boundary` | beforeは成功、at/afterはexpired |
| 29 | `fact/identity/UserAccount/verification/email/notice/delivery/failure` | `delivery-failure-separated` | emissionはdurable、UserはPending、retry/resend可能 |

Fact 4、15、19、23、28はclosedなcase配列を持つ1 Factである。caseの種類もcompilerが導出し、agentが減らせない。
1つのrepository testへまとめるか複数testへ分けるかはtarget側が決める。

Facts 25〜27はownership predicateをsubjectとし、ProfileとProfileEditのinteraction/viewを
`sourceNodes`とexpectationへ含める。これにより表示だけを守って編集を開放する実装、またはその逆を同じFact群で
拒否する。

Fact 8は同じ隔離scenario内でregister後にverificationとsigninを行い、credential bindingをauthenticationの
成功／失敗から観測してよい。これは1つのtest scenario内の観測stepであり、Fact 13や22のcoverage結果を前提にする
`dependsOn`ではない。Fact payloadは観測先としてsignin operationを参照するが、自由なstatement列は持たない。

## 15. Source nodesとfact derivation

各Factは少なくともsubjectに加え、意味の根拠となるnodeを`sourceNodes`へ持つ。

例:

```text
credential-non-projectable
  identity/UserAccount/credential/password

registration-created
  identity/UserAccount/operation/register
  entity/User
  entity/User/state/status

verification-accepted
  identity/UserAccount/verification/email
  identity/UserAccount/operation/verify
  action/User/activate

ownership-denied
  identity/UserAccount/ownership/self
  identity/UserAccount/session/current
  page/Profile
  page/ProfileEdit
```

fact IDとpayloadはResolved Intentだけから決定的に再導出する。Source Map、repository、agent feedbackは導出入力に
しない。

## 16. Review Requirements

Acceptance Factsと別に次のschemaを導入する。

```go
type ReviewRequirements struct {
    Version       string
    IntentVersion string
    Requirements  []ReviewRequirement
}

type ReviewRequirement struct {
    ID          SemanticID
    Kind        string
    Subject     SemanticID
    SourceNodes []SemanticID
    Instruction string
}
```

最初のIdentityから必ず次の3件を導出する。

| ID | kind | 人間が確認すること |
| --- | --- | --- |
| `review/identity/UserAccount/secret-redaction` | `secret-redaction` | feedback、user-visible diagnostic、repository logへruntime credential/evidence valueがない |
| `review/identity/UserAccount/secret-storage` | `secret-storage` | credential/evidenceを平文domain dataとして保存せずrepository標準の安全な方式を使う |
| `review/identity/UserAccount/fixture-fidelity` | `fixture-fidelity` | agentがsemantic setupをrepository固有testへ変換する際、実operation・認可・観測経路をstubや直接注入で迂回していない |

`Instruction`はcompiler所有の固定文であり、agentが書き換える自由記述ではない。requirementsはID順にsortする。

### verifyの扱い

- `ValidateRequest`はResolved Intentからrequirementsを再導出し、request内の集合と完全一致させる。
- requirementsを`FactCoverage`や`passed` countへ含めない。
- Generation Feedbackにreview statusを追加しない。
- `forma verify`はmachine-verifiable Factsが成功しても3件を必ず表示する。
- CLI exit 0は機械検査が通ったことだけを意味し、orchestration layerは人間review前に作業全体をcompleteとしない。

人間の確認結果を将来記録する場合も、agent feedbackではなく別のreview attestationとして設計する。このsliceでは
attestation formatを決めない。

## 17. Generation Request v0alpha3 → v0alpha4

B3で採用した`v0alpha3`へ、B4でreview diffと完全なbaseline version metadataを追加した。current shapeは次である。

```text
GenerationRequest
  schema: forma/generation-request/v0alpha4
  resolvedIntent
  acceptanceFacts
  reviewRequirements
  sourceMap
  implementationPolicy
  requestedChange
    baseline
      requestSchema
      resolvedIntentVersion
      acceptanceFactsVersion
      sourceMapVersion
      reviewRequirementsVersion
    intentChanges
    factChanges
    reviewRequirementChanges
    unchangedIntentNodes
    unchangedFacts
    unchangedReviewRequirements
  verification
    requiredFactIds
    displayReviewRequirementIds
```

`displayReviewRequirementIds`はagentへ完了報告させるためではなく、orchestration UIが必ず表示する集合の複製である。
検証の正本はResolved Intentから再導出したReview Requirementsとする。

B3の`v0alpha3` builderはReview Requirementsが変わるincremental requestを明示的に拒否していた。
B4のcurrent `v0alpha4`はreview requirementのadded/changed/removedも明示し、Identity追加では3件をaddedにする。

B4の`RequestBaseline`には既存のrequest、Resolved Intent、Acceptance Facts versionに加え、Source Map versionと
Review Requirements versionを記録する。historical `v0alpha2` baselineにはReview Requirementsが存在しないため、
upgrade時は明示的な`none`として扱い、current versionを偽装して埋めない。

## 18. 旧baselineからのversion migration

Stage Dは、すでにtargetへ適用済みのadmin `v0alpha2` requestをbaselineとしてIdentityを追加する。新binaryが
current versionだけを受理すると、このlineageが切れる。

次の境界を実装する。

1. historical requestはそのschema versionに対応するvalidator/fact builderでcanonicalに検証する。
2. baseline digestはhistorical schema専用codecで得たcanonical bytesから計算し、current structのzero fieldを
   足して書き換えない。
3. diff前にhistorical Resolved Intent / Factsをcurrent in-memory shapeへlosslessにupgradeする。
4. admin flowにIdentityがなければ、upgradeだけでsemantic nodeやFactのadded/changedを発生させない。
5. current `v0alpha4` Identity requestとの差分だけを`intentChanges`、`factChanges`、`reviewRequirementChanges`へ記録する。
6. `ValidateIncrementalBaseline`も同じupgradeとdiffを再実行する。

単にversion文字列をcurrentへ置換すること、baseline requestを新compilerで作り直してlineageを付け替えることは
認めない。upgradeできないhistorical shapeは明示的に拒否する。

このmigrationはgeneralな永続schema migrationではない。Formaの交換形式間のpure、deterministicなupgradeである。

## 19. canonicalization

- Identity、Identifier、Credential、Verification、Ownership、interactionはID順。
- `sourceNodes`は重複除去してID順。
- semantic setupのsubjectsはhandle順、子handleは親subject配下でsortする。
- case配列はspecで定めたclosed order、集合はID順。
- `atomicOutcome`のような集合は辞書順。
- identifier canonicalizationだけは適用順に意味があるためsource順を保持する。
- durationは整数amount + closed unitで表し、target依存のduration stringへしない。
- compilerはclockをsampleしない。Fact setupがexpiryとの相対関係を持つ。

同じForma sourceからはbyte-identicalなResolved Intent、Facts、Review Requirements、Source Mapを出力する。

## 20. target vocabularyを入れない

候補schemaとgolden JSONに次を含めない。

- route、path、HTTP method/status、cookie、header、DOM、component。
- SQL table/column、transaction API、ORM、identity library。
- bcrypt/Argon2などhash方式、token entropy、UUID、database sequence。
- SMTP/SES/SendGridなどdelivery provider。
- test runner、fixture adapter、mock framework。
- `preventDuplicateDispatch`、`recheckAccess`など実現対策名。

at-most-once、access enforcement、durable emissionはobservable Factとして表す。

## 21. 実装順序

### B1 — schemaとcanonical fixture

- [x] `IRIdentity`と関連node、Identity access requirement、interaction refをGo typeへ追加した。
- [x] parserを変更せず、test-onlyの`membershipIntentFixture`からcanonical JSONを生成した。
- [x] Resolved Intentを`v0.5`、Source Mapを`v0.3`へ上げ、既存admin goldenの意味を維持した。
- [x] stable ID、sort、duplicate ID、reference integrity、Source Map 1対1 coverageをtestした。

### B2 — 29 Facts

- [x] Fact-local semantic setupとIdentity payloadを追加した。
- [x] `membershipIntentFixture`からIdentity専用29件を完全導出した。
- [x] Fact ID、kind、setup handle、sourceNodesのgoldenを固定した。
- [x] credential nodeが`preserveInput`と`stored: "input"`へ入らないnegative testを追加した。
- [x] Fact schemaにcredential/evidence raw value用fieldがないことをstructural testで固定した。
- [x] Fact kindごとのrequired execution/setupとforbidden setup/inputを検査し、self-fulfilling setupをnegative testで
  拒否した。
- [x] canonical Factの27 kindと`FactKindContract` registry key集合の完全一致をfixture testで固定し、汎用validatorは
  contractを持つkindの部分集合を受理するようにした。

### B3 — 3 Review Requirements

- [x] `forma/review-requirements/v0alpha1`のcanonical builderとschemaを追加した。
- [x] current `v0alpha3` requestへ埋め込み、request validationでResolved Intentから再導出する。
- [x] feedbackへreview coverageを追加せず、Identity requestのCLI成功出力へ3件を必ず表示する。
- [x] historical `v0alpha1` / `v0alpha2` requestはIdentityを含まない場合だけReview Requirementsなしで受理する。
- [x] B3ではbuilderと`ValidateIncrementalBaseline`の両方がReview Requirementの変化を拒否する。

### B4 — Generation Requestとhistorical baseline

- [x] `v0alpha4` incremental requestへreview requirement diffとbaseline version metadataを追加した。
- [x] historical `v0alpha1` / `v0alpha2`のschema専用codec、version-dispatched validation、lossless upgradeを実装した。
- [x] 実際に適用されたadmin `v0alpha2` Git blob `5751ecf85e9b7be2665aa91854ee5b69798e81a3`から、admin semanticsを
  保ったIdentity追加requestへのpairwise lineageをtestした。
- [x] historical bytesのidentity、43既存Factsのunchanged、38 Factsと3 Review Requirementsのaddedを固定した。

### B5 — proposal gate

- passwordless、external provider、email変更の3つをGo fixtureだけで試す。
- schemaを壊さず表現できない部分を記録し、未対応組合せはdiagnostic対象にする。
- このgateを通るまでsurface syntaxを追加しない。

## 22. Stage B exit criteria

- target repositoryやForma syntaxなしでIdentity semanticsをcanonical JSONへ表せる。
- Identity graphの全nodeがstable IDを持ち、Source Map kindが決まっている。
- Stage Aの29 Factsがちょうど1回ずつ決定的に導出される。
- 各Factは値を含まないsetupだけで独立実行でき、`dependsOn`を持たない。
- compilerがsetupとexpectationのpre/post contractを検査し、self-fulfilling setupを拒否できる。
- credential/evidence valueをResolved Intent、Source Map、Facts、Requestへ格納できない。
- Credential nodeがprojection、`preserveInput`、`stored: "input"`へ現れない。
- 3 Review RequirementsがFactsと分離され、`forma verify`から隠れない。
- existing admin semanticsを変えず、新versionのgoldenへ移行できる。
- historical admin requestからIdentity incremental requestへのlineageを検証できる。
- target/framework vocabularyを含まない。
- unsupportedなIdentity組合せをagentへ渡さず拒否できる。

## 23. Stage Cへ残す問い

- `identity` declarationとnested clauseの最小syntax。
- register/verify/resend/signin/signout interactionをpageへどう記述するか。
- Credential policyをinlineにするかnamed policyにするか。
- anonymous、authenticated、selfを`allow`へどう統合するか。
- verification noticeをIdentity syntaxへ置くか、採用後のEffect syntaxを参照するか。
- passwordless/external provider比較からIdentifiers/Credentialsの複数性をどこまで有効化するか。

これらはStage BのResolved Intentを一意に生成できるかで評価し、syntaxの短さだけで決めない。
