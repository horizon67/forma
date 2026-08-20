# Order Approval, Inventory, and Effect Proposal

Status: exploratory proposal — not valid v0 syntax and not a language decision

この文書は、注文・承認・在庫を扱うapplicationをFormaでどう表すかというdesign probeである。目的は
effect syntaxを先に決めることではなく、[`roadmap.md`](roadmap.md)のMilestone 6が挙げる
derived value、entity invariant、state以外のaction precondition、複数entityをまたぐeffect、
transaction boundary、runtime由来fieldが、実例のどこで必要になるかを実測することである。

規範仕様は[`v0-primitives.md`](v0-primitives.md)である。本書に現れる`effect`、`on`、`emit`、
Derived Value、Action Preconditionなどの式は現在のParserでは受理されず、最終的なlanguage designも
未決定である。probe後、reference compilerには規範v0外のexperimental sliceとして、名前付きInvariantと
selfのrequired field同士の`<=`だけを実装した。範囲と現在地は
[Minimal Expression Layer Proposal](expression-proposal.md)に記録する。

[`email-verified-membership-probe.md`](email-verified-membership-probe.md)がidentityの最初の実測probeであるのに対し、
本書はeffectとoccurrenceのprobeである。両方に共通して必要なものだけをeffect proposalの対象とする。

## 対象とするflow

1. staffが注文を作成し、明細を追加して提出する。
2. adminが承認または却下する。承認は在庫を引き当てる。
3. 承認時に顧客へ通知し、監査記録を残す。
4. 通知が届かなかった場合、状態を変えずに通知だけ再送する。
5. 在庫が閾値を下回ったら補充を促す。
6. 一定期間承認されない注文を検出する。

このflowには、domain data、値の導出と不変条件、外部への効果という異なる責務が含まれる。

## 現行v0で表現できる範囲

最初のprobeでは[`examples/orders.forma`](../examples/orders.forma)を10プリミティブだけで書き、`forma check`が
errorなしで通ることを確認した。現在の同exampleには、後続probeで実装したexperimentalな
`invariant stockAvailable: reserved <= onHand`も加えている。v0だけで表現できた範囲は次である。

| flow | v0で表現できること |
| --- | --- |
| 注文・明細・商品・在庫の構造 | `entity`、to-one/to-many relation、named type、`unique` |
| 注文のlifecycle | `state status Draft \| Submitted \| Approved \| Rejected \| Shipped initial Draft` |
| 承認・却下・出荷の許可された遷移 | `action Order.approve: Submitted -> Approved confirm allow admin` |
| 誰が承認できるか | `role admin`、`allow admin, staff` |
| 一覧・詳細・作成・編集・削除と検索・絞り込み・ソート・ページング | `page`、`list`、`detail`、`form` |
| 承認後の遷移と二重実行防止 | Resolved Intentのsubmit intentとaction referenceへ決定的に解決される |

つまりv0は、この領域の**構造とlifecycleと認可**をすでに表現できる。不変条件はexperimentalな
self-only sliceまで進んだが、値の導出、状態遷移以外の事前条件、そして外部への効果はまだ足りない。

## probe実施時にv0が拒否した記述（実測）

予想ではなく、probe実施時のcompilerに与えて確認した結果を記録する。これは当時の不足を保存する
historical resultであり、後から追加したexperimental syntaxの現在地は表の注記とexpression proposalを参照する。

| 書きたかったこと | 結果 |
| --- | --- |
| `action Order.resendApproval: Approved -> Approved` | `F2301` transition source and destination must differ |
| `action Order.resendApproval`（遷移なし） | `F1002` expected `:` after the action name |
| `lineTotal Decimal = quantity * product.price` | `F1002` expected a field modifier, found `=` |
| `invariant reserved <= onHand` | probe時は`F1101` / `F0001` / `F1002`。現在も匿名形はinvalidだが、`invariant stockAvailable: reserved <= onHand`はexperimental sliceで受理する |

最初の2件が重要である。**v0の`action`は、状態を変えないactionを文法レベルで宣言できない。**
通知の再送は業務上明確に「利用者が意図して実行する操作」だが、`action`の定義が
「許可された状態遷移」であるため、宣言する場所がない。

## 不足しているsemantic axis

| 必要なもの | 例 | 現状 |
| --- | --- | --- |
| derived value | `Order.total`、`OrderLine.lineTotal` | 式レイヤがない |
| snapshot / runtime由来field | 注文時点の`unitPrice`を固定する | `default`はliteralのみ。§9で除外 |
| entity invariant | `reserved <= onHand` | 規範v0にはない。reference compilerにself-only `<=`のexperimental sliceだけある |
| state以外のprecondition | 明細0件では提出不可、在庫不足では承認不可 | `A -> B`しか書けない |
| 状態を変えないaction | 通知の再送 | `F2301`で拒否される |
| 複数entityをまたぐchange | 承認が`Order.status`と`StockItem.reserved`を同時に変える | ない |
| effect | 承認通知、監査記録 | §9で除外 |
| schedule trigger | 未承認注文の検出 | §9で除外 |
| inverse relation | `Order.lines`と`OrderLine.order` | 独立した2 relationとして扱われる |
| derived list | 閾値を下回った在庫の一覧 | ない |

## Probe 1 — occurrenceの単位はactionで閉じるか

Effectの発生条件を`State change -> Effect`にできないことは、通知の再送で確定している。では
`action実行 + schedule`の2つで閉じるか。この例から出た候補を分類する。

| occurrence | 分類 |
| --- | --- |
| 注文が承認された | action実行（`action/Order/approve`） |
| 承認通知が再送された | action実行だが状態変化なし。**現在の文法では宣言できない** |
| 未承認の注文が期限を超えた | schedule |
| 在庫が閾値を下回った | **どちらでもない可能性がある** |

最後の1件が反例候補である。在庫の閾値割れは`Order.approve`が引き当てた結果として起きるが、
`Order.approve`が常にそれを起こすわけではない。occurrenceをactionへ束ねると、observableな事実は
「承認された」までしか言えず、「閾値を下回った」は表現できない。これは
**述語の成立が変化したこと自体をobservable occurrenceにできるか**という問いであり、
`onHand < threshold`を書ける式レイヤがなければ判断できない。

したがってProbe 1の結論は「まだ閉じられない」であり、決定は式レイヤの後に置く。ただし
`action実行`と`schedule`の2つが必要であることは、この例で確定している。

## Probe 2 — `changes`と`effect`の境界

承認は3つの結果を持つ。どちらに置くべきかを1件ずつ判定する。

| 結果 | 実装上rollback可能か | 意味としてrollbackすべきか | 判定 |
| --- | --- | --- | --- |
| `StockItem.reserved`の増加 | 可能 | すべき | `changes` |
| 監査記録 | 可能（同じDBに置ける） | すべき（承認が失敗したなら承認記録も残らない） | `changes` |
| 顧客への承認通知 | 不可能 | — | `effect` |

この表から、以前検討した「atomic boundaryの内側か外側か」という基準は**判定に使えない**ことが
分かる。監査記録はrollback可能なので、実装上の性質だけを見ると`changes`にも`effect`にも置ける。
判定を決めているのは2列目ではなく3列目である。

したがって基準は意味の側に置く。

```text
effect = そのoccurrenceが取り消されても取り消せない、外向きの結果
```

「取り消せるか」という能力ではなく「取り消すべきか」という意味で分ける。これは
`list User`がHTML tableを意味しないのと同じで、実装場所をForma semanticsの判定基準にしない
という既存の方針とも一致する。

なおこの基準では、監査記録は`entity`と`changes`で表す。監査を`effect`にしたくなるのは
「アプリケーションのdomainではない」という直感からだが、その直感は責務の分離であって
semanticsの区別ではない。

## Probe 3 — atomic boundaryは単一entityに収まらない

承認は`Order.status`と`StockItem.reserved`を同時に変える。v0の`action Entity.name: A -> B`は
単一entityのstateだけを対象にしており、複数entityの同時変更を表す場所がない。

「1回のaction実行 = 1つのatomic boundary」という方針を維持するなら、`changes`は
**宣言したaction 1つが、複数entityにまたがる事後条件を持てる**必要がある。これはv0の
action構文の拡張であって、新しいプリミティブではない可能性が高い。

同時に、在庫引当は`reserved <= onHand`というinvariantをpost-stateで満たさなければならない。
つまりatomic boundary、事後条件、invariantの3つは同時に設計する必要がある。

## Probe 4 — compilation unit境界を固定した

`examples/`へ2つ目の例を置いた時点で、次が失敗した。

```text
$ forma check examples/
examples/users.forma:3:6: error[F2001]: duplicate role `admin`
help: the first declaration is at examples/orders.forma:5
```

`forma check`は渡されたすべての`.forma`を再帰的に集め、1つのprogramとしてcompileする。
これはcross-file resolutionの意図どおりだったが、このprobeの時点では
**何がcompilation unitの境界かが仕様に書かれていなかった**。

`examples/users.forma`と`examples/orders.forma`はそれぞれ独立したapplicationなので、
個別に指定すれば通る。しかしdirectoryを渡すと1つのapplicationとして解釈される。
この穴は[`v0-primitives.md`](v0-primitives.md) §2.1で次のように固定した。

- 1回のcompile operationへ明示的に渡したsource集合が1 compilation unit、1 application namespace。
- file/directory引数は1つの集合へunionし、directory構造からapplicationやmodule境界を推測しない。
- 1 repositoryに複数applicationを置いてよいが、それぞれ別のcompile operationとして指定する。
- `forma check`はsource引数を必須とし、引数なしの暗黙current-directory探索を行わない。
- Generation Requestを作るcallerは、対象にするsource集合を明示する。

したがって`forma check examples/`の失敗は、独立applicationを意図的に1 unitへ渡した結果である。
両exampleを個別にcheckする現在のREADMEの用法がcanonicalであり、directory layoutの変更は不要である。

## Effect emission identityへの要求

[`v0-primitives.md`](v0-primitives.md)のsemantic identityは`action/Order/approve`のように
**宣言**を指す。emission identityにはこれに加えて**実行**の区別が要る。

```text
emission identity = f(occurrence instance, occurrence declaration id, effect declaration id, binding site id)
```

後半3つはResolved Intentから決定的に導出できる。最初の1つはruntimeが持つ値である。ここでrepositoryの
UUID generatorやDB sequenceに意味を依存させてはならない。実装方式を変えると「同じ論理emissionが1回」
という判定まで変わるためである。

Acceptance Factsは**配信ではなくemission log上の事実**を要求する。coding agentはtarget repositoryの
testで同じemission identityを二度処理しないことを検査する。外部serviceの実配送回数はFormaの
application contractにしない。

## 式レイヤはeffectより先に必要になる

この例で必要になったものを並べると、effect単独では設計できないことが分かる。

- 承認のprecondition「在庫が足りる」 — 式
- invariant「`reserved <= onHand`」 — 式
- derived value「`Order.total`」 — 式
- effectのbinding「`recipient customer.email`」 — 式（最小形）
- occurrenceの候補「`onHand < threshold`」 — 式

`effect` / `on` proposalを書くには、少なくともfield参照とrelation traversalを含む式の最小形が
決まっている必要がある。したがってeffect proposalの前に、式レイヤの範囲を決める段階を1つ置く。
この最小形の候補は[Minimal Expression Layer Proposal](expression-proposal.md)に分離している。

## `changes`の意味論として決めること

statement languageへ退行させないために、次を明示する必要がある。

1. 右辺はすべてpre-stateから評価する（同時代入）。read-after-writeを許すと代入順序が意味を持つ。
2. 同じfieldへの二重代入はcompile errorにする。順序で解決しない。
3. `clock.now`はaction実行につき1回サンプルし、同一action内では同じ値とする。
4. invariantはpost-stateで、atomic boundaryのcommit前に検査する。
5. 1回のaction実行 = 1つのatomic boundary。複数entityを含む。

3はschedule triggerのtestに必要な仮想時計と同じ要件である。Acceptance Factsはclockの意味を示し、
具体的なclock injectionはcoding agentがrepositoryに合わせて実装する。

## 未決定事項

- 状態を変えないactionを、`action`の遷移を任意化して表すか、別の宣言にするか。
- occurrenceをactionから導出するか、`emits`で明示するか、独立したprimitiveにするか。
- 述語の成立変化をobservable occurrenceにできるか。できない場合、在庫閾値をどう表すか。
- `effect`のpayloadをentity参照で持つか、値のcopyで持つか。
- 1つのoccurrenceに複数の`on`があるときの発火順序を宣言順にするか、順序非依存にするか。
- effectの失敗がoccurrenceの成否に影響するか。しないなら失敗をどこで観測するか。
- derived valueをentity fieldとして宣言するか、`list`の`columns`側で表すか。
- snapshot fieldを`default`の拡張とするか、runtime由来fieldの一般構文とするか。
- inverse relationを宣言するか、片側から導出するか。
- schedule triggerのclock semanticsをAcceptance Factsへどう表すか。

## 決定前に書く比較例

注文承認だけで`effect` / `on`のgrammarを固定しない。少なくとも次を同じ候補syntaxとResolved Intentで記述する。

1. 注文承認 — `OrderApproved` → 承認通知（effect）＋ 監査記録（changes）＋ 在庫引当（複数entity changes）。
2. 会員登録 — `UserRegistered` → `VerificationEmail`。[identity probe](email-verified-membership-probe.md)と接続する。
3. 通知の再送 — 状態変化を伴わないoccurrenceから同じeffectを発生させる。
4. 期限超過の検出 — scheduleから発生するoccurrence。
5. 在庫閾値割れ — 述語の成立変化から発生するoccurrence。表現できないなら、その理由を記録する。

各例で次を確認する。

- target repositoryを知らなくても、発生条件と発生してはいけない条件を説明できるか。
- 同じeffectを複数のoccurrenceから発生させたとき、bindingが重複せずに書けるか。
- emission identityがResolved Intentから決定的に導出できるか。
- `changes`に制御構文を導入せずに書けるか。
- coding agentがframework固有lowering ruleなしに、異なるrepository contextへ実装できるか。

3と5は、「effectはすべて状態遷移に紐づければよい」という誤った一般化を防ぐために必ず含める。

この比較が終わるまでは、`effect`、`on`、`emit`、式をEBNF、10 primitives、reference compilerへ
追加しない。

## Roadmapへの影響

Milestone 6の「最初の追加例候補」は本書と[`examples/orders.forma`](../examples/orders.forma)で
着手した。probeの結果、検討順序に1段追加する必要がある。

```text
1. 式レイヤの最小形（field参照、relation traversal、比較、算術）
2. changesの事後条件semantics + invariant + atomic boundary
3. occurrence model（actionから導出するか、明示するか）
4. effect / on proposal
```

式レイヤを飛ばしてeffectを設計すると、bindingとpreconditionを書けないまま構文だけが決まる。
