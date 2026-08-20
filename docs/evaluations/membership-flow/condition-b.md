# Condition B — Forma Source + Projections

このsessionでは共通referenceを読んだ後、現行Forma sourceと、同じsourceから生成済みのread-only projectionを読む。次のfile以外のrepository文書、
command、web検索は使用しない。

- common reference: [`common-reference.md`](common-reference.md)

Membership:

- source: [`../../../experiments/membership-agent-e2e/app.forma`](../../../experiments/membership-agent-e2e/app.forma)
- visual flow: [`../../../internal/compiler/testdata/membership.flow.md`](../../../internal/compiler/testdata/membership.flow.md)
- outcome detail: [`../../../internal/compiler/testdata/membership.outcomes.txt`](../../../internal/compiler/testdata/membership.outcomes.txt)
- domain state detail: [`../../../internal/compiler/testdata/membership.states.txt`](../../../internal/compiler/testdata/membership.states.txt)

Admin CRUD:

- source: [`../../../examples/users.forma`](../../../examples/users.forma)
- visual flow: [`../../../internal/compiler/testdata/users.flow.md`](../../../internal/compiler/testdata/users.flow.md)

各taskで開始・終了時刻、回答、確信度1〜5を記録する。projectionは編集対象の正本ではない。overviewに結び付かない
outcome/stateはunlinked indexと詳細projectionに残る。

## T1 — 正常系

applicationを開いた地点からProfileへ到達するまでの正常系を説明する。各surface、operation、application外との境界、
`User.status`変化、session変化を含める。sourceからapplicationのdefault entryを一意に判断できるかも答える。

## T2 — 失敗と禁止結果

次の3 caseについて、operation result、`User.status`、evidence/notice/credential/subjectへの影響を、outcome detailで
明示された範囲に限定して説明する。書かれていない逆命題を作らない。

1. verification evidenceが期限切れ
2. canonicalization後に既存identifierと一致するduplicate signup
3. registration commit後のemail notice delivery failure

## T3 — OnboardingGuide追加

空の`OnboardingGuide` pageを`RegistrationComplete`と`SignIn`の間へ追加し、両方のpageを残す変更を考える。
現在のsyntaxとprojectionだけでnavigationの正本を一意に表せるか。可能なら変更箇所、不可能なら不足するsemantic
capabilityを答える。

## T4 — Regression review

同じsource mutationから再生成されたflow projectionには次のsemantic diffが出た。意味上の問題があれば、変わるroute、
飛ばされるoperation/effect、変わらないeffectを答える。

```diff
- E07 | RegistrationComplete -> SignIn | continue / continue
+ E07 | Profile -> SignIn              | continue / continue

- E08 | VerifyEmail -> RegistrationComplete | UserAccount.verify / success; effects=User.activate
+ E08 | VerifyEmail -> Profile              | UserAccount.verify / success; effects=User.activate
```

## T5 — Lifetime変更

verification evidenceの有効期限を30分から60分へ変更する正本のdeclarationを、block階層まで特定する。

## T6 — Admin CRUD

adminのUsers listからdetailを開くroute、edit formを開く2つのroute、edit成功後のrouteを列挙する。
これらのためにmembership専用の別syntaxまたはflow declarationが必要かも答える。
