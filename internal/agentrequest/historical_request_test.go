package agentrequest

import (
	"testing"

	"github.com/horizon67/forma/internal/compiler"
)

// A v0.4 page could host both the detail and the edit form for one entity, so
// the edit reference recorded the same page as target and success. Source nodes
// are deduplicated, so that page appears once and must survive the upgrade that
// drops the duplicate success record.
func hasSuffix(value, suffix string) bool {
	return len(value) >= len(suffix) && value[len(value)-len(suffix):] == suffix
}

func TestHistoricalUpgradeKeepsSharedTargetAndSuccessPage(t *testing.T) {
	const source = `role admin

entity User {
    name String required label
}

page Users {
    allow admin
    list User {
        columns name
        actions view, edit
    }
}

page UserPage(user User) {
    allow admin
    detail user {
        fields name
    }
    form user {
        fields name
        submit edit
    }
}
`
	result := compiler.Compile([]compiler.SourceFile{compiler.NewSourceFile("shared.forma", source)})
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	facts, err := compiler.BuildAcceptanceFacts(result.Intent)
	if err != nil {
		t.Fatal(err)
	}

	// Rebuild the pre-v0.7 shape: the reference carried the success page and the
	// navigation fact repeated it.
	historical := Request{
		Schema: PreviousRequestSchema, ResolvedIntent: result.Intent,
		AcceptanceFacts: facts, SourceMap: result.SourceMap,
	}
	clearKind := func(access *compiler.IRAccess) {
		for index := range access.AllOf {
			access.AllOf[index].Kind = ""
		}
	}
	var sharedPage string
	for pageIndex := range historical.ResolvedIntent.Pages {
		page := &historical.ResolvedIntent.Pages[pageIndex]
		for viewIndex := range page.Views {
			view := &page.Views[viewIndex]
			if view.Submit != nil {
				clearKind(&view.Submit.Access)
			}
			for actionIndex := range view.Actions {
				action := &view.Actions[actionIndex]
				clearKind(&action.Access)
				if action.Name == "edit" && action.Kind == "standard" {
					action.SuccessPage = action.TargetPage
					sharedPage = action.TargetPage
				}
			}
		}
	}
	if sharedPage != "UserPage" {
		t.Fatalf("edit target = %q, want the shared page", sharedPage)
	}
	for factIndex := range historical.AcceptanceFacts.Facts {
		fact := &historical.AcceptanceFacts.Facts[factIndex]
		if fact.Kind == "navigation" && fact.Expected.Navigation != nil &&
			fact.Expected.Navigation.TargetPage == compiler.SemanticID("page/"+sharedPage) &&
			fact.Expected.Navigation.SuccessPage == "" && hasSuffix(string(fact.Subject), "/action/edit") {
			fact.Expected.Navigation.SuccessKind = "page"
			fact.Expected.Navigation.SuccessPage = compiler.SemanticID("page/" + sharedPage)
		}
	}
	historical.ResolvedIntent.Version = historicalResolvedIntentVersion
	historical.AcceptanceFacts.Version = historicalAcceptanceFactsVersion
	historical.AcceptanceFacts.IntentVersion = historicalResolvedIntentVersion
	historical.SourceMap.Version = historicalSourceMapVersion
	historical.SourceMap.IntentVersion = historicalResolvedIntentVersion

	outputs, err := upgradeHistoricalCompilerOutputs(historical)
	if err != nil {
		t.Fatalf("upgrade shared-page historical request: %v", err)
	}
	canonical, err := compiler.BuildAcceptanceFacts(outputs.intent)
	if err != nil {
		t.Fatal(err)
	}
	if !canonicalJSONEqual(canonical, outputs.facts) {
		t.Fatal("upgraded facts diverge from canonical facts when target and success share a page")
	}
}
