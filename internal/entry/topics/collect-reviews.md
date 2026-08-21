title: Combining stored reviews
aliases: collecting, combined-reviews
related: store, id
---
`collect-reviews --ref=SHA` combines every stored submission for one ref
into one object: `ref`, `repo`, `store`, `head_check`, `total`,
`submissions`, `comments` — seven fields, none a review-document field,
because this is not one.

Two submissions can share an `id`. Each comment's id is qualified to stay
unique: `profile:origin_id` when its submission is current for that
profile, `#ordinal:origin_id` otherwise — no profile claimed, or
superseded. `ordinal` is positional and shifts on the next submission;
treat a `#N:id` form as good for one call only.

Resubmitting under the same profile supersedes (`topic:store`): the
earlier submission gets `superseded_by`, naming the superseding ordinal.
Nothing is ever dropped — filter out anything carrying `superseded_by` for
current opinions only.

`--format markdown` renders the same data for a human, or a PR comment;
`--format json` is the default. Neither validates as a review document.
