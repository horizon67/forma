# Condition A — Forma Source Only

このsessionでは共通referenceを読んだ後、現行Forma sourceだけを読む。次の3 file以外のrepository文書、生成projection、command、web検索は
使用しない。

- common reference: [`common-reference.md`](common-reference.md)
- membership: [`../../../experiments/membership-agent-e2e/app.forma`](../../../experiments/membership-agent-e2e/app.forma)
- admin CRUD: [`../../../examples/users.forma`](../../../examples/users.forma)

各taskで開始・終了時刻、回答、確信度1〜5を記録する。分からない場合も推測とsourceから確認できない部分を分けて書く。

## T1 — 正常系

applicationを開いた地点からProfileへ到達するまでの正常系を説明する。各surface、operation、application外との境界、
`User.status`変化、session変化を含める。sourceからapplicationのdefault entryを一意に判断できるかも答える。

## T2 — 失敗と禁止結果

次の3 caseについて、operation result、`User.status`、evidence/notice/credential/subjectへの影響をsourceから分かる範囲で
説明する。書かれていない逆命題は推測と明記する。

1. verification evidenceが期限切れ
2. canonicalization後に既存identifierと一致するduplicate signup
3. registration commit後のemail notice delivery failure

## T3 — OnboardingGuide追加

空の`OnboardingGuide` pageを`RegistrationComplete`と`SignIn`の間へ追加し、両方のpageを残す変更を考える。
現在のsyntaxだけでnavigationを一意に表せるか。可能なら変更箇所、不可能なら不足するsemantic capabilityを答える。

## T4 — Regression review

次のsource diffだけが入った。意味上の問題があれば、変わるroute、飛ばされるoperation/effect、変わらないeffectを答える。

```diff
 page VerifyEmail {
     interact UserAccount.verify {
         evidence email
-        success RegistrationComplete
+        success Profile
         continue SignIn
         feedback invalid, expired, failure
     }
 }
```

## T5 — Lifetime変更

verification evidenceの有効期限を30分から60分へ変更する正本のdeclarationを、block階層まで特定する。

## T6 — Admin CRUD

adminのUsers listからdetailを開くroute、edit formを開く2つのroute、edit成功後のrouteを列挙する。
これらのためにmembership専用の別syntaxまたはflow declarationが必要かも答える。
