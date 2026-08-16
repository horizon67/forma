# Public Membership and Identity Proposal

Status: exploratory proposal — not valid v0 syntax and not a language decision

この文書は、一般利用者が自分で会員登録、メール認証、loginを行う表側applicationをFormaでどう表すか
というdesign probeである。現在の[`examples/users.forma`](../examples/users.forma)は管理者向けの
ユーザー管理を表し、認証方式、credential、verification、sessionは扱わない。

規範仕様は[`v0-primitives.md`](v0-primitives.md)である。ここに示す`identity`、`register`、`verify`、
`login`などは現在のParserでは受理されず、primitive、modifier、Resolved Intent nodeのいずれにするかも未決定である。

## 対象とするflow

1. name、email、passwordなどを受け取りvalidationする。
2. 会員を未認証状態で作成し、credentialを安全に保存する。
3. verificationを発行して認証メールを送る。
4. linkまたはcodeを検証し、会員を有効化する。
5. identifierとcredentialで本人確認し、authenticated sessionを開始する。

このflowには、domain data、identity semantics、実装architectureという異なる責務が含まれる。

## 現行v0で表現できる範囲

| flow | v0で表現できること | 不足しているsemantic axis |
| --- | --- | --- |
| 会員情報のvalidation | `required`、named type、`matches`、`unique` | credential専用constraint |
| 会員作成 | create form、初期state | register operationとcredential保存 |
| 認証メール | なし | verificationとnotification effect |
| メール認証 | state transitionだけ | token/code入力、検証、transitionとの接続 |
| login | なし | identity、credential、principal、session |
| 本人向けpage | static roleだけ | authenticated、`self`、ownership認可 |

v0だけなら、次のdomain部分までは書ける。

```forma
type Email = String matches /.+@.+/

entity User {
    name  String required
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

このsourceはpassword、verification、loginを宣言していないため、完全な会員登録applicationとは
みなさない。coding agentが未宣言の認証flowを推測で追加してはならない。

## 責務の分離

### Membership domain — Forma source

- 会員のnameとemail
- emailの一意性
- `Pending`、`Active`、`Suspended`などのmembership lifecycle
- verification成功後に許されるdomain transition
- Activeな会員だけloginできる、など利用者から観測できるrule
- authenticated principalへ与えるroleとownership rule

### Identity semantics — Formaに必要な新しい層

- login identifier
- entity fieldとは異なるcredential
- register、verify、login、logout、recovery
- verification requirement
- authenticated principal
- `self`またはownership authorization
- credentialとverificationに必要なuser-visible constraint

### Identity implementation — Coding agent / Repository

- credentialを保護して保存する具体方式
- verification token/codeのformatとstorage
- mail delivery provider
- sessionのtransportとstorage
- identity libraryまたはexternal identity provider
- target固有のroute、controller、database schema

implementation detailをFormaへ書かない一方、「verificationが必須」「Activeでなければloginできない」
などのobservable semanticsをrepository実装へ隠してはならない。coding agentは宣言済みのIdentity
Intentを、repositoryのarchitectureへ合わせて実装する。

## passwordをentity fieldにしない

次のmodelは採らない。

```forma
entity User {
    password String required
}
```

普通のfieldにすると、list、detail、search、projection、API、persistenceから読み出せるdomain dataとして
扱われる余地が生じる。passwordは読み出す値ではなく、入力、更新、検証だけが許されるcredentialである。

credentialには少なくとも次の性質が必要になる。

- list/detail/search/filter/labelへ指定できない
- Resolved Intentやdiagnosticへsecret valueを保持・表示しない
- form inputとして受け取ってもentity fieldへ代入しない
- target repositoryで安全なcredential storageを実装できなければagentがblockerとして報告する
- password requirementは宣言・検査可能だが、保護方式はcoding agentとrepositoryが所有する

## 専用identity modelのsyntax sketch

次は議論用であり、現在のgrammarではない。`N`は具体値ではなくpassword policyのmetavariableである。

```forma
role member

entity User {
    name  String required
    email Email required unique

    state status Pending | Active | Suspended initial Pending
}

action User.activate: Pending -> Active

identity UserAccount for User {
    identifier email

    credential password {
        minLength N
    }

    verification email {
        activates activate
    }

    login when status Active
    grants member
}

page SignUp {
    register UserAccount {
        fields name, email
        credential password
    }
}

page VerifyEmail {
    verify UserAccount
}

page Login {
    login UserAccount
}
```

このsketchは、少なくとも次の解決済みsemanticsを生成する意図を持つ。

```text
register
  validate name and email
  validate credential policy
  create User with status Pending
  store credential through the repository's secure identity implementation
  issue email verification

verify
  validate the verification evidence
  dispatch User.activate atomically
  change Pending to Active

login
  validate identifier and credential
  require User.status == Active
  create an authenticated principal
  grant member authorization context
```

## 必要になりそうなResolved Intent

名称は未決定だが、coding agentへ未解決sourceを渡さないため、次のようなtarget-neutral intentが
必要になる。

- `IdentityIntent`
  - subject entity
  - identifier field
  - login eligibility
  - granted authorization context
- `CredentialIntent`
  - credential kind
  - input/update constraint
  - secret handling obligations
- `VerificationIntent`
  - verified subject
  - evidence kind
  - success transition
- `RegistrationIntent`
  - accepted entity fields
  - accepted credentials
  - initial entity state
- `AuthenticationViewIntent`
  - register、verify、login、logoutなどのsurface operation

Acceptance Factsには正常系だけでなく、duplicate email、invalid credential input、expired/invalid
verification、reused verification、inactive membership、unauthenticated access、他人のresourceへのaccess
などの否定caseが必要になる。正確なcase setはidentity semanticsと合わせて決定する。

## 検討する三つの設計方向

### A. 専用`identity` semanticを追加する

identityに固有のsecret、verification、principal、sessionを閉じたmodelとして扱う。application authorが
任意の認証処理を組み立てる必要がなく、Resolved IntentとAcceptance Factsを標準化しやすい。
現時点の第一候補である。

### B. 汎用action inputとeffect modelで表す

register、mail、token、loginをgeneral action、input、effectの組合せで表す。表現力は高いが、credentialや
sessionの安全性をapplicationごとの手続きへ委ね、Formaをgeneral workflow languageへ広げる危険がある。
identity固有semanticsを一般effectだけへ還元できるかは未証明である。

### C. すべてcoding agentの推測へ任せる

Forma sourceには`User`だけを書き、coding agentがsignup/loginを追加する。この案はagent実行ごとにobservableな
application behaviorが変わり、Forma sourceがsource of truthでなくなるため採らない。

## Domain stateとidentity stateを同一視しない

最小例ではemail verification成功を`Pending -> Active`へ接続できる。しかし一般には次は別のaxisである。

- membership lifecycle: Pending、Active、Suspended
- email verification: unverified、verified、reverification required
- authenticated session: runtime principalの有無

email変更時の再認証や複数identifierを考えると、これらを一つの`User.status`へ押し込めない。専用
identity modelがどのstateを所有し、domain transitionとどう接続するかを明示する必要がある。

## Authorizationへの影響

現在のv0 `role`はstaticなauthorization vocabularyだけを定義し、authenticationやownershipを表さない。
表側applicationには少なくとも次が必要になる。

- unauthenticated visitor
- authenticated principal
- principalに対応するUser identity
- `self`または`owns relation`によるresource access
- domain roleとidentityから得るroleの対応

`allow member`だけでは、ある会員が別会員のprofileを編集することを防げない。page/action authorizationと
identity/ownership ruleの合成をResolved IntentとAcceptance Factsへ固定する必要がある。

## 未決定事項

- primitive名を`identity`、`account`、`auth`のどれにするか。
- register、verify、loginをprimitive、view kind、standard actionのどれにするか。
- credential constraintを専用grammarにするか、named policyとして宣言するか。
- verificationのlink/code、期限、再利用禁止を標準semanticsにするかmodifierにするか。
- passwordless、passkey、federated identityを同じmodelで表現できるか。
- external identity providerがUser record作成とverificationを所有する場合のbinding。
- email変更、credential変更、logout-all、account recoveryのmodel。
- membership suspensionと既存session失効をどう合成するか。
- user enumeration、rate limit、abuse preventionのどこまでをlanguage semanticsに含めるか。
- `self`と一般的なownership authorizationを同時に設計するか。
- notification copy、localization、mail templateの所有者。

## 決定前に書く比較例

password signupだけでgrammarを固定しない。少なくとも次を同じ候補syntaxとResolved Intentで記述する。

1. email + password + email verification。
2. passwordを使わないidentity、またはexternal identity provider。
3. email変更に再verificationが必要な既存会員flow。
4. authenticated userだけが自分のprofileを編集できるpage。

各例で次を確認する。

- target repositoryを知らなくてもflowと拒否条件を説明できるか。
- secret valueがentity fieldやdiagnosticへ漏れないか。
- domain state、verification state、session stateが混同されていないか。
- coding agentがframework固有lowering ruleなしにrepositoryへ実装できるか。
- 専用identity modelがgeneral workflowへ不必要に拡張していないか。

この比較が終わるまでは、上記syntaxをEBNF、10 primitives、reference compilerへ追加しない。

## Roadmapへの影響

最初のagent generation experimentでsignup/signin flowを扱うため、identity設計は後回しにできない。
管理画面flowと並行して最小Identity IntentとAcceptance Factsを決め、Generation Requestへ載せる。
credential保護やsession方式はForma coreへ実装せず、coding agentがtarget repositoryの標準的で安全な
仕組みを選ぶ。
