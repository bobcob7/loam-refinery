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
reader's own browser (§5.1). Every value it ever compares or displays
comes from a data-priority-css / data-category-css / data-lens
attribute — each already constrained, server-side, to an enum member or
an integer-derived token — or from the two small label tables below,
which are keyed on this renderer's own closed vocabulary (review-
document.md §8's four priority bands, §9's seven categories plus the
"other" fallback), never on anything a reviewer wrote. Every handler is
wired with addEventListener; no attribute in this document's markup
begins with "on".

Element.closest and CSS.escape are this script's two non-baseline DOM
APIs (§5.5) — both have shipped in every evergreen browser for years,
and are the first two things to check if this renderer is ever asked to
support a browser target older than "whatever ships today". closest
walks a <details> ancestor chain during anchor routing and expand/
collapse; CSS.escape turns an arbitrary fragment value into a valid CSS
ID-selector suffix ("#" + CSS.escape(id)) — the shape document.getElementById
tolerates unescaped but a selector string parsed by querySelector would
not, since a qualified id's leading "#" or embedded ":" would otherwise
be read as CSS selector syntax rather than as literal id characters.

Bead .6 owns this file's content. */ -}}
{{define "script"}}<script>
(function () {
document.documentElement.classList.add("js");

var FINDING_SELECTOR = "details.finding";

var PRIORITY_LABELS = {
"pri-mustfix": "Must fix",
"pri-shouldfix": "Should fix",
"pri-worthfixing": "Worth fixing",
"pri-optional": "Optional"
};
var PRIORITY_ORDER = ["pri-mustfix", "pri-shouldfix", "pri-worthfixing", "pri-optional"];

var CATEGORY_LABELS = {
"cat-correctness": "Correctness",
"cat-security": "Security",
"cat-performance": "Performance",
"cat-maintainability": "Maintainability",
"cat-testing": "Testing",
"cat-documentation": "Documentation",
"cat-style": "Style",
"cat-other": "Other"
};
var CATEGORY_ORDER = [
"cat-correctness", "cat-security", "cat-performance", "cat-maintainability",
"cat-testing", "cat-documentation", "cat-style", "cat-other"
];

var FACETS = [
{ key: "priority", attr: "data-priority-css", containerID: "facetPriority", labels: PRIORITY_LABELS, order: PRIORITY_ORDER },
{ key: "category", attr: "data-category-css", containerID: "facetCategory", labels: CATEGORY_LABELS, order: CATEGORY_ORDER },
{ key: "lens", attr: "data-lens", containerID: "facetLens", labels: null, order: null }
];

var active = { priority: {}, category: {}, lens: {} };
var activeCount = { priority: 0, category: 0, lens: 0 };

function findings() {
return Array.prototype.slice.call(document.querySelectorAll(FINDING_SELECTOR));
}

function byID(id) {
return document.getElementById(id);
}

function selectByID(id) {
if (!id) {
return null;
}
try {
return document.querySelector("#" + CSS.escape(id));
} catch (err) {
return null;
}
}

function resolveFragmentTarget(fragment) {
var raw = fragment;
var target = selectByID(raw);
if (target) {
return target;
}
var decoded = raw;
try {
decoded = decodeURIComponent(raw);
} catch (err) {
return null;
}
if (decoded === raw) {
return null;
}
return selectByID(decoded);
}

function openDetailsAncestors(el) {
var node = el.closest("details");
while (node) {
node.open = true;
node = node.parentElement ? node.parentElement.closest("details") : null;
}
}

function routeHash() {
var hash = location.hash;
if (!hash || hash.length < 2) {
return;
}
var target = resolveFragmentTarget(hash.slice(1));
if (!target) {
return;
}
openDetailsAncestors(target);
target.scrollIntoView();
}

function setAllOpen(open) {
findings().forEach(function (details) {
details.open = open;
});
}

function presentValues(attr) {
var seen = {};
var values = [];
findings().forEach(function (details) {
var value = details.getAttribute(attr);
if (value && !seen[value]) {
seen[value] = true;
values.push(value);
}
});
return values;
}

function orderedValues(facet) {
var present = presentValues(facet.attr);
if (!facet.order) {
return present.slice().sort();
}
return facet.order.filter(function (value) {
return present.indexOf(value) !== -1;
});
}

function toggleChip(facetKey, value, chip) {
if (active[facetKey][value]) {
delete active[facetKey][value];
activeCount[facetKey] -= 1;
chip.setAttribute("aria-pressed", "false");
} else {
active[facetKey][value] = true;
activeCount[facetKey] += 1;
chip.setAttribute("aria-pressed", "true");
}
applyFilters();
}

function makeChip(facet, value) {
var chip = document.createElement("button");
chip.type = "button";
chip.className = "chip tag";
if (facet.key === "priority" || facet.key === "category") {
chip.classList.add(value);
} else {
chip.classList.add("tag-profile");
}
chip.setAttribute("aria-pressed", "false");
chip.textContent = facet.labels && facet.labels[value] ? facet.labels[value] : value;
chip.addEventListener("click", function () {
toggleChip(facet.key, value, chip);
});
return chip;
}

function buildFacets() {
FACETS.forEach(function (facet) {
var container = byID(facet.containerID);
if (!container) {
return;
}
orderedValues(facet).forEach(function (value) {
container.appendChild(makeChip(facet, value));
});
});
}

function updateCount(visible, total) {
var el = byID("filterCount");
if (el) {
el.textContent = visible + " of " + total + " findings shown";
}
}

function updateBanner(anyActive) {
var el = byID("filterBanner");
if (!el) {
return;
}
if (anyActive) {
el.hidden = false;
el.textContent = "Some findings are hidden by the active filters.";
} else {
el.hidden = true;
el.textContent = "";
}
}

function passesFacet(details, facetKey, attr) {
var count = activeCount[facetKey];
if (count === 0) {
return true;
}
return !!active[facetKey][details.getAttribute(attr)];
}

function applyFilters() {
var list = findings();
var visible = 0;
list.forEach(function (details) {
var pass = passesFacet(details, "priority", "data-priority-css")
&& passesFacet(details, "category", "data-category-css")
&& passesFacet(details, "lens", "data-lens");
details.hidden = !pass;
if (pass) {
visible += 1;
}
});
var anyActive = activeCount.priority > 0 || activeCount.category > 0 || activeCount.lens > 0;
updateCount(visible, list.length);
updateBanner(anyActive);
}

function resetFilters() {
FACETS.forEach(function (facet) {
active[facet.key] = {};
activeCount[facet.key] = 0;
});
document.querySelectorAll(".chip[aria-pressed]").forEach(function (chip) {
chip.setAttribute("aria-pressed", "false");
});
applyFilters();
}

function wireToolbar() {
var expandBtn = byID("expandAllBtn");
if (expandBtn) {
expandBtn.addEventListener("click", function () {
setAllOpen(true);
});
}
var collapseBtn = byID("collapseAllBtn");
if (collapseBtn) {
collapseBtn.addEventListener("click", function () {
setAllOpen(false);
});
}
var resetBtn = byID("resetFiltersBtn");
if (resetBtn) {
resetBtn.addEventListener("click", resetFilters);
}
}

buildFacets();
wireToolbar();
applyFilters();
document.addEventListener("DOMContentLoaded", routeHash);
window.addEventListener("hashchange", routeHash);
})();
</script>{{end}}
