# Identity Surface Syntax Proposal

Status: Stage C first slice implemented — local-password + email-verification only

この文書は、Stage Bで固定したIdentity Resolved IntentをForma sourceから一意に導出するための最小surface syntaxを
定める。Identity全体を一般化する仕様ではない。passwordless、external provider、identifier変更は引き続き
未対応であり、coding agentへ補完させずcompile diagnosticで拒否する。

設計上の前提は[`identity-semantic-model-proposal.md`](identity-semantic-model-proposal.md) §23と
[`identity-variant-probe.md`](identity-variant-probe.md)である。

## 1. このsliceで閉じるもの

最初のsliceは次だけを受理する。

- subject entity 1件。
- current email identifier 1件。
- `localPassword` authentication proof 1件。
- email link verification 1件。
- register、verify、resend、signin、signout operation。
- authenticated principalとsubject entity自身の`self` ownership。
- page上のIdentity interactionと、authenticated / owner access requirement。

surfaceで複数形を許しても、Checkerはこのcardinality以外を拒否する。IRがsliceであることを、未実装方式を
受理する理由にしない。

## 2. 完全な例

```forma
type Email = String matches /.+@.+/

entity User {
    name     String required
    nickname String
    email    Email required unique

    state status Pending | Active | Suspended initial Pending
}

action User.activate: Pending -> Active

identity UserAccount for User {
    identifier email from email {
        canonicalize trimUnicodeWhitespace, asciiCaseFold
    }

    proof password localPassword {
        minLength 12
        maxLength 128
        lengthUnit unicodeScalarValue
        preserveWhitespace
    }

    registration register {
        identifier email
        proof password
        attributes name
        initial status Pending
        verification email
        existingIdentifier rejectAndGuideResend
    }

    verification email emailLink {
        verify verify
        resend resend
        eligible status Pending
        success User.activate
        lifetime 30 minute
        maxUses 1
        rotation invalidatePriorUnconsumed
        notice email durable
        deliveryFailure pendingAndRetryable
        resendDisclosure uniform
    }

    authentication {
        identifier email
        proof password
        signin signin
        signout signout
        eligible status Active
        failure generic
    }

    ownership self
}

page SignUp {
    interact UserAccount.register {
        fields name
        identifier email
        proof password
        success CheckEmail
        feedback invalid, failure
    }
}

page CheckEmail {
    interact UserAccount.resend {
        identifier email
        stay
        feedback uniform, failure
    }
}

page VerifyEmail {
    interact UserAccount.verify {
        evidence email
        success RegistrationComplete
        continue SignIn
        feedback invalid, expired, failure
    }
}

page RegistrationComplete {
}

page SignIn {
    interact UserAccount.signin {
        identifier email
        proof password
        success Profile
        feedback generic, failure
    }
}

page Profile(user User) {
    require authenticated UserAccount
    require owner UserAccount.self for user

    detail user {
        fields name, nickname, email, status
    }

    interact UserAccount.signout {
        require authenticated UserAccount
        success SignIn
        feedback failure
    }
}

page ProfileEdit(user User) {
    require authenticated UserAccount
    require owner UserAccount.self for user

    form user {
        fields name, nickname
        submit edit
    }
}
```

## 3. `proof`はcredentialの別名ではない

```forma
proof password localPassword { ... }
```

は、`password`という名前のAuthentication Proofを宣言する。`localPassword` proofは秘密入力と永続的な照合材料を
必要とするため、Resolved Intentではproof nodeとcredential nodeの両方を持つ。

```text
identity/UserAccount/proof/password
  kind: local-password
  credential: identity/UserAccount/credential/password
```

この分離により、将来は既存syntaxを変更せずに次を兄弟として追加できる。

```forma
proof emailCode verificationEvidence { ... } // future, unsupported
proof companySSO externalAssertion { ... }   // future, unsupported
```

最初のCheckerは`localPassword`以外を専用diagnosticで拒否する。未知のproofを普通のfieldやcredentialへ縮退させない。

## 4. sourceから導出する意味

次はsourceへ重複して書かず、宣言の組合せから決定的に導出する。

- registrationのatomic outcomeは`subject`、`credential-binding`、`verification-evidence`、
  `notice-emission-record`。
- `emailLink` evidenceはopaque、一度限りで、`now < issuedAt + lifetime`だけ有効。
- `notice email durable`はdurable emission recordを要求する。deliveryはatomic outcomeに含めない。
- authentication sessionはcurrent principalをsubjectへ結び、signoutはcurrent sessionだけを終了する。
- `ownership self`はprincipal subject identityとresource identityの一致を意味する。

これらはimplementation defaultではなく、first sliceのclosed semanticsである。hash、token format、mail provider、
cookie、route、database schemaは導出しない。

## 5. page interaction

`interact Identity.operation`は、Identity declarationで一度だけ定義したoperationをpageから参照する。
interaction bodyは入力、成功後のnavigation、利用者へ観測可能なfeedback、accessだけを持つ。

- `fields`はsubject entityのdomain field。
- `identifier`はIdentity identifier。
- `proof`はproofが要求する秘密入力。Resolved Intent上のinput refは現在のcredential nodeを指す。
- `evidence`はverification evidence。
- `success Page`は固定page navigation。
- `stay`はsame-context navigation。
- `continue Page`は成功pageから選べるcontinuation。

operationごとに許される入力とfeedbackはclosedである。例えば`verify`へpassword proofを渡す、`signout`へidentifierを
渡す、`register`から`expired` feedbackを宣言する、といった組合せはCheckerが拒否する。

最初のsliceではregister、verify、resend、signin、signoutの各operationに対し、application全体でinteractionを
**ちょうど1件**要求する。0件ならoperationが利用者から観測できず、2件以上なら現在のmonolithic Fact builderが一部の
interactionを検証できないため、どちらも`F2715`で拒否する。複数surfaceを許可するには、存在するinteraction nodeごとに
Factを合成するbuilderを先に実装する。

## 6. access

既存の`allow admin, editor`はrole requirementのまま維持する。Identity accessをrole定数へ縮退させず、独立した
`require` clauseとして追加する。

```forma
require authenticated UserAccount
require owner UserAccount.self for user
```

複数の`require`と既存`allow`はANDで合成する。`allow admin, editor`のrole一覧だけがORである。したがって、
`(admin OR editor) AND authenticated AND owner`を平坦化しない。

pageの`require`はpage、既存view submit、action referenceへ適用する。interaction内の`require`はそのoperationへ
適用する。interactionがpage上に置かれていても、operation contractに不要なowner条件を暗黙追加しない。

## 7. 一意性とnegative cases

同じResolved Intentへ複数の書き方を作らないため、最初のsliceでは次を禁止する。

- `credential password`やentity field `password`をproofの代替にする。
- identifier fieldを省略して名前から推測する。
- operation名、state、success page、feedbackを慣習から推測する。
- registrationのdomain fieldをpage inputだけから逆算する。
- `allow member`をauthenticatedまたはownerの代替にする。
- external providerを`proof ... localPassword`へ偽装する。
- email変更を2個目のcurrent identifierまたは通常`edit`として宣言する。

少なくとも次のnegative testをParser / Checkerへ置く。

1. 未対応proof kindを拒否する。
2. proof、identifier、verificationの重複または欠落を拒否する。
3. unknown subject field、state、action、page、operationを拒否する。
4. operationごとのinput / feedback集合の過不足を拒否する。
5. anonymous pageへauthenticated requirementを推測で追加しない。
6. protected pageのowner bindingがpage parameterでない場合を拒否する。
7. `UserAccount.changeEmail`のような未宣言lifecycle operationを拒否する。
8. 同じoperationのinteraction重複と、interactionのないoperationを拒否する。

## 8. Stage C exit criteria

- この文書のsourceからStage B membership fixtureと同じResolved Intent semanticsを生成する。
- source declaration順を変えてもcanonical JSONが変わらない。
- 全semantic nodeがSource Mapへ1対1で載る。proofとcredentialは同じsource spanを共有してよいが、別nodeである。
- 38 Acceptance Factsと3 Review Requirementsをsource由来Intentから再導出できる。
- credential/evidence valueはsource、Intent、Source Map、Factsへ入らない。
- unsupported axisはGeneration Requestより前にdiagnosticで拒否される。
