title: Comment ids and grouping
aliases: comment-ids, slugs
related: id, id-unique, id-grouping
---
Every comment carries a reviewer-authored id: a kebab-case slug naming the kind
of finding, then a numeric suffix — missing-context-2, unchecked-error-1.

The slug groups and the suffix addresses. Four findings of the same kind share a
slug, so a consumer collapses missing-context-1 through -4 into one theme
without reading four bodies; the suffix means an orchestrator can say "resolve
missing-context-2" and both sides know which finding is meant.

There is no controlled vocabulary. Naming the finding is part of reviewing:
unchecked-error groups with other unchecked errors, where client-go-1 and
issue-1 group with nothing.

Ids are unique within a document — a duplicate makes one finding unaddressable,
which is a structural error. Suffixes within a slug run contiguously from 1; a
gap raises id-grouping, because a consumer cannot tell a deliberate gap from a
lost finding.
