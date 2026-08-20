---
name: legacy-code-archaeology
mode: reverse
summary: Understand undocumented source code you inherited rather than compiled artifacts.
---

# Legacy code archaeology

Not every reverse-engineering target is a binary. Inherited source with no documentation, no tests, and no surviving author is the same problem in a friendlier format: recover intent from artifacts.

## Start with the boundaries, not the code

A large unfamiliar codebase cannot be read front to back. Find its edges first:

- **Entry points.** `main`, HTTP route registrations, message-queue consumers, cron definitions, CLI subcommands. These are where behavior begins.
- **Data model.** Schema files, migrations, ORM definitions. The data model constrains everything and changes more slowly than the code around it.
- **External interfaces.** Outbound HTTP calls, database queries, file paths, environment variables. These define what the system touches.
- **Configuration.** Often the fastest map of features, since every toggle names one.

## Use the history

Version control is a recording of intent that source alone lacks:

- Commit messages explain *why* a line exists, which the line cannot.
- `blame` on a strange condition usually leads to a bug fix, and the message names the bug.
- The issue or ticket referenced in a commit is documentation someone already wrote.
- Code that has not changed in years is either stable or dead — check whether it is reachable before assuming the former.

A puzzling construct is often a fix for a real failure. Find the commit before removing it.

## Read tests as specification

Existing tests, however sparse, state what someone believed the code should do. They also reveal the intended usage of an API more accurately than any comment.

Where tests are absent, writing one is how you verify your understanding: pin the current behavior in a test, then refactor against it. A test that captures behavior you do not yet endorse is still valuable — it tells you when you changed something.

## Establish behavior empirically

Reading tells you what the code can do; running tells you what it does. Add logging at the boundaries, run a real workload, and observe. For a system you cannot run, tracing a single request through the code by hand — every branch, in order — is slower but conclusive.

Instrument before refactoring. Changing code you do not understand, without a way to observe the difference, is how outages happen.

## Distinguish accident from intent

Legacy code mixes deliberate design with accumulated accident. Signals of intent: consistency across several places, a comment explaining a tradeoff, a test asserting it, a commit message describing it. Signals of accident: one-off inconsistency, dead branches, duplicated logic that has drifted apart, defensive checks for conditions that cannot occur.

Preserve intent; clean up accident. Confusing the two either breaks working behavior or enshrines a bug.

## Document as you go

Write down the map you are building — entry points, data flow, module responsibilities, and the surprises. This is the artifact that outlasts your reading and the thing the next person needs. See `notes-and-reporting` for structure.

Record open questions explicitly. "Unclear why X is retried three times" is more useful than silence, and someone may know.

## When to stop reading and start changing

You do not need complete understanding to make a change — you need enough to bound the blast radius. That means knowing what calls the code you are touching, what it calls, what data it mutates, and how you will know if you broke something. Once those four are answered for your specific change, proceed; full comprehension of a large system is a goal you will not reach and do not need.
