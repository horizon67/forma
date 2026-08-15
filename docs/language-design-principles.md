# Forma Language Design Principles

Status: design guide

この文書は、Formaの構文案を「短いか」「自然言語らしいか」ではなく、人間が正確に、安心して、
長く読めるかで評価するための設計指針である。言語の規範的なsyntaxとsemanticsは
[`v0-primitives.md`](v0-primitives.md)に定める。

## 想定する読者

Formaは自然言語でも、非programmer向けのno-code notationでもない。想定する読者は、domainと
applicationの構造を考えられるsoftware builderである。entity、state、validation、permissionなどの
概念は理解してよいが、生成先のReact、Go、SQL、HTTP、database schemaを知らなくてもForma sourceを
理解できなければならない。

## 可読性contract

Forma sourceを読んだ人が、target実装を調べずに次へ答えられることを目標とする。

1. 何が存在するか。
2. 誰が何を見て、何を変更できるか。
3. どのconstraint、state、transitionが適用されるか。
4. 要求を変えるにはどの宣言を変更するか。
5. diffによってapplicationの観測可能な意味がどう変わるか。

## 設計原則

### 1. Intentを直接表す

application固有の判断を、frameworkやtransportの語彙から逆算させない。`entity`、`state`、`action`、
`list`のように、読者が考えるapplication概念を直接表す。

### 2. 同じ概念は同じ形で表す

一つの意味に複数のcanonical syntaxを与えない。convenience syntaxを検討する場合も、少数の
canonical semantic nodeへ一意に正規化でき、formatterが一つの表記へ戻せることを条件とする。

### 3. 短さではなく意味密度を上げる

実装詳細は省略してよいが、applicationの意味を決める事実まで隠してはならない。文字数が少なくても、
外部のconventionを知らなければ挙動を説明できないsyntaxは高密度とはみなさない。

### 4. 暗黙性を閉じ、説明可能にする

defaultと自動解決は、閉じた規則から決定的に導出できる場合だけ許す。導出結果はSemantic IRへ記録し、
compilerから人間向けに表示できなければならない。target profileやAI generatorに推測させない。

### 5. 依存を明示的に追跡できるようにする

すべてを同じfileへ置く必要はない。ただし、ある宣言の意味へ影響するtype、role、action、pageなどを
名前解決とSource Mapから機械的に列挙できなければならない。file配置や実行時reflectionだけで意味を
変えてはならない。

### 6. 変更の意味を読めるようにする

小さなtext変更が常に小さな影響になるとは限らない。permissionの1語変更が多くの利用者へ影響する
こともある。重要なのは影響を小さく見せることではなく、Semantic IRとconformance contractの差分から
正確な影響を確認できることである。

### 7. 高水準でも原因を追えるようにする

diagnostic、conformance failure、generated artifact上の重要なbehaviorは、対応するForma declarationへ
戻れなければならない。抽象化はtarget codeを読まなくてよくするためのもので、原因を隠すためのものでは
ない。

## `examples/users.forma`の可読性監査

完全例を、構文の見た目ではなく上記contractに照らして確認した。

| 対象 | 監査結果 | 判断 |
| --- | --- | --- |
| `role admin` | 認可主体が独立して見える | 維持 |
| `type Email = String matches ...` | domain typeとconstraintを一箇所で読める | 維持 |
| `name String required label` | relation表示で使うfieldを推測させない | `label`の意味を仕様で限定し、構文は維持 |
| `team Team` | to-one relationがfieldと同じ形で読める | 維持 |
| `state status ...` | state名は明示的だが、先頭値を初期値とする規則は重要な意味を順序へ隠していた | `initial Pending`を必須化 |
| `action User.activate: Confirmed -> Active` | preconditionと結果を一行で直接読める | 維持 |
| actionの`confirm allow admin` | modifierの役割は位置から判別できる。`confirm`はaction名にも使えるが文法上の役割は一意 | 維持し、canonical orderをformatterで固定する候補とする |
| 各pageの`allow admin` | repetitionはあるが、pageを局所的に読んで認可を判断できる | 共通group構文を追加せず維持 |
| listの`columns/search/filter/sort/paginate/actions` | 提示能力とquery能力が直接読める | 維持 |
| 標準actionとdomain actionの混在 | 解決規則は閉じているが、読者は参照先を確認することがある | 構文は維持し、解決済み表示をcompiler projectionへ含める |
| `form User` / `form user` | entityとbindingの区別はcaseだけでなく、page parameterと`submit create/edit`から確認できる | 新しいform mode構文は追加せず維持 |
| `submit create/edit` | modeと整合する冗長な記述が誤読と誤指定を検査できる | canonical exampleでは明示する |
| 標準actionの成功後navigation | source上は省略されるが、仕様から一意に解決されSemantic IRへ記録される | 維持し、人間向けの展開表示をfront-end要件にする |

今回の監査では、10個のプリミティブを増やす必要はなかった。変更したのは、作成時の挙動を決める
初期stateを明示することだけである。

## 新しい構文を追加する前のchecklist

- 新しいapplication概念を表すのか、既存概念の別表記にすぎないか。
- 同じ意味を既存の構文でも書けるなら、canonical formが二つにならないか。
- 省略されるのは実装詳細か、利用者から観測できるapplication semanticsか。
- 宣言だけを読んで、適用されるconstraintとdependencyを予測できるか。
- compilerは省略された意味と参照解決の理由を表示できるか。
- target frameworkの用語がSemantic IRへ漏れていないか。
- 同じ変更を数か月後にreviewしても、意味の差分を説明できるか。

判断に迷う場合は、同じSemantic IRを表す候補を複数作り、説明、誤り発見、変更、影響予測、再読の
taskで比較する。主観的な「美しさ」だけを採用理由にしない。
