# Eval results

**Status:** active · **Derived from:** phase runs · **Updated:** 2026-11-21

One file per phase run, named `<phase>-<date>.json`, each record shaped
`{id, pass, score, details, commit}`. Written by the `fitness-recorder` skill.

This directory is empty because no phase has run. An eval whose target path does not
exist is recorded as absent, never as a pass -- see
[../evals/evals.md](../evals/evals.md).
