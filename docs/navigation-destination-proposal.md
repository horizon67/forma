# Navigation Destination Proposal

Status: minimal proposal — syntax not yet implemented

Stage D の最初のコンパイルで、admin surface と member surface が同じ entity の detail / edit form を
宣言したときに標準 action の宛先が解決できないことを実測した。本書はその最小の解決案を固定する。

## 1. 測定した問題

`experiments/admin-agent-e2e/app.forma`（admin CRUD）へ `examples/email-verified-membership.forma` の
Identity と membership page を足すと、次で止まる。

```text
error[F2501]: action `view` resolves to 2 detail destinations
error[F2501]: action `edit` resolves to 2 edit form destinations
error[F2501]: action `edit` resolves to 2 edit success detail destinations
error[F2501]: action `user` resolves to 2 edit success detail destinations
```

`UserDetail` / `UserEdit`（`allow admin`）と `Profile` / `ProfileEdit`（`require owner`）が、同じ `User` の
detail と edit form として両方一致する。**「既存 application へ Identity を足す」という Milestone 4 の目的が、
現行の宛先解決規則では成立しない。**

診断自体は正しい。v0 §6 の「候補が 0 個または複数なら曖昧さを推測せず error にする」が働いている。
欠けているのは、作者が曖昧さを解消する手段である。

## 2. 却下した案 — access による候補の絞り込み

「source を開ける principal が開けない destination を候補から外す」規則を試作したが、採用しない。

**現行の認可モデルと矛盾する。** `composeAccess` は source page と destination page の requirement を
`AllOf` で合成する（`docs/v0-primitives.md` §497）。つまり `source: admin` / `destination: member` は
`(admin AND member)` として有効であり、action は両方を満たす principal にだけ提示される。開けない
principal にリンクが出ることはないので、「到達不能なリンクは潜在バグ」という前提が成り立たない。

**非単調になる。** 候補が 1 件のときは access を見ない実装にすると、無関係な page を 1 つ足しただけで
既存 action の宛先が変わるか消える。program の離れた場所の変更が navigation を書き換えるのは、Forma が
避けてきた推測そのものである。

**admin と owner は排他ではない。** admin 本人が対象レコードの owner である場合があり、
「admin は owner page を開けない」という前提自体が誤りだった。

## 3. 現行の宛先解決点

標準 action と form submit は、次の 7 箇所で宛先を解決している。

| # | 解決点 | 候補 | 候補 0 件の現行挙動 |
| --- | --- | --- | --- |
| 1 | `create` の target | create form | error |
| 2 | `create` の success | detail | 現 page |
| 3 | `view` の target | detail | error |
| 4 | `edit` の target | edit form | error |
| 5 | `edit` の success | detail | 現 page |
| 6 | `delete` の success | list（detail 上のみ） | error |
| 7 | form `submit` の success | detail → list → 現 page | caller-list または same-context |

**#2 / #5 / #7 は「その entity の唯一の detail を探す」同一ロジックの 3 つの写し**である。片方だけ直すと、
同じ program で `edit` は解決できるのに `create` だけ失敗する、という不整合になる。

4.1 で #2 と #5 を削除し、残る解決点は **#1 #3 #4 #6 #7 の 5 箇所**になる。

## 4. 提案

### 4.1 create / edit action の success を持たせない

`IRActionRef` は `SuccessPage string` しか持たないが、`IRSubmitIntent.Success` は `IRNavigationIntent` で
`page` / `caller-list` / `same-context` の 3 種類を取る。したがって「form の success を使う」だけでは、
`caller-list` を action 側でどう投影するかが決まらない。

**action 側に success を持たせない。** 選ばれた form page の `SubmitIntent.Success` を唯一の正本とする。

| action | target | success |
| --- | --- | --- |
| `create` | create form page | 持たない（target form の SubmitIntent が正本） |
| `edit` | edit form page | 持たない（同上） |
| `view` | detail page | 元から無し |
| `delete` | 無し | `SuccessPage`（list または現 page）を維持 |

`delete` は form を経由しないので `SuccessPage string` を残す。`IRNavigationIntent` への一般化は行わない。

**この変更は fact の被覆を減らさない。** 現在の membership request では同じ success が 3 回アサートされている。

```text
page/UserEdit/view/form/edit/User/submit/navigation   successPage: page/UserDetail   ← 正本
page/UserDetail/view/detail/User/action/edit/navigation successPage: page/UserDetail  ← 重複
page/Users/view/list/User/action/edit/navigation        successPage: page/UserDetail  ← 重複
```

削除されるのは重複 2 件で、正本は submit intent の fact に残る。あわせて #5 と #7 が独立解決である現状
（単一 surface では偶然一致しているだけで食い違いうる）も構造的に解消する。

**影響**: `create` / `edit` の `navigation` fact から `successPage` が消えるため、Acceptance Facts の
version を上げ、既存 golden を更新する。Resolved Intent 側も `IRActionRef` の意味が変わるため version を
上げる。実装時に、admin experiment の historical baseline が §18 の version-dispatched 経路で
引き続き検証できることを確認する。

### 4.2 明示 destination は標準 action 参照にだけ許可する

navigation の keyword は既に `goto` がある（`action_mod` の `goto Page`）。`->` は state transition が
使っているので、新しい記号を導入せず `goto` を再利用する。

```forma
list User {
    columns name, email, status
    actions view goto UserDetail, edit goto UserEdit
}

form user {
    fields name, nickname
    submit edit goto UserDetail
}
```

文法は `actions` の各要素を `name [ "goto" type_name ]`、`submit` を `action_name [ "goto" type_name ]` とする。

**domain action 参照へ inline `goto` は書けない。**

```forma
action User.suspend: Active -> Suspended goto SuspendedUsers

detail user {
    actions suspend goto UserDetail   // error
}
```

domain action の navigation は top-level 宣言だけを正本とする（v0 §10 決定事項 7「domain action は
top level で一度だけ宣言し、view 側は参照だけを行う」）。参照側に `goto` を許すと正本が 2 つになる。

### 4.3 解決規則

| 状況 | 挙動 |
| --- | --- |
| `goto` あり、指定先が正しい候補 | 採用 |
| `goto` あり、指定先が候補でない | error |
| `goto` なし、候補 1 件 | 自動解決 |
| `goto` なし、候補 2 件以上 | F2501（help を更新） |
| 候補 0 件 | 解決点ごとの既存規則（§3 の表）を維持 |

**候補 1 件でも `goto` を書ける。** 当初は Stage C の一意性制約に倣って禁止する案だったが、採らない。
Stage C の制約（resend の `stay` 必須など）は意味上の要求であって canonical form の強制ではなく、
`actions view` と `actions view goto UserDetail` は同じ Resolved Intent へ正規化されるので検証上の
曖昧さがない。禁止すると detail page の増減のたびに無関係な `goto` を足したり消したりする source churn が
発生する。明示形を常に許せば page 増減に対して source が安定する。

## 5. 診断

```text
error[F2501]: action `view` resolves to 2 detail destinations
help: name the destination with `view goto <Page>`

error[F25xx]: `goto Profile` is not an edit form for `User`
help: name a page that declares `form` for this entity binding

error[F25xx]: domain action `suspend` cannot name a destination at the reference
help: declare `goto` on `action User.suspend` instead
```

## 6. 必要な negative test

1. admin detail と member detail が共存し、`goto` なしで F2501 になる
2. `goto` を付けると両 surface が解決し、それぞれ正しい page を指す
3. `goto` が候補でない page を指すと error になる
4. domain action 参照への inline `goto` が error になる
5. 候補 1 件でも `goto` を書ける（禁止しない回帰テスト）
6. 5 つの解決点すべてで同じ規則が働く
   1. `create` の target
   2. `view` の target
   3. `edit` の target
   4. `delete` の success
   5. form `submit` の success
7. `create` / `edit` action の success が、選択した form の `SubmitIntent.Success` と一致する
   （action 側に重複保持しないことの確認）
8. access がより厳しい destination が 1 件だけの場合、`goto` なしで解決する
   （access を宛先選択に使わないことの回帰テスト）
9. AND 合成で有効な role destination が複数ある場合も、access ではなく `goto` で決まる

## 7. 本書で決めないこと

- `actions` の block 形式
- 同じ entity に 3 つ以上の surface が並ぶ場合の可読性
- Identity interaction の `success` / `continue` との統合（現状は別語彙で衝突していない）

## 8. 影響範囲

既存の `examples/*.forma` と 2 つの experiment は entity ごとに surface が 1 つなので `goto` を書く必要が
なく、source は変わらない。4.1 の success 削除は Resolved Intent と Acceptance Facts の shape を変えるため、
version を上げて golden を更新する。historical baseline の検証経路が壊れていないことを実装時に確認する。
