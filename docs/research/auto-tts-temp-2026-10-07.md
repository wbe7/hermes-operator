# Automatic Telegram TTS temporary-path failure — 2026-10-07

## Symptom and cause

After operator 0.4.0 enabled `VoiceOnly`, the smoke gateway had been restarted
at 14:33:06 UTC and `gateway_voice_mode.json` contained the correct per-chat
mode. Manual speech generation and STT passed, but voice inputs received text.
A restart was therefore not evidence of automatic voice-reply correctness.

The actual pinned Hermes `BasePlatformAdapter._synthesize_auto_tts()` reproduced
the failure with the managed smoke configuration. Its generated output path was
`/tmp/hermes_voice/tts_reply_<id>.ogg`. `agent.file_safety` classified the path as
outside `HERMES_WRITE_SAFE_ROOT=/opt/data`; TTS returned `success: false` and the
automatic function returned no audio paths. It did not raise a visible synthesis
warning for this unsuccessful JSON result. The manual call omitted `output_path`
and used the normal audio cache, which explains the earlier false confidence.

A first diagnostic used a controlled `/tmp` path. The unmodified native path
builder reproduced the same failure, ruling out that diagnostic fixture as the
cause. Changing only TMPDIR to a directory under `/opt/data` made the original
automatic synthesis function return audio. No upstream code or file protection
was modified.

## Smoke mitigation

Only `hermes-operator-test/hermes-smoke` was changed: created
`/opt/data/.cache/hermes-tmp` and set `spec.extraEnv.TMPDIR` to that path.
The operator rolled the Pod and it returned to Ready. Re-running native automatic
synthesis with the actual managed environment passed, without a process-local
override. This temporary mitigation uses PVC storage and should be removed after
installing the permanent operator fix. It does not by itself prove the complete
incoming Telegram voice → LLM → outgoing voice conversation; that verification
was requested separately from the user.

## Permanent fix and regression coverage

The workload template sets `TMPDIR=/opt/data/.cache/tmp` and mounts the existing
bounded `tmp` emptyDir at that path as well as `/tmp`. The file-safety root remains
unchanged, no new privilege or host mount is introduced, and temporary files
remain ephemeral. The mount path is reserved for temporary data, not personal
files. Agent core and image remain unchanged.

- A workload regression first failed on the old template; it checks the safe
  TMPDIR and its writable, bounded ephemeral backing volume.
- The pinned original-image test now calls actual `_synthesize_auto_tts()` with
  safe-root enforcement and cleared session-platform context. It uses the real
  speech SDK against a local fixture server, requires audio output, and checks
  Ogg bytes and an Opus API request. Existing auth, modes, disabled-tool and
  personal-state cases remain covered.
- The live diagnostic used the actual S2 Pro endpoint. All diagnostic wrappers
  ran in separate processes; no instrumentation was installed into the running
  gateway. Private scripts/evidence are in `/tmp/hermes-voice-debug/`.

## Released correction — 2026-10-08

[Operator 0.4.1](release-0.4.1.md) deployed the permanent fix to Berger Apps.
The smoke-only `spec.extraEnv.TMPDIR` was removed, the obsolete empty diagnostic
directories were cleaned up, and all 16 active agents passed native safe-temp
checks. Automatic synthesis and STT passed before and after a controlled smoke
restart. PVC identities, personal files and existing conversation history were
preserved. See the release report for the separate Telegram delivery scope.
