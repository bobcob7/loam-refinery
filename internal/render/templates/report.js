{{- /* internal/render/templates/report.js — the one <script> element
this page ships (§5): anchor routing, expand-all/collapse-all, and
priority/category/lens filtering (§5.3) — static, every byte fixed at
compile time, never derived from a template action and never containing
anything read from the review data it sits beside (§5.1). Wrapped in
{{define "script"}}, including its own literal <script>...</script>
tags, with zero {{ }} actions inside — that is what keeps this
renderer's script-purity property true by construction rather than by
convention: there is no template.JS or template.JSStr anywhere in this
file for the static check §2.2.1 pins to find, because there is no
template action here at all for either type to wrap. Its only channel
into the page's data is reading data-* attributes and element content
report.gohtml's other partials already wrote, at runtime, in the
reader's own browser (§5.1).

Bead .6 owns this file's content. Left as an empty define here so
templates/report.gohtml's {{template "script"}} call resolves — and
renders nothing, so no <script> element ships until bead .6 gives it
one — without bead .6 having to touch report.gohtml. */ -}}
{{define "script"}}{{end}}
