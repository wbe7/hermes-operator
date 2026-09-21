## Agent skills

### Issue tracker

Issues and specs live in GitHub Issues for `wbe7/hermes-operator`. Before reading, creating, or updating tickets, read `docs/agents/issue-tracker.md`.

### Triage labels

Use the five canonical triage roles. Before classifying tickets, read `docs/agents/triage-labels.md`.

### Domain docs

This is a single-context project: `CONTEXT.md` and `docs/adr/`. Before exploring the project, follow `docs/agents/domain.md`.

## Project design

Before proposing architecture or implementing the operator, read `docs/design/interview.md` for accepted requirements and open decisions. Consult `docs/research/upstream-and-isolation.md` when a decision depends on Hermes or Kubernetes behavior; distinguish source inspection from verified runtime behavior.

## Hermes image updates

Before changing the Hermes version, rebuilding its image, or publishing it, follow [Updating the Hermes image](README.md#updating-hermes-image). It covers version pins, runtime compatibility, multiarch verification, publication and rollout.
