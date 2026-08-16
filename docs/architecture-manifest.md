# Repository and Architecture Context

Status: superseded exploration — not part of the Forma grammar or core roadmap

この文書は以前、Application Profile、Deployment Profile、capability matrix、`forma.lock`を持つ
Architecture Manifestを提案していた。その設計は、Forma自身がframework別generatorを所有する前提へ
寄りすぎるため撤回する。

現在の実行モデルは[`agent-generation.md`](agent-generation.md)に定める。

```text
Resolved Intent + Acceptance Facts
  → AI coding agent + existing repository
  → repository-native implementation
```

## Architectureを誰が決めるか

coding agentは、まず対象repositoryを調べる。

- 使用中のruntimeとframework
- databaseとmigration方式
- component、route、API、persistenceの既存pattern
- test、lint、build command
- dependencyとversion policy
- deploymentやsecurityに関するrepository policy

既存repositoryがないgreenfieldの場合は、user instructionと明示的な制約をもとにagentが候補を提示し、
人間が承認する。選択結果をForma compilerのprofile registryへ登録する必要はない。

## source of truthの分担

| source | 所有する判断 |
| --- | --- |
| `*.forma` | entity、state、action、permission、pageなどのapplication intent |
| repository code/config | framework、route、database、library、deploymentなどの実装 |
| repository policyまたはuser instruction | 実装上守る制約、禁止事項、品質条件 |
| Resolved Intent / Acceptance Facts | agentへ渡す解決済み要求と検査すべき意味 |

同じ判断を二重に宣言しない。たとえばpermissionはFormaが所有し、repositoryはその実装だけを持つ。
一方、Rails、PostgreSQL、AWSの選択はapplication semanticsではないため、`.forma`へ入れない。

## 明示的なarchitecture constraints

greenfieldや大きなarchitecture変更では、agentへ次のような制約を渡すことがある。

```text
- use the repository's existing Ruby on Rails application
- keep PostgreSQL and the current migration tool
- do not introduce a second authorization library
- deploy only through the existing CI workflow
```

これらは自然言語でもrepository policy fileでもよい。Forma languageの意味としてparse・type-checkする
対象ではない。将来、同じconstraintが多くのprojectで繰り返され、machine-readableである必要が実例から
確認できた場合だけ、Generation Requestの補助schemaとして検討する。

## dependency

Formaはlibrary packageとsemantic capabilityのregistryを持たない。agentはrepositoryの既存dependencyを
優先し、新規dependencyが必要なら通常のcode reviewで理由を示す。lockfileは対象ecosystemの標準toolが
所有する。

## feedback

選んだarchitectureでForma intentを実装できなかった場合、compilerがprofile incompatibilityを返すのでは
なく、agentが次を報告する。

- 実装できなかったintent node
- repository上の制約または衝突
- 失敗したbuild/test commandとdiagnostic
- repository変更、constraint変更、Forma source変更のどれが必要か

この情報から人間が判断する。agentが未宣言のapplication behaviorを追加したり、Forma constraintを
黙って弱めたりしてはならない。

## 検証方法

新しいmanifest schemaを設計する前に、次を実測する。

1. architectureが存在するrepositoryへGeneration Requestを渡す。
2. agentが追加のprofile metadataなしで既存patternを再利用できるか確認する。
3. greenfield repositoryでは、最小限のuser constraintから実装planを提案できるか確認する。
4. Forma変更をincrementalに適用しても、手書きcodeとrepository policyを保てるか確認する。

このprobeで不足が確認されるまでは、`forma.arch.toml`、Application Profile、Deployment Profile、
capability registry、architecture build keyを復活させない。
