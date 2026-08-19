title: Effort and blast radius
aliases: tradeoffs, sizing
related: effort, scope, suggestions, broad-scope-alone
---
Every suggestion carries two independent ordinals, and keeping them apart is the
point.

effort is how much work: trivial, small, medium, large. scope is blast radius,
what else has to be re-read or re-tested: line, block, file, module, project.

They come apart constantly: one shared constant is trivial effort with project
blast radius, a gnarly parser internal large effort with block radius. An
orchestrator triaging needs both:

                narrow radius     wide radius
  low effort    apply it          cheap but far-reaching; check first
  high effort   schedule it       escalate; this is its own change

scope sits on the suggestion because one finding can be answered at several
radii: patch this call site, or change the type so the mistake stops being
expressible. The choice belongs to the caller, which is why a lone module or
project suggestion raises broad-scope-alone.
