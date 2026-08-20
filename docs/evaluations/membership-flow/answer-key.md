# Membership Flow Evaluation Answer Key

このfileはsession完了まで参加者へ見せない。満点31点で、記載した部分点だけを使う。

## T1 — 6 points

- 1: application default entryは未宣言であり、SignUpと断定できない。
- 1: SignUp -- register --> CheckEmail。
- 1: durable noticeの外部delivery/open boundaryを経てVerifyEmailへ入る。同期page navigationやdelivery成功保証ではない。
- 1: VerifyEmail -- verify / User.activate --> RegistrationCompleteでPendingからActiveになる。
- 1: RegistrationComplete -- continue --> SignIn。
- 1: SignIn -- signin --> Profileでsessionが始まる。

順序が正しくてもdefault entry、external boundary、domain/session effectを欠けば対応点は与えない。

## T2 — 12 points

Expired verification（4点）:

- 1: verify resultはrejected。
- 1: User.statusはPendingのまま。
- 1: at-expiry/after-expiryではevidence conditionはissuedのまま。
- 1: source/Factsにない逆命題を追加保証として断言しない。

Duplicate signup（4点）:

- 1: exactとcanonical-equivalentの両方がrejectedである。
- 1: subjectは既存1件のままかつunchanged、credentialもunchanged。
- 1: evidenceは既存1件でadded=0、noticeも既存1件でadded=0。
- 1: user-visible disclosureはresend guidanceである。

Notice delivery failure（4点）:

- 1: registrationはrollbackされずsubjectは1件、Pendingで残る。
- 1: verification evidenceはissuedで残る。
- 1: durable notice emission recordは1件あり、deliveryだけがfailedである。
- 1: operation resultはretryableである。

## T3 — 3 points

- 1: pageを追加するだけでは既存の`continue SignIn`の前にもう1本のsurface-only edgeを置けないと判断する。
- 1: projectionはread-onlyなので、図へedgeを足す回答を正本変更として扱わない。
- 1: genericなsurface continuation/transition（または同等の新semantic）が不足すると特定する。

## T4 — 4 points

- 1: verify成功先がRegistrationCompleteからProfileへ変わる。
- 1: continuationのsourceもProfileになり、ProfileからSignInへ進むedgeになる。
- 1: RegistrationCompleteと通常のSignIn operationを飛ばすため、session-start保証なしでProfileへ到達する問題を指摘する。
- 1: `User.activate`はverify success effectのままで、Pending -> Active自体は変わらないと区別する。

## T5 — 2 points

- 1: `identity UserAccount` > `verification email emailLink` blockを特定する。
- 1: `lifetime 30 minute`を`lifetime 60 minute`へ変更する。page navigationやflow viewを編集しない。

## T6 — 4 points

- 1: Users -- User.view --> UserDetail。
- 1: Users -- User.edit --> UserEdit。
- 1: UserDetail -- User.edit --> UserEdit、かつUserEdit submit success --> UserDetail。
- 1: 現行sourceのpage-local action/submit destinationで表され、別のmembership専用syntaxやflow declarationは不要。

## False assertions

採点と別に、次を1件ずつ数える。

- application default entryをSignUpと断定する。
- durable noticeからemail delivery成功までapplicationが保証すると断定する。
- flow projectionまたはMermaid edgeを編集すればsource semanticsが変わると答える。
- Outcome Factに無い禁止結果または逆命題を保証として加える。
- signin前のProfile到達でもsessionが自動的に始まると断定する。
