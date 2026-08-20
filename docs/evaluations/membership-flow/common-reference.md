# Common Forma Reference for the Evaluation

このreferenceはA/B両条件へ同じ内容で配布し、task timer開始前に読む。特定applicationの答えは含めない。

- 1回のcompileへ渡したForma file群が1 applicationのsource of truthである。declaration順や最初に書かれたpageは
  applicationのdefault entryを意味しない。
- `action Entity.name: Source -> Destination`はdomain state transitionである。page内の`actions name`はそのactionを
  surfaceから起動する。
- page-localな`goto Page`は固定destinationである。form/interactionの`success Page`はoperation成功時のdestination、
  `stay`はsame contextである。
- Identity interactionの`continue Page`は、そのinteractionのsuccess destinationから1回だけ続くsurface transitionである。
  空pageだけでは新しいcontinue capabilityを得ない。
- Identityのregistration、verification、authentication blockはdomain operation、credential/evidence policy、eligible state、
  success action等を所有する。pageの`interact` blockは入力、feedback、navigation、accessを所有する。
- durable notice emissionと、application外で行われるdelivery/openは同じ保証ではない。
- compiler projectionはsourceから再生成するread-only viewである。projectionの図、table、textを編集してもsource semanticsは
  変わらない。projectionに無い逆命題を新しい保証として推論しない。
