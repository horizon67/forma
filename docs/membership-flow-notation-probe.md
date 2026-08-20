# Membership Flow Notation Probe

Status: candidate B navigation and outcome text projections implemented — candidate C remains pseudocode and is not a language decision

## 1. 目的

メール認証付き会員登録の同じapplication semanticsを、次の3案で比較する。

1. 現行Forma sourceだけを読む。
2. 現行Forma sourceを正本のまま維持し、flowとoutcome tableを投影する。
3. `flow`をinter-page navigationの正本にし、pageからnavigation destinationを除く。

比較対象は構文の短さや見た目の好みではない。初見理解、失敗経路の発見、変更箇所の特定、diff review、
数か月後の再読で、applicationの観測可能な意味をどれだけ正確に説明できるかを評価する。

最初のcandidate B sliceはParser、Resolved Intent、Acceptance Factsを変更せず実装した。候補3のsyntaxは比較のための
pseudocodeである。

## 2. 固定するsemantic baseline

正本となる現行sourceは
[`../experiments/membership-agent-e2e/app.forma`](../experiments/membership-agent-e2e/app.forma)である。
比較中に次の意味を簡略化しない。

### Domain

- `User.status`は`Pending`で始まり、正しいverificationだけが`User.activate`を通して`Active`へ変える。
- profileの表示・編集はauthenticated principal本人に限る。
- adminのUser CRUD surfaceも同じapplication内に残す。

### Registration and verification

- registerはUser、credential binding、verification evidence、durable notice emission recordをatomicに成立させる。
- duplicateまたはcanonical equivalentなidentifierは新しいregistrationを作らず、再送を案内する。
- verification evidenceは30分、一度限りで、再送時に以前の未使用evidenceを無効にする。
- notice delivery failureはatomic registrationをrollbackせず、Userを`Pending`かつretryableに保つ。
- resendはPending、Active、unknown identifierでuser-visible outcomeを同じにする。

### Authentication and interaction

- `Active` Userだけがsigninでき、失敗理由はgenericである。
- signoutはcurrent sessionを終了する。
- SignUpを開いた後の正常系surface progressionは次である。application全体のdefault entryは現行sourceに宣言されていない。

```text
SignUp
  -> CheckEmail
  -> [verification email link]
  -> VerifyEmail
  -> RegistrationComplete
  -> SignIn
  -> Profile
```

`CheckEmail -> VerifyEmail`は通常のpage actionではない。durableに記録されたemail noticeが外部配送され、
利用者がlinkを開くことで`VerifyEmail`へ入る。projectionはこの境界を通常の同期navigationへ偽装してはならない。

## 3. 評価task

各候補を次の同じtaskで評価する。将来、人間参加で測定する場合は正答率、完了時間、見落とし、誤検出、
確信度を記録する。

| ID | task | 正答に必要な情報 |
| --- | --- | --- |
| T1 | 初見で正常系を入口からProfileまで説明する | page、operation、外部email境界、domain transition |
| T2 | expired evidence、duplicate signup、delivery failureの結果を説明する | feedbackだけでなくstateとside effectの禁止事項 |
| T3 | RegistrationCompleteとSignInの間にOnboardingGuideを追加する | navigationの正本、surface-only transitionの表現力 |
| T4 | verification成功先が誤って`Profile`へ変わるdiffを発見する | signin/sessionを飛ばす意味的な違反 |
| T5 | verification lifetimeを30分から60分へ変更する場所を特定する | navigationではなくIdentity evidence policy |
| T6 | admin CRUDのlist/detail/editを追跡する | 複雑なflow対応が単純なCRUDを不必要に冗長化しないこと |

## 4. 候補A — 現行sourceだけ

現行syntaxをそのまま正本とする。Identity operationの意味は`identity`、interactionの入力・feedback・navigationは
各`page`が所有する。

```forma
action User.activate: Pending -> Active

identity UserAccount for User {
    registration register {
        initial status Pending
        verification email
        existingIdentifier rejectAndGuideResend
        // identifier、proof、attributesは省略せず現行sourceに存在する
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
        signin signin
        signout signout
        eligible status Active
        failure generic
        // identifierとproofは省略せず現行sourceに存在する
    }
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

page SignIn {
    interact UserAccount.signin {
        identifier email
        proof password
        success Profile
        feedback generic, failure
    }
}
```

### 評価

- pageを一つずつ読むtaskと、operationの局所的な変更には強い。
- 各semantic factの正本は一箇所で、生成viewとのdriftがない。
- T1では`success`、`continue`、`stay`を複数pageから集め、Identityの`success User.activate`と合成する必要がある。
- T1からdefault entryを一意には答えられない。`SignUp`がregistration interactionを持つことと、application起動時の
  default surfaceであることは別の意味である。
- T2ではinteractionの`feedback`だけでは足りず、Identity内のatomic outcome、evidence、delivery failureも読む必要がある。
- T3は現行の`success Page` + 単一`continue Page`だけでは、既存のRegistrationCompleteを残したまま
  OnboardingGuideとSignInまでの2本のcontinuation edgeを表現できない。これは可読性ではなく表現力の不足である。
- T4のnavigation diffは小さいが、離れたpageを読まないとflow全体への影響を確認しにくい。
- T6のCRUDには現在の局所的な`goto`とsubmit destinationが適している。

## 5. 候補B — 現行source + 決定的projection

候補Aを唯一のsource of truthとして維持し、Resolved Intentから読み取り専用viewを生成する。viewを編集、commit、
または別の要求入力として扱わない。

### 5.1 Navigation projection

```text
application default entry: unspecified

SignUp [registration surface]

SignUp
  -- UserAccount.register : success --> CheckEmail

CheckEmail
  -- UserAccount.resend : success --> CheckEmail
  -- UserAccount.email notice : external delivery/open --> VerifyEmail

VerifyEmail
  -- UserAccount.verify : success / User.activate --> RegistrationComplete

RegistrationComplete
  -- continue --> SignIn

SignIn
  -- UserAccount.signin : success / session started --> Profile

Profile
  -- UserAccount.signout : success / current session ended --> SignIn
```

`notice : external delivery/open` edgeは、applicationが外部配送成功を保証するという意味ではない。sourceが保証する
durable emission recordと、application外のdelivery/openを異なるedge kindで表示する。

### 5.2 Domain state projection

```text
User.status

Pending -- UserAccount.verify / User.activate --> Active

signin eligible: Active only
resend eligible: Pending
```

### 5.3 Outcome table projection

| operation | case | domain result | interaction result | prohibited result |
| --- | --- | --- | --- | --- |
| register | valid | Pending Userと4 atomic records | CheckEmail | partial registration |
| register | invalid | no change | SignUp + invalid | User、credential、notice作成 |
| register | duplicate | no change | resend guidance | 2人目、credential上書き |
| resend | Pending | state unchanged、新evidence、notice | CheckEmail + uniform | 旧evidenceの継続利用 |
| resend | Active/unknown | state unchanged | CheckEmail + uniform | identity existence disclosure |
| verify | valid/unconsumed/in time | Pending -> Active、evidence consumed | RegistrationComplete | 2回目のactivation |
| verify | invalid/expired/consumed | state unchanged | VerifyEmail + feedback | session開始、state変更 |
| signin | Active + correct proof | state unchanged、session開始 | Profile | — |
| signin | Pendingまたはinvalid proof | state unchanged、no session | SignIn + generic | failure reason disclosure |
| signout | authenticated | state unchanged、current session終了 | SignIn | other sessionの暗黙終了 |
| notice delivery | failure | Pending、retryable | failure/retry path | Active化、registration rollback |

### 5.4 Projection contract

- edge、row、nodeはResolved Intentのstable semantic IDを持つ。
- 各要素からSource Mapの宣言位置へ戻れる。
- 同じResolved Intent versionと内容からbyte-identicalなtext projectionを得る。
- ordering、label、external boundaryの表示規則をtool versionで固定する。
- source diff reviewでは、old/new Resolved Intentからsemantic projection diffを再計算する。
- diagram layoutは正本へ保存しない。visual graphを提供する場合も同じprojection modelから生成する。

### 評価

- T1とT2はsourceだけよりglobal/localを往復しやすい。
- T1ではdefault entryが未指定であることも見える。projectionが`SignUp`をapplication homeとして推測してはならない。
- T3では現在のResolved Intentに必要な2本目のsurface-only edgeが存在しないことを可視化できるが、projectionだけで
  edgeを追加することはできない。
- T4はnavigation projection diffで全体への影響を確認でき、source上の正本も一箇所のままである。
- T5はdomain state projectionからIdentity policyへ辿れるが、navigationへ誤って置く余地を増やさない。
- T6は既存CRUD syntaxを変えず、必要なときだけnavigation projectionで俯瞰できる。
- 欠点はprojectionを生成するtoolが必要なことと、projectionの情報設計自体をversion管理する必要があることである。

## 6. 候補C — `flow`がnavigationを所有する

次は比較用pseudocodeであり、現在のParserは受理しない。`flow`を追加する場合、inter-page destinationは
`flow`だけが所有する。page interactionへ`success Page`、`stay`、`continue Page`を併記してはならない。

```forma
flow Registration {
    start SignUp
    entry VerifyEmail via UserAccount.email

    SignUp              -> CheckEmail           on success UserAccount.register
    CheckEmail           -> CheckEmail           on success UserAccount.resend
    VerifyEmail          -> RegistrationComplete on success UserAccount.verify
    RegistrationComplete -> SignIn               on continue
    SignIn               -> Profile              on success UserAccount.signin
    Profile              -> SignIn               on success UserAccount.signout
}

page SignUp {
    interact UserAccount.register {
        fields name
        identifier email
        proof password
        feedback invalid, failure
    }
}

page CheckEmail {
    interact UserAccount.resend {
        identifier email
        feedback uniform, failure
    }
}

page VerifyEmail {
    interact UserAccount.verify {
        evidence email
        feedback invalid, expired, failure
    }
}

page RegistrationComplete {
}

page SignIn {
    interact UserAccount.signin {
        identifier email
        proof password
        feedback generic, failure
    }
}

page Profile(user User) {
    // 現行sourceと同じauthenticated + owner accessとdetailを持つ
    interact UserAccount.signout {
        require authenticated UserAccount
        feedback failure
    }
}
```

`via UserAccount.email`は、verification noticeに含まれるevidenceから入るexternal entryを表す仮名である。
採用する場合は、delivery成功を保証せず、どのsemantic nodeを参照するかを先に定義する必要がある。
`RegistrationComplete -> SignIn on continue`のedgeは、continuation capabilityとdestinationの両方をflowが所有する。
page側へ同じ`continue`を再宣言しない。

### 必須のownership rule

- navigation destinationはすべて`flow`が所有する。
- pageはsurfaceの内容、入力、feedback、accessだけを所有する。
- domain transition、evidence policy、atomic outcomeは引き続きIdentity/actionが所有する。
- flow edgeはdomain state transitionを再宣言せず、既存operation/actionを参照する。
- 同じpageを複数flowから参照するときも、同じinteractionのsuccess destinationを競合させてはならない。

### CRUDへの適用

navigationを常にflow所有にするなら、admin CRUDも概念上は次を必要とする。

```forma
flow UserAdministration {
    Users      -> UserDetail on User.view
    Users      -> UserEdit   on User.edit
    UserDetail -> UserEdit   on User.edit
    UserEdit   -> UserDetail on success User.edit
}
```

この記述が不要なときにcompilerが一意な宛先を導出するなら、明示flowと省略形のcanonicalization規則が必要になる。
flowだけをmembership専用の別表記にすると、同じnavigation conceptに二つのcanonical formを持つため採用しない。

### 評価

- T1、T3、T4のnavigation理解と変更局所性は高い。
- T3は`RegistrationComplete -> OnboardingGuide on continue`と
  `OnboardingGuide -> SignIn on continue`へ局所的に変更できる。ただしgeneric continuationの正確なsemanticsは未決定である。
- pageだけを見ても成功先が分からず、候補Aとは逆方向のhidden dependencyが生じる。
- T2とT5に必要なatomicity、evidence、state、securityはflowだけでは読めず、Identityとの往復は残る。
- T6では単純CRUDにもflow boilerplateを要求するか、省略規則を設計する必要がある。
- external entry、nested flow、複数flowからの参照、back/cancel、同一operationの複数surface、並行interactionの
  semanticsが未決定で、現時点の実装コストは3案中最も大きい。

## 7. 比較結果

| 観点 | A: 現行source | B: 現行 + projection | C: flow正本 |
| --- | --- | --- | --- |
| semantic factの正本 | 一箇所 | 一箇所 | ownership ruleを守れば一箇所 |
| 正常系のglobal overview | 複数pageを追う | flow viewで確認できる | sourceのflow blockで確認できる |
| application default entry | 未指定 | 未指定と表示 | `start`で指定する候補 |
| 例外・禁止結果 | Identityとpageに分散 | outcome tableで統合できる | Identityとの往復が残る |
| page単体の局所理解 | 強い | 強い | destinationはflow参照が必要 |
| navigation diff review | source探索が必要 | semantic projection diffを併用 | source diffが局所化する |
| 任意のsurface-only chain | 現行shapeでは表せない | 不足を表示できるが表せない | 候補syntaxでは表せる |
| admin CRUDの簡潔さ | 現状維持 | 現状維持 | boilerplateまたは省略規則が必要 |
| 新しいlanguage semantics | 不要 | 不要 | 必要 |
| 最初の実装risk | なし | projection correctness | grammar、IR、Facts、migration |

候補Bを次の実装probeとする。これは候補Cを却下する決定ではない。まずsourceを増やさず、現在のResolved Intentに
global overviewを作るのに十分な意味が既にあるかを測る。同時にT3によって、projectionでは埋められない
surface-only transitionのsemantic gapを明示する。projectionで得る共通edge modelを、後続の`flow` source候補でも
再利用できる形にする。

したがって現時点の判定は「projectionだけで十分」ではない。

- 読みやすさの最小改善は候補Bで測る。
- 任意のmulti-step interactionを表すための新semanticは必要になる可能性が高い。
- そのsurface syntaxを`flow`にするか、汎用page actionにするかは、共通edge modelと追加例を作ってから決める。

## 8. 最初の実装probeの結果

最初のvertical sliceはnavigation text projectionに限定し、`forma project navigation`として実装した。

1. [x] current Resolved Intentからpage、interaction、standard action、submit、continuationのedgeを導出した。
2. [x] Identity noticeからverification pageへのentryを`external-boundary` edgeとして分離した。
3. [x] default entryがsource/Intentに無い場合は`unspecified`と表示し、page順や名前から推測しない。
4. [x] 各edgeへstable semantic ID、source node、destination policy、trigger、outcome、Source Map provenanceを持たせた。
5. [x] declaration順とsource pathを変えてもbyte-identicalなtextになることをtestした。
6. [x] destination、operation、external entry surfaceのmutationがsemantic edge差分へ現れることをtestした。
7. [x] admin CRUDとmembershipを同じprojection modelで表し、`same-context`と`caller-list`を固定pageへ潰さず保持した。

membership projectionでは、registration、resend、external email boundary、verification時の`User.activate`、
signin/signout時のsession変化、continuationを一つのtext viewで確認できる。admin projectionではcreate/view/edit/delete、
domain transition、form submitを同じedge modelで確認できる。すべてのedgeはsource pathではなくsemantic node ID集合を
provenanceとして持ち、既存Source Mapから宣言位置へ戻れる。

次に`forma project outcomes`を実装した。compiler-owned Acceptance Factsのうち観測可能なresult、feedback、navigationを
group化し、multi-case Factをcase行へ展開する。membership applicationは83行、admin CRUDは69行となり、同じmodelと
formatterでgolden固定した。各行はFact IDとSource Map provenanceを保持する。

概念例の11行へ手作業で畳み込むと、複数Factを一つの物語へ結合する際に未宣言の禁止結果を作りやすい。最初の実装では
Factとの1対1または1対case対応を維持し、`count=0`、`added=0`、`absent`、`excluded`、`stored=unchanged`だけを
`must not`へ分離した。たとえばinvalid registrationのsubject/credential/evidence/notice非作成、duplicate時の
evidence/notice非追加、signin失敗時のsession非作成が表示される。一方、Factに無い逆命題は推測しない。

domain-state専用projectionとvisual diagramは後続候補である。最初からlayout、Mermaid syntax、UIをsemantic modelへ入れない。

## 9. 判定を見直す条件

次のいずれかを実測した場合は候補Cを再評価する。

- projectionを見ても、navigationのsource変更箇所を一意に特定できない。
- page-localなdestinationの追加・削除が複数fileへ波及し、semantic diffの見落としが繰り返される。
- 同じoperationを複数surfaceから起動したとき、page-owned navigationでは矛盾を静的検査できない。
- nested、interruptible、parallel flowを表すために、page interactionへnavigation modifierを増やし続ける必要がある。
- 人間参加のT1〜T4で、候補Cが候補Bより正確かつ速く、T6の悪化も許容範囲だと確認できる。

逆に、生成projectionでglobal overviewとsemantic diffを十分に読めるなら、`flow`は新primitiveにせずviewとして維持する。
