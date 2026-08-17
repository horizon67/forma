# Email-verified Membership Flow Probe

Status: active P1 probe — Stage B1–B5 complete; minimal Stage C syntax next

Stage Bで検討するtarget-neutral node、29 Factsのcandidate ID、semantic setup、Review Requirements、version境界は
[`identity-semantic-model-proposal.md`](identity-semantic-model-proposal.md)に記録する。

## 1. 目的

このprobeは、Formaが単一画面のCRUDだけでなく、メール認証を含む段階的な会員登録flowをcoding agentへ
曖昧さなく伝え、通常のrepositoryへ正しく実装させられるかを検証する。

対象は次の一連のobservable behaviorである。

```text
会員情報入力
  → Pending User作成
  → verification発行
  → 認証メールemission
  → メール確認案内
  → URLからverificationを検証
  → UserをActiveへ変更
  → 登録完了
  → signin
  → 本人profile
```

このflowを自然言語の補足promptだけでagentへ渡してはならない。必要な意味はForma sourceから検査済みの
Resolved IntentとAcceptance Factsへ到達しなければ、このprobeの成功とはみなさない。

一般的なIdentityの設計候補と比較例は
[`public-membership-proposal.md`](public-membership-proposal.md)に記録する。本書はそのうち最初に実装・実測する
具体的なvertical sliceを固定する。

## 2. 最初のsliceで固定する選択

選択肢をagentへ残さないため、最初のsliceは次の意味に固定する。

- identifierは`User.email`。
- emailのidentifier/unique比較では、前後空白とASCII大小文字だけが異なる値を同一と扱う。保存表現は固定しない。
- credentialはpassword。通常のentity fieldとは別に扱う。
- passwordは前後空白を含む入力をそのまま扱い、Unicode scalar valueで12文字以上128文字以下とする。
- signup成功時のdomain stateは`Pending`。
- email verificationはメール内URLから行う。
- verification evidenceは発行後30分で失効し、一度だけ利用できる。
- expiry境界は`now < issuedAt + 30 minutes`なら有効、同時刻以降はexpiredとする。
- 再送はUserのstateを変えず、新しいverificationを発行する。
- 再送後は以前の未使用verificationも無効になる。
- `Pending` Userはsigninできず、`Active` Userだけがsigninできる。
- verification成功時に`Pending -> Active`を適用する。
- verification成功後は登録完了画面を表示し、signin画面へ進む。自動signinはしない。
- 途中離脱したUserは`Pending`のまま残す。期限切れ後も再送から継続できる。
- 同じemailで再度signupしても新しいUserやverificationを作らず、再送flowを案内する。
- 再送要求は、emailに対応するUserが存在するかを画面上の応答から判別できない。
- signoutは現在のauthenticated sessionを終了する。
- authenticated Userは自分のprofileだけを表示・編集できる。
- 最初のpublic profileはname、nickname、email、statusを表示し、nameとnicknameだけを編集できる。email変更は
  後続のreverification probeへ残す。

verification evidenceのformat、token storage、password hash、session transportなどはここでは固定しない。

## 3. 正常系の段階flow

| 段階 | 利用者の操作 | 成立する意味 | 次のsurface |
| --- | --- | --- | --- |
| 1 | SignUpを表示 | anonymous principalに登録入力を提示する | SignUp |
| 2 | name、email、passwordをsubmit | 値とcredential policyを検査する | validation failureまたは登録処理 |
| 3 | 正しい入力を登録 | `Pending` Userとcredentialを1 atomic boundaryで作る | verification発行 |
| 4 | verificationを発行 | 30分・一度限りのevidenceを作る | email emission |
| 5 | 認証メールを発生させる | Verification Email emissionを1件記録する | CheckEmail |
| 6 | メール内URLを開く | evidenceを検査する | failureまたはverification成功 |
| 7 | verification成功 | evidenceを消費し、`Pending -> Active`を1回適用する | RegistrationComplete |
| 8 | signinへ進む | identifierとcredentialを提示する | SignIn |
| 9 | Active Userがsignin | authenticated principalを開始する | Profile |
| 10 | profileを操作 | principalと同じUserだけを許可する | Profile |

User作成、credential保存、verification発行、Verification Email emissionの記録は、どれかが失敗した場合に
中途半端な登録を残さない同じatomic outcomeとする。coding agentはtransactional outboxなどrepositoryに合う
方法でこの意味を実現してよい。一方、外部providerへの配送はrollbackできないため、このboundaryの外にある。

## 4. 例外系の意味

| case | 期待する結果 | 発生してはいけないこと |
| --- | --- | --- |
| name/email/passwordがinvalid | formをinvalidとして再提示する | User作成、credential保存、email emission |
| emailが既存またはcanonical equivalent | signupを拒否し再送flowを案内する | 2人目のUser、既存credentialの上書き |
| 登録またはemission記録が失敗 | failureを提示する | 部分的なUser、credential、verification、emission |
| 外部email deliveryが失敗 | UserはPendingのまま、delivery retryまたは再送を可能にする | Active化、登録のrollback |
| verificationが不正 | invalidとして拒否する | state変更、session開始 |
| verificationが期限切れ | expiredとして拒否し再送を案内する | state変更、evidence再利用 |
| verificationが使用済み | consumedとして拒否する | 2回目のactivation |
| verification再送 | 新しいevidenceとemail emissionを発生させる | User state変更、古いevidenceの継続利用 |
| 未知またはActive emailへの再送 | 既知のPending emailと同じ画面応答を返す | User存在の開示、不要なstate変更 |
| Pending Userのsignin | genericな認証失敗として拒否する | session開始、未認証理由の過剰な開示 |
| passwordが不正 | genericな認証失敗として拒否する | session開始 |
| anonymousのprofile access | 拒否する | profile dataの開示 |
| 他Userのprofile access/edit | 拒否する | dataの開示または変更 |
| 登録途中で離脱 | Pending Userを維持し、verificationを期限切れにできる | 自動Active化 |

rate limit、CAPTCHA、lockoutなどのabuse preventionは重要だが、最初のsliceではrepository policyと後続probeへ
残す。ただしunknown emailへの再送応答を同一にするuser-enumeration境界だけはobservable behaviorとして含める。

## 5. Formaとcoding agentの責任境界

| Formaが決めること | coding agent / repositoryが決めること |
| --- | --- |
| identifierがemailである | normalized emailの保存・indexの具体方式 |
| passwordはsecret credentialである | hash algorithm、cost、identity library |
| password length policy | target固有validation APIと表示方法 |
| signupでPending Userを作る | transaction、schema、repository method |
| evidenceは30分・一度限り | token format、entropy、hash、storage |
| resendで古いevidenceを無効化する | rotate/update/deleteの具体処理 |
| Verification Email emissionを要求する | outbox、SMTP、SES、SendGrid、fake mail adapter |
| Pendingはsignin不可、Activeは可 | session store、cookie、middleware |
| verification成功でActiveになる | route、handler、database update |
| selfだけprofileを操作できる | authorization middlewareやqueryの実装 |
| 正常系・拒否系Acceptance Facts | repository固有testと観測点 |

Formaが保証するのは外部providerへの最終配送ではなく、application境界でVerification Email emissionが
durableに記録された事実までである。agent E2Eではfake mail adapterからemissionとverification URLを観測し、
URLを開いて登録完了まで検査する。

## 6. 最小Acceptance Facts

IDとJSON shapeは未決定だが、少なくとも次の事実をGeneration Requestから欠落させてはならない。

### Registration

1. anonymous principalがSignUpを表示・submitできる。
2. SignUpはname、email、passwordを入力として要求する。
3. passwordはentity field、list、detail、search、filter、labelへ現れない。
4. invalid name/email/passwordは保存を拒否し、Userとemail emissionを作らない。
5. validation再表示ではnameとemailを保持してよいが、password valueを返さない。
6. duplicateまたはcanonical equivalentなemailは新しいUser、credential、verification、email emissionを作らない。
7. valid signupはUserを`Pending`でちょうど1件作る。
8. valid signupはcredentialを通常fieldへ保存せずUserへidentity bindingとして関連付け、後のauthenticationで
   検証できる。
9. valid signupは30分・一度限りのverificationを1件発行する。
10. valid signupはVerification Email emissionを1件発生させる。
11. valid signup後はCheckEmailへ遷移する。
12. 同じ論理signup dispatchはUserとemissionを重複適用しない。

### Verification and resend

13. 正しい未使用・期限内evidenceだけが`Pending -> Active`を成功させる。
14. verification成功はevidenceを消費し、同じevidenceを再利用できない。
15. 不正・期限切れ・使用済みevidenceはUser stateを変更しない。
16. verification成功後はRegistrationCompleteを表示し、SignInへ進める。
17. Pending Userへの再送はstateを変更せず、新しいverificationとemail emissionを発生させる。
18. 再送後は以前の未使用evidenceも無効になる。
19. 未知・Active・Pending emailへの再送は同じuser-visible outcomeを返す。
20. 同じ論理resend dispatchはemissionを重複させない。

### Authentication and ownership

21. Pending Userは正しいpasswordでもsigninできない。
22. Active Userは正しいidentifierとcredentialでsigninできる。
23. 不正なidentifier/credentialはsessionを開始せず、同じgeneric failureを返す。
24. signout後は以前のsessionでprotected pageへaccessできない。
25. anonymous principalはProfileを表示・編集できない。
26. authenticated principalは自分のUser profileを表示・編集できる。
27. authenticated principalは別Userのprofileを表示・編集できない。

### Time and delivery boundary

28. 30分の境界をagent E2Eから注入可能なclockで検査できる。
29. 外部email deliveryが失敗してもUserをActiveにせず、delivery retryまたは再送可能なfailureとして観測できる。

Fact数を減らすために複数caseを1 testへまとめてもよいが、required fact ID集合は縮めない。

### Factの独立実行とsemantic fixture

Fact 22以降は、credentialが確立済みのUserやauthenticated principalを前提にする。しかし、それをFact 7・8の
成功結果へ依存させない。各Factは新しい隔離されたscenarioから、単独・任意順・反復可能に実行できなければ
ならない。

Stage Bでは、通常のentity record fixtureとは別に、Factのpreconditionを表すtarget-neutralなsemantic fixture
またはsetup intentを設計する。少なくとも次の初期状態を作れる必要がある。

- validなtest credentialがidentityへbindingされた`Pending` User。
- 上記Userをverification済みにした`Active` User。
- ownershipの正常系・拒否系を検査する、別identityを持つ2人の`Active` User。
- 未使用、期限切れ、使用済み、rotation前後のverification evidence。
- authenticated sessionと、signout済みの同じsession。

fixtureはcredentialやevidenceの値そのものではなく、`subject/alice/credential/primary`、
`subject/bob/credential/primary`、`subject/alice/evidence/current`のようなsubject-scopedな
**symbolic handleと成立すべき状態**を運ぶ。Generation Request、Resolved Intent、Source Mapへ平文passwordや
verification tokenを埋め込まない。repository固有testがpassword policyを満たす合成test値を選び、identity
library、signup flow、test factoryなどrepositoryに合う方法でpreconditionを確立する。verification evidenceは
fake mail adapterなどの観測点から取得し、contractへ固定値として書かない。

合成test credentialはproductionで発生したruntime secretではない。target test codeが既知のtest値を持つことと、
Formaの交換形式やGeneration Feedbackがruntime secretを保持することを区別する。compilerが導出するsetupは
expectationのoutcomeを事前に成立させてはならず、repository固有testへの変換も操作対象そのものを迂回しては
ならない。たとえばsignin FactはActive Userをsetupしてよいが、active sessionやsignin成功判定を直接注入しては
ならない。

semantic fixtureの確立に失敗した場合は、そのFactを失敗とする。他Factの失敗結果を引き継ぐ`dependsOn` graph、
実行順序、部分成功という意味は最初のsliceへ導入しない。Forma coreはframework別fixture adapterを持たず、coding
agentがこのpreconditionをrepository固有test fixtureへ変換する。

### Acceptance Factsとは別に検査するsecurity boundary

security boundaryには、上の29 Acceptance Factsと同じ`passed`集合へ入れてはいけない2種類の保証がある。

#### Forma front-endが構造的に保証するinvariant

- credentialとverification evidenceのruntime valueをResolved IntentとSource Mapへ格納するfieldを持たない。
- Credential nodeをentity field、list、detail、search、filter、sort、labelへ解決できない。
- credential由来inputのnode IDを、validation factの`preserveInput`へ含めない。
- credentialを含むregistrationへ、すべてのinputをそのまま保存する`stored: "input"`を導出しない。
- domain fieldの保存期待とcredential bindingの期待を分け、credentialの正しさは後のauthentication成功／失敗で
  観測する。
- compilerが導出するsemantic setupはFact kindごとのpre/post contractを満たし、expectationのoutcomeをsetup時点で
  成立させない。

これらはagentのcoverage報告ではなく、Forma compiler自身のnegative testで保証する。

#### 人間のreviewを必須にするobligation

- Generation Feedback、user-visible diagnostic、repository logへcredential/evidence valueを出さない。
- credentialとverification evidenceを平文のdomain dataとして保存せず、repository標準の安全な方式を使う。
- agentがsemantic fixtureをrepository固有testへ変換する際、実operation・認可・観測経路をstubや直接注入で
  迂回していない。

Formaは実行時のsecret valueやrepository固有testのsetup codeを保持しないため、これら3件を再計算して
`passed`とは判定できない。Stage Bでは
Acceptance Factとは別のstable review requirementとしてGeneration Requestへ載せ、`forma verify`が
「人間のreviewが必要」と必ず表示する最小modelを設計する。agentの自己申告だけで表示を消してはならない。

## 7. 現行Forma v0で書ける部分（実測）

[`../examples/public-membership.forma`](../examples/public-membership.forma)は、現行v0だけで書けるdomainと
presentationのsubsetである。

```forma
type Email = String matches /.+@.+/

entity User {
    name  String required label
    email Email required unique

    state status Pending | Active | Suspended initial Pending
}

action User.activate: Pending -> Active

page SignUp {
    form User {
        fields name, email
        submit create
    }
}
```

2026-08-17時点のreference front-endで`forma check`は成功し、11 Acceptance Factsを導出した。

- anonymousによるSignUp表示とsubmit
- name/email fieldの提示
- create mutationの成功とat-most-once
- required、email matches、email uniqueの拒否case
- invalid/failure feedback

一方、create成功後のnavigationは`same-context`でSignUpへ戻る。標準`create`へ`goto`を付けられないため、
CheckEmailへの遷移は表現できない。`action User.activate`は宣言できるが、verification成功と結び付ける場所はなく、
このactionを実行するAcceptance Factも導出されない。

## 8. 現行front-endが拒否または誤って受理する境界（実測）

候補記述をcompilerへ実際に与えた結果を記録する。

| 書きたかったこと | 実測結果 | 意味 |
| --- | --- | --- |
| `identity UserAccount for User` | `F1001: unknown declaration identity` | Identityを宣言するnodeがない |
| `page VerifyEmail { verify User }` | `F1001: unknown page member verify` | verification surfaceがない |
| `effect VerificationEmail` | `F1001: unknown declaration effect` | email emissionを宣言できない |
| `action User.resendVerification: Pending -> Pending` | `F2301: transition source and destination must differ` | stateを変えないresend actionがない |
| `action User.resendVerification` | `F1002: expected : after the action name` | transitionなしactionを宣言できない |
| `allow self` | `F2003: unknown role self` | principalとresourceのownershipがない |

さらに、`password String required`をUser fieldとして書くと**errorなしで受理される**。Resolved Intentには
`entity/User/field/password`として入り、create formの通常fieldにも投影される。これはcompiler bugというより、
現行languageにcredentialとfieldを区別するsemanticsがないことの実測である。P1ではpassword fieldを許可する
方向ではなく、読み出し不能なCredential Intentを追加する。

Generation Requestではさらに、required/matches/unique違反の4 Factsがpassword field IDを`preserveInput`へ
含め、create成功Factが`stored: "input"`を要求する。したがって現行規則のままpasswordをfieldへ置くと、
passwordを再表示せずhashだけを保存する安全な実装が不合格になり、平文を再表示・保存する実装だけが要求へ
一致してしまう。Stage BはCredential Intentを追加するだけでなく、**Acceptance Factの導出規則そのものを
credential-awareにする**必要がある。

login、signout、session、verification expiry、single-use evidence、clock、email emission failureにも宣言場所が
ない。これらをagentがrepositoryから推測するだけでは、Forma sourceがapplication behaviorの正本にならない。

## 9. このprobeから必要になったsemantic axis

名称とprimitive分類は未決定だが、最小sliceには次の独立した意味が必要である。

- Identity — subject entity、identifier、authenticated principal。
- Credential — secret input、policy、verify/updateのみ可能な境界。
- Registration — entity create、credential binding、initial state、verification issuanceのatomic boundary。
- Verification — evidence kind、TTL、single use、rotation、success transition。
- Authentication — signin eligibility、session開始、signout。
- Ownership — principalとentity identityの一致を使うauthorization predicate。
- Occurrence / Effect binding — registration/resendからVerification Email emissionを発生させる。
- Clock boundary — expiryを決定的に検査するためのtarget-neutral time semantics。
- Navigation — CheckEmail、RegistrationComplete、SignIn、Profileへの結果遷移。

Identity固有の安全性を、汎用Effectや自由なAction bodyだけへ分解しない。一方、Verification Email emissionと
状態を変えないresendは注文通知probeにも現れているため、共通部分は
[`order-approval-proposal.md`](order-approval-proposal.md)のOccurrence / Effect modelと後から合流させる。

## 10. 実装を段階化する

### Stage A — flow contract（本書）

- flow、例外時の結果、責任境界、最小Facts、非目標を固定する。
- 現行v0の受理・拒否境界を実測する。
- syntaxを決めない。

### Stage B — target-neutral semantic model（B1–B5 complete）

具体的なcandidate shapeは
[`identity-semantic-model-proposal.md`](identity-semantic-model-proposal.md)で設計する。

- [x] Identity / Credential / Verification / Authentication / Ownershipの最小Resolved Intent shapeを作る。
- [x] 各nodeへstable semantic IDとSource Map entryを与える。
- [x] 上の29 factsを正常系・拒否系として機械的に導出した。
- [x] credential、verification、sessionを必要とするFactへ、値を含まないsymbolic handleとsemantic setupを導出した。
- [x] 各Factを新しいscenarioから独立実行可能にし、Fact間の`dependsOn` graphを導入していない。
- [x] `preserveInput`からCredential nodeを除外し、registrationの保存期待をdomain fieldとcredential bindingへ分けた。
- [x] Credentialを含むFactに`stored: "input"`が現れないことをnegative testで固定した。
- [x] Acceptance Fact schemaに平文credential/evidenceを格納できるfixture fieldがないことを構造testで固定した。
- [x] secret valueを保持できるfieldがResolved Intent / Source Map schemaにないことを構造testで固定した。
- [x] Fact kindごとのpre/post contractでself-fulfillingなsemantic setupをcompilerが拒否する。
- [x] canonical membership fixtureの27 kindとpre/post contract registryをfixture testで完全一致させ、汎用validatorは
  規則未定義の生成kindを拒否しつつ、定義済みkindの部分集合を受理する。
- [x] 再計算できないsecret redaction、storage、repository fixture fidelityを、Factとは別のstable review requirementsとして
  出力した。
- [x] `forma verify`が未解決のreview requirementsを必ず人間へ表示し、feedbackの`passed`へ吸収しない境界を実装した。
- [x] Review Requirement diffと全compiler artifact versionを持つGeneration Request `v0alpha4`を実装した。
- [x] 実際に適用したadmin `v0alpha2` requestのbyte identityを保ち、43既存FactsをunchangedとしてIdentity追加requestへの
  pairwise lineageを検証した。
- [x] passwordless、external provider、email変更をtest-only fixtureで比較し、共通化に必要なproof、external authority、
  identifier binding lifecycleを[`identity-variant-probe.md`](identity-variant-probe.md)へ記録した。

### Stage C — experimental Forma syntax

- Stage Bの意味を一意に作れる最小surface syntaxをParserとCheckerへ追加する。
- [x] passwordless / external provider / email変更の比較fixtureへ同じmodelを当て、現行`v0.5`の対応範囲と不足axisを確認した。
- passwordをIdentityの構造上の必須節にせず、current sliceではlocal-password proofだけを意味検査で受理する。
- 将来のverification-evidence proof、external authority、identifier changeを既存syntaxへadditiveに追加できることを
  [`identity-semantic-model-proposal.md`](identity-semantic-model-proposal.md) §23の設計制約として守る。
- unsupportedな組合せはagentへ渡さずcompile errorにする。

### Stage D — agent E2E

- 現在のadmin targetをbaselineとして固定し、public membership flowをincremental requestで追加する。
- fake mail adapter、injectable clock、session、identity storageはrepository-nativeにagentが実装する。
- 既存admin 43 Factsと新しい29 Identity Factsを含む全required factsを維持する。
- root/targetのbuild/testと`forma verify --baseline`を通す。
- credential/evidenceのreview requirementsをCLIへ表示し、人間がstorage、feedback、diagnostic、logを確認する。

## 11. 非目標

最初のsliceには次を含めない。

- 実SMTP/SES/SendGridへの配送成功の保証。
- SMS code、magic link signin、passkey、OAuth/OIDC。
- password reset、email変更、account recovery。
- 複数device session、logout-all、session rotationの一般model。
- pending accountのschedule削除。
- rate limit、CAPTCHA、IP reputationなどabuse preventionの一般言語化。
- email copy、HTML template、localization。
- Identity providerやlibraryをForma coreが選ぶこと。

これらを暗黙に実装してよいという意味ではない。必要なproductではForma source、Implementation Policy Manifest、
または明示的な後続contractへ追加する。

## 12. Exit criteria

- flowの各段階と例外結果をtarget repositoryなしで説明できる。
- passwordとverificationを通常fieldへ縮退させない。
- register、verify、resend、signin、signout、self accessからtarget-neutral factsを導出できる。
- invalid/expired/reused evidenceとPending signinを明示的に拒否できる。
- emissionと外部deliveryを分離できる。
- clock、secret、principalの境界をframework vocabularyなしで表せる。
- Credential nodeが`preserveInput`や`stored: "input"`へ入らないことをcompilerが構造的に保証する。
- credential/evidenceを値ではなくsymbolic handleで参照し、各Factを他Factの結果へ依存せず実行できる。
- 再計算できないsecret handlingをFactの成功数へ混ぜず、review requirementとして可視化する。
- coding agentが既存repositoryを保持したままflowを実装し、全required factsを通せる。
- testが通ってもsecret漏洩やownership欠落があれば成功とみなさない。
