# Compatibility report

## Packaged contract and build targets

| Component | Contract |
| --- | --- |
| Kubernetes API | Chart minimum 1.34; generated CRD and envtest target 1.34.1 |
| Hermes Agent | `v2026.9.14`, official image digest pinned in the runtime catalog |
| Operator image | Release workflow builds linux/amd64 and linux/arm64 |
| Provider surface | `custom` + `chat_completions`, with `APIKey` or `None` |
| Network | Native IPv4 and dual-stack configuration; NAT64-only is outside v1 |

These are package inputs and build targets. A multi-architecture manifest does not prove runtime behavior on either architecture.

## Verification boundary

The repository contains focused runtime-adapter, API/envtest and network-policy evidence. The full Task 8 cluster scenarios, including the required release platform matrix, remain a release gate. Until that gate is recorded in release notes, this document does not claim release readiness or full Kubernetes/runtime compatibility.
