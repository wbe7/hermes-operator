# Agent image validation — 2026-09-21

Image: `wbe7/hermes:v2026.9.14`, Linux amd64 and arm64 manifest list
`sha256:feecb5d71f4876758b61e83cf1527fa6cdbb4f7e8919808c5eed7daf5085640f`.
Base: unchanged official image pinned in [Dockerfile](../../images/hermes/Dockerfile).
Tool inventory and operational commands: [agent image guide](../guides/agent-image.md).

## Local runtime evidence

Docker Desktop on Apple Silicon: arm64 native, amd64 through emulation.
Both platform variants passed, as UID/GID 10000, with a read-only root filesystem,
all capabilities dropped, no-new-privileges, writable temporary storage and no
external network:

- Eight end-to-end tool tests: Playwright navigation/screenshot/PDF; agent-browser
  navigation; unchanged Hermes `browser_exec` with CDP and screenshot; DOCX templates
  and Pandoc; XLSX formula recalculation; PPTX to PDF; PDF generation/extraction/
  merge/raster; RU/EN scanned PDF OCR; pandas chart and archive operations.
- 23 startup adapter tests against the derived image.
- Go-rendered config/native resolver scenarios, including credential retirement,
  Telegram pairing and preserved personalization.
- Standalone image entrypoint `hermes --help` (arm64).

The original image failed the added Python Playwright smoke with
`ModuleNotFoundError`. The live original-image agent's recorded `browser_exec`
result was `ChromeNotFound`. The derived image tests invoke the actual upstream
tool without installing packages at runtime or patching its source.

Plain headless XLSX conversion retained XlsxWriter's initial cached zero; the
`hermes-recalculate` helper explicitly calls LibreOffice
[`calculateAll()`](https://api.libreoffice.org/docs/idl/ref/interfacecom_1_1sun_1_1star_1_1sheet_1_1XCalculatable.html)
and the test reads the saved cached value `42` through openpyxl, then exports it
to PDF. This is functional evidence, not a claim of complete Excel compatibility.

Chromium uses explicit `--no-sandbox,--disable-dev-shm-usage`. Kubernetes security
controls remain required; browser processes share the agent's container trust
boundary. See [security](../guides/security.md).

The `Hermes image` workflow repeats both platform tests on separate native GitHub
runners before an explicitly requested publication. A configured workflow is not
itself evidence of a completed CI run.

## Publication and Berger Apps

Docker Hub accepted the tested manifest list with the digest above. Anonymous
registry inspection confirmed both architectures; compressed layer totals were
approximately 1.55 GiB for amd64 and 1.53 GiB for arm64.

The dev controller was updated from source archive SHA256
`8f0256eefb51a4d6aef431111954d2db1b97e1ac4881f9e8fedf7ae42fa68afc`.
This remains the existing dev source-build deployment, not a published operator
release or Helm-upgrade acceptance. Unit, API/envtest, chart, generated/docs checks
and `go vet ./...` passed after registering the image digest.

`hermes-operator-test/hermes-smoke` switched from the official repository to
`docker.io/wbe7/hermes`, generation 7, applied revision `00cb645b92664783`.
The new Pod `4fa06db5-a3d8-4411-8e7b-2c9e3b94db1d` was Ready with zero restarts and
the published digest. UID/GID 10000, RuntimeDefault seccomp, drop ALL, read-only
root filesystem and NetworkPolicy remained in effect.

- All eight tool tests passed inside the actual amd64 Kubernetes Pod, using a
  separate temporary home. The existing 2 GiB memory limit was sufficient for
  these fixtures; this is not a bound for arbitrary documents or browser workloads.
- A separate upstream `browser_exec` call loaded the installation's real settings,
  opened `https://example.com`, returned `Example Domain` and produced a screenshot.
  The gateway health PID remained 1. No Telegram message was sent by the test.
- The PVC UID remained `a0a97776-8dd2-436c-b8fc-6e4aade348b9`. All 793 prior
  personalized/workspace file hashes and the personal-config hash matched. All
  205 previous messages matched by content-prefix hash; 12 new messages appeared
  while the user continued working. The model remained `qwen38-27b`, provider
  `custom`, reasoning `xhigh` from the declaration.

Private before/after snapshots remain outside Git. This validates the image
upgrade and tool availability, not every conversational Telegram acceptance case.
