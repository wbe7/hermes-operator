# Domain docs

This repository has one domain context.

## Before exploring

Read root `CONTEXT.md` for terminology and relevant files under `docs/adr/` for accepted architectural decisions. If a domain file is absent, proceed silently; `domain-modeling` creates documentation when terms and decisions are actually resolved.

## Layout

- `CONTEXT.md`: glossary only.
- `docs/adr/NNNN-slug.md`: numbered architectural decisions.

Use the glossary's canonical terms in issue titles, proposals, hypotheses, and test names. If a concept is missing, distinguish a real vocabulary gap from an unnecessary synonym and bring genuine gaps to `domain-modeling`.

Surface conflicts with an existing ADR explicitly, naming the ADR and explaining why it may need to be revisited. An open question in a design document is not an accepted decision.
