# Fresh smoke acceptance — 2026-09-21

Operator `0.1.0` via Helm on Berger Apps (amd64), original unpatched Hermes
runtime `v2026.9.14` in `wbe7/hermes`, image index
`sha256:feecb5d71f4876758b61e83cf1527fa6cdbb4f7e8919808c5eed7daf5085640f`.
This bounded manual acceptance uses only the explicitly authorized Telegram bot
`@HermesOperatorK8SBot`; no other contact or group was used. No second bot token
was available. The [full ledger](v1-acceptance.md) retains the unexecuted gates.

## Clean installation

The previous `hermes-operator-test/hermes-smoke` CR was deleted normally. Its
existing 10Gi PVC `runtime-smoke-data` was retained with the same identity and
contents. No old home/config was copied to the new installation.

A new CR with the same name provisioned a different 10Gi PVC,
`hermes-smoke-hermes-data`, with `Retain`. The new native gateway became Ready.
A read-only baseline confirmed zero messages/sessions, model `qwen38-27b` and
reasoning `xhigh`. The original namespace-local source Secret was reused without
logging its values. The previous poller was fully stopped before the new one.

## Real Telegram onboarding

The authenticated desktop client sent harmless nonce prompts to the exact bot.
Native read-only SQLite checks correlated the allowed Telegram user/chat,
`source=telegram`, user and assistant nonce messages and new token accounting for
`qwen38-27b`. The initial baseline contained no native history for that identity.
The completed reply was also visible in Telegram.

The agent saved its requested name and Russian concise style in `SOUL.md`, and
the user's preferred test name in persistent `memories/USER.md`. Both files
contained the onboarding nonce; file reads separately confirmed the chosen
names. In a second turn, the agent created
`workspace/operator-smoke/telegram-created.txt` and a personal `smoke-acceptance`
skill with `SKILL.md`. Both contained a second nonce and the bot confirmed the
completed writes. This is agent-created personalization, not an injected fixture.

The native “No home channel” notice was expected for a fresh home. `/sethome`
was sent in this same authorized DM and the bot confirmed its selection. No
scheduled task or cross-platform message was sent as part of this acceptance.

## Container restart and configuration authority

After both turns completed, `/reasoning low --global` was sent through Telegram.
The native loader confirmed effective `low` while the CR still declared `xhigh`.
A controlled SIGTERM restarted PID 1 in the same Pod; restart count changed 0→1.
The native loader then confirmed the CR model/provider/credential and `xhigh`.

All 340 checked personal file hashes, the same native session with 20 history
messages and the unmanaged personal configuration digest remained identical.
This includes the real Telegram-created SOUL, USER memory, skill and workspace
file. The existing live harness's generic stdout says no conversational model
change was seeded; this run separately seeded a **reasoning** change, verified
in `before-restart/live-state.json`. It did not change the model.

A subsequent normal Pod deletion created a new Pod UID on the same PVC. The
same 340 file hashes, 20 history messages/session and personal config digest
again matched. Native model/provider/credential and `xhigh` were correct.
A new Telegram prompt after both restarts received a visible reply recalling
both names, the previous workspace marker and the existing skill. Native
user/chat/nonce and new `qwen38-27b` accounting corroborated that reply.
The configured home channel also still pointed to the same authorized DM.

## Two installations with one token

A second CR `hermes-smoke-b` in the temporary namespace
`hermes-operator-test-pair-20260921` used a separate 1Gi PVC and `medium` reasoning.
The two credentials were copied in memory into a namespace-local test Secret;
they were never printed or committed. This test-only reuse is not the production
independent-credential contract.

A was suspended and its Pod fully disappeared before B started. B became natively
Ready, its effective config was checked by the native loader, and its native
history initially contained zero messages. The CR/PVC UIDs differed and A's
workspace marker was absent in B. B wrote its own marker, then was suspended and
fully stopped before A resumed. A became Ready with its original marker intact
and no B marker. At most one Telegram poller ran at any time.

This proves sequential reconciliation, effective config and state separation.
It does not prove two simultaneously Ready Telegram gateways with independent
credentials. Unauthorized identity and group cases remain `not-run`; no silence
was counted as proof of denial. After verification, the temporary B CR,
its namespace/PVC and copied Secret were removed. Primary A stayed Ready; its
new PVC and the old retained `runtime-smoke-data` remained intact. Cleanup is
recorded in `cleanup.json`.

## Tool image checks

Inside fresh A, `/opt/hermes-tools/smoke.py` passed all eight tests in 21.418s:
Playwright/agent-browser, native Hermes browser_exec, charts/archives, DOCX/Pandoc,
PDF generation/extraction/merge/rasterization, PPTX→PDF, Russian/English OCR and
XLSX recalculation. The test used a separate temporary HOME and did not install
packages or modify the upstream code. This is in-container tool coverage; it is
not a claim that each format was separately requested through Telegram.

Seven offline acceptance-harness tests and the generated-runtime check also
passed. Documentation links are checked with `make verify-docs`.

Private evidence: `/tmp/hermes-fresh-smoke-20260921/`, including
`fresh-verification.json`, `telegram-before.json`, `telegram-onboarding.json`,
`telegram-files.json`, `personal-files.json`, `two-installations-a.json`,
`two-installations-b.json`, `secondary-state/live-snapshot.json` and
`container-restart/live-snapshot.json`, `pod-replacement.json`,
`telegram-recall.json` and `home-channel.json`. Do not publish credentials, raw
conversation/database exports, personal snapshots or desktop screenshots.
