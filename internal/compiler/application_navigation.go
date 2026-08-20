package compiler

import "fmt"

// checkApplicationNavigation validates the navigation semantics that are not
// owned by an operation: the application's default entry and page-local
// surface transitions.
func (c *checker) checkApplicationNavigation() {
	for index, entry := range c.program.Entries {
		if index > 0 {
			first := c.program.Entries[0]
			c.error(entry.Span, "F2504", "duplicate application `entry` declaration", fmt.Sprintf("the first entry is at %s:%d; declare at most one default entry", first.Span.File, first.Span.Start.Line))
			continue
		}
		c.checkUnboundNavigationTarget(entry.Page, "application entry", "choose a parameterless page as the application entry")
	}

	for _, page := range c.program.Pages {
		if c.pages[page.Name.Text] != page {
			continue
		}
		seen := map[string]Span{}
		for _, transition := range page.SurfaceTransitions {
			if previous, exists := seen[transition.Kind]; exists {
				c.error(transition.Span, "F2505", fmt.Sprintf("duplicate `%s` transition on page `%s`", transition.Kind, page.Name.Text), fmt.Sprintf("the first transition is at line %d; a page has at most one transition of each kind", previous.Start.Line))
				continue
			}
			seen[transition.Kind] = transition.Span
			c.checkUnboundNavigationTarget(transition.Destination, "surface transition", "choose a parameterless page because a surface-only transition carries no entity binding")
		}
	}

	// The legacy interaction-local continuation is still accepted, but the
	// success page may not also own the same continuation. That would create two
	// sources of truth for one visible capability.
	for _, page := range c.program.Pages {
		for _, interaction := range page.IdentityInteractions {
			if interaction.Continuation == nil || interaction.SuccessPage == nil {
				continue
			}
			successPage := c.pages[interaction.SuccessPage.Text]
			if successPage == nil {
				continue
			}
			for _, transition := range successPage.SurfaceTransitions {
				if transition.Kind == "continue" {
					c.error(interaction.Continuation.Span, "F2505", fmt.Sprintf("continuation from page `%s` is declared twice", successPage.Name.Text), "keep the page-local `continue` and remove the interaction-local continuation")
				}
			}
		}
	}
}

func (c *checker) checkUnboundNavigationTarget(target Name, owner, hint string) {
	page := c.pages[target.Text]
	if page == nil {
		c.error(target.Span, "F2504", fmt.Sprintf("%s references unknown page `%s`", owner, target.Text), "declare the page or choose an existing page")
		return
	}
	if page.Param != nil {
		c.error(target.Span, "F2504", fmt.Sprintf("%s cannot enter parameterized page `%s` without an entity binding", owner, target.Text), hint)
	}
}
