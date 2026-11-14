---
name: ralph-upgrade-note
description: Write or check an upgrade note, the entry in docs/UPGRADING.md that tells a user what to run after an update. Use when a change moves a state file, renames a config key, removes a command or flag, or needs a command run once; when asked "does this need an upgrade note"; or when reviewing an entry someone else wrote.
---

# Upgrade notes

An upgrade note tells a user what to do after they update. This skill turns
one change into one entry, the same way every time: fill in the input, decide
whether an entry is needed, write the output, then check it.

## 1. Fill in the input

Collect these facts from the diff and the pull request before writing a word
of the entry. A field you cannot fill is a question to answer, not a blank to
skip.

```yaml
repo: ralph                # the repo the change lands in
pr: 69                     # pull request number
title: State files follow the config file   # names the change, no verb tense games
change: >                  # what is different for the user, one or two sentences
  ralph keeps its state in the directory that holds the config file.
  Before, the state was always in ~/.config/ralph.
who: >                     # "everyone", or the condition that makes the step needed
  only when the config file is not ~/.config/ralph/config.toml
when: before the first ralph up   # order constraint, or "any time"
steps:                     # commands a user can paste, in order
  - run: export RALPH_STATE_DIR=~/.config/ralph
    note: in your shell rc file      # optional
alternatives: []           # other step lists, when the user picks one of several
skipped: >                 # what happens to a user who does not do the step
  the first run starts from empty state: builds rerun, cleanup removes nothing
verify:
  command: ralph config
  expect: the state dir line names the directory you chose, and no warning prints
  passes_when_already_done: true     # must be true; rewrite the check if not
detectable: true           # can ralph doctor see a machine that missed the step?
```

## 2. Decide whether an entry is needed

An entry is needed when a user who only updates the binary ends up with a
setup that is broken, different, or carrying leftovers. In practice:

| The change | Entry |
|---|---|
| Moves or renames a state file, a directory, or a generated file | yes |
| Renames or removes a config key, a command, or a flag | yes |
| Changes a default so an existing setup behaves differently | yes |
| Needs a command run once (an import, a rebuild, a cleanup run) | yes |
| Leaves something behind that ralph will not remove | yes |
| Adds a command, a flag, or a config key with a safe default | no |
| Fixes a bug with no change to files or config | no |
| Changes docs, tests, or internals only | no |

When the answer is no, stop and report `needed: false` with the reason.

## 3. Write the output

The entry goes under `## Unreleased` in `docs/UPGRADING.md`, newest first. It
has a `###` title and three labelled paragraphs, in this order:

```markdown
### <title>

Change: [#<pr>](https://github.com/mad01/ralph/pull/<pr>). <change>

Run: <who>. <skipped> <when>:

- <step or alternative>
- <step or alternative>

Verify: `<verify.command>` <verify.expect>.
```

Rules the entry must meet:

1. `Change:`, `Run:` and `Verify:` each start a line, appear once, and come in
   that order.
2. `Change:` names the pull request as `#<number>`.
3. `Run:` says who needs the step when it is not everyone, and gives commands
   to paste, not a description of them.
4. `Verify:` is a command plus the output to look for, and it also passes for
   a user who already did the step.
5. The entry sits under `## Unreleased`, which is the first release section.
6. No em dashes, short sentences, the same word for the same thing.

Then report the result in this shape:

```yaml
needed: true
reason: the state directory moves for users with RALPH_CONFIG set
file: docs/UPGRADING.md
section: Unreleased
entry: |
  ### State files follow the config file
  ...
pr_body: the same steps, in one or two lines
doctor_check: exists | added | not detectable
checked: go test ./internal/upgradenotes/ passes
```

## 4. Check it

In the ralph repo, the form is checked by a test that reads the real page:

```bash
go test ./internal/upgradenotes/
```

It fails with the line and the rule when an entry breaks rules 1, 2 or 5. CI
runs it on every pull request. Rules 3, 4 and 6 need a reader: run the entry's
`Run` and `Verify` lines in a scratch setup (see "Running ralph safely" in the
repo's `CLAUDE.md`) and confirm `Verify` fails before the step and passes
after it.

`internal/upgradenotes` holds the data structure the test uses: `Parse`
returns the entries (`Release`, `Title`, `Change`, `Run`, `Verify`, `Line`),
and `Check` returns the problems.

## Reviewing an entry someone else wrote

Rebuild the input from the entry. Every field in step 1 must be answerable
from the entry text alone. A field you cannot fill is the review finding:
"the entry does not say who needs this", "Verify gives no expected output".

## Other repos

A repo that deploys from its main branch keys entries by date and adds a
`Since:` line, because its users never see a release. The input gains three
fields and the entry two parts:

```yaml
date: 2026-11-14
components: [belt]         # the heading reads "## <date> <components>: <title>"
since: {belt: unreleased}  # unreleased | a version | "main only"
steps_without_ralph: []    # only when they differ from steps
```

Follow that repo's own `docs/UPGRADING.md` for the exact form. Its entries are
not covered by the test above.
