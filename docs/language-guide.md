# Writing Forma for the Alpha

This guide teaches an AI or a person to write source accepted by the
`v0.1.0-alpha.2` reference front-end. The alpha language profile is the
normative boundary. If this guide and that profile differ, the profile and the
installed compiler win.

## Authoring protocol

When translating a natural-language application request into Forma:

1. Express application meaning: data, constraints, states, actions, pages,
   access, and navigation.
2. Do not prescribe framework components, HTTP routes, database tables, file
   paths, CSS, or test libraries in Forma. Those belong to the target
   repository or an Implementation Policy Manifest.
3. Do not invent syntax. The alpha deliberately rejects constructs outside the
   forms shown here.
4. Preserve unresolved product decisions as questions for the person instead
   of silently choosing domain behavior.
5. Write the proposed source to one or more `.forma` files and run
   `forma check <file-or-directory>`. Correct diagnostics before generation.

One explicit source set is one application namespace. Declaration order does
not change resolution. `//` starts a line comment. A declaration body opens
with `{` followed by a newline, and each member is written on its own line. The
compiler rejects a declaration body written on one line.

## Core declarations

Declare roles and the initial parameterless page when the application needs
them:

```forma
role admin
role member

entry Home
```

The built-in scalar types are `String`, `Int`, `Decimal`, `Bool`, `Date`, and
`DateTime`. A named scalar type may directly refine a built-in with the
constraints accepted by the compiler:

```forma
type Email    = String matches /.+@.+/
type Quantity = Int min 0
type Score    = Decimal min 0 max 100
```

Do not rely on inherited constraint composition through another named type in
this alpha.

An entity contains scalar fields, to-one relations, to-many relations, and at
most one state declaration:

```forma
entity Task {
    title    String required label
    project  Project required
    assignee User
    tags     [Tag]

    state status Planned | Active | Done initial Planned
}
```

Available field modifiers are `required`, `unique`, `readonly`, `default`, and
`label`. A to-one relation is an entity-typed field. A to-many relation uses
`[Entity]`. Do not invent separate relation declarations.

A domain action changes the single state owned by its entity:

```forma
action Task.start:    Planned -> Active allow admin, member
action Task.complete: Active  -> Done confirm allow admin, member
```

`confirm` requires confirmation before dispatch. `allow` belongs to that
action. Standard `create`, `view`, `edit`, and `delete` actions do not need
top-level declarations; `delete` always requires confirmation. The alpha does
not specify cascade, detach, or restrict behavior for related records. Preserve
that as a human decision instead of inventing a destructive cascade.

## Pages and views

A page is parameterless or has one entity parameter. A list uses the entity
type; detail and edit forms use the page parameter:

```forma
page Tasks {
    allow admin, member

    list Task {
        columns title, project, assignee, status
        search title
        filter project, assignee, status
        sort title asc
        paginate 20
        actions create, view, edit, delete, start, complete
    }
}

page TaskDetail(task Task) {
    allow admin, member

    detail task {
        fields title, project, assignee, status
        actions edit, delete, start, complete
    }
}

page TaskCreate {
    allow admin, member

    form Task {
        fields title, project, assignee
        submit create
    }
}

page TaskEdit(task Task) {
    allow admin, member

    form task {
        fields title, project, assignee
        submit edit
    }
}
```

List options are explicit columns, search fields, filter fields, one stable
sort, bounded pagination, and contextual actions. A form owns either
`submit create` or `submit edit`. Page names do not define URLs or components.

A page-local transition performs no domain operation and targets a fixed
parameterless page:

```forma
page Welcome {
    continue Tasks
}
```

When standard action destinations are ambiguous, use the compiler-supported
explicit `goto` modifier rather than relying on a page-name guess.

## Access

`allow` lists the roles that may access a page or invoke an action. Forma
composes the source page, action, and destination access rules at a presented
surface. Do not treat an omitted page rule as an instruction to invent a role.

The Identity facet additionally supports the closed email-verified membership
shape demonstrated by the second complete example bundled below.
It describes identifier canonicalization, local-password proof rules,
registration, verification and resend, signin and signout, ownership, and
authenticated/owner page requirements. It is not a general OAuth, session, or
credential-provider language. When that exact implemented shape is
insufficient, report the gap instead of approximating it with new keywords.

## Experimental domain behavior included in the alpha

An entity may declare a self-only invariant with one `<=` comparison:

```forma
entity StockItem {
    onHand   Quantity required
    reserved Quantity required

    invariant stockAvailable: reserved <= onHand
}
```

An explicit domain action may have at most one named precondition and one
change assignment. A supported expression reads `self` or one required to-one
relation from one consistent pre-state. Exact numeric `+` is available only in
the bounded cases accepted by the compiler:

```forma
action Reservation.commit: Pending -> Committed confirm allow staff {
    precondition withinLimit: stock.reserved + requested <= plan.limit

    changes {
        stock.reserved = stock.reserved + plan.approved
    }
}
```

Do not write multiple assignments, arbitrary arithmetic or boolean logic,
nested expressions, optional or multi-hop traversal, collection expressions,
record creation from actions, Derived Value, Occurrence, or Effect. Those are
post-alpha work.

## Validation loop

Return plain Forma source when the caller asks for a `.forma` file. Then use:

```text
forma check app.forma
forma project flow app.forma
forma request app.forma
```

`check` is authoritative for accepted syntax and static semantics. `project
flow` is a human-readable review view. `request` produces the structured input
for the implementation agent. A successful parse alone is not permission to
ignore compiler diagnostics or unsupported product requirements.
