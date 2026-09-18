# Базовые поля CR и конфигурация Hermes

Проверка исходников: `v2026.9.14`, commit `345cd2b057a452236de401d3534b8502a7465e8d`. Текущие имена полей зафиксированы в [спецификации API](../design/v1alpha1.md); upstream paths проверены по исходникам, runtime-тесты ещё не проводились. По последнему решению эти пути используются для восстановления параметров перед запуском, а не для read-only блокировки работающего агента.

## Основные поля

| Предлагаемое поле spec | Куда передаётся в Hermes | Примечание |
| --- | --- | --- |
| `model.name` | `model.default` в config.yaml | Имя модели. |
| `model.provider` | `model.provider` | Для собственного OpenAI-compatible inference используется `custom`. |
| `model.baseURL` | `model.base_url` | URL задаётся через YAML. |
| `model.apiMode` | `model.api_mode` | Например, `chat_completions`; проверяется совместимость provider. |
| `model.apiKeySecretRef` | Provider-specific env; для custom — `HERMES_MODEL_API_KEY` и `model.api_key: "${HERMES_MODEL_API_KEY}"` | Единого штатного env для всех built-in providers нет. |
| `reasoning.effort` | `agent.reasoning_effort` | Provider может ограничивать допустимые значения. |
| `reasoning.overrides` | `agent.reasoning_overrides` | Per-model правила; default пустой, локальные overrides восстанавливаются при старте. |
| `model.contextLength` | `model.context_length` | Не является универсальным лимитом output tokens. |
| `agent.maxTurns` | `agent.max_turns` | Лимит шагов выполнения. |
| `agent.runBudgetSeconds` | `agent.run_budget_seconds` | Бюджет времени выполнения. |
| `tools.enabled` | `platform_toolsets.telegram` | Набор toolsets для Telegram. |
| `tools.disabled` | `agent.disabled_toolsets` | Явно отключённые toolsets. |
| `terminal.timeoutSeconds` | `terminal.timeout` | `terminal.backend: local` и рабочий путь устанавливает оператор. |
| `memory.charLimit`, `memory.userCharLimit` | `memory.memory_char_limit`, `memory.user_char_limit` | Ограничения объёма памяти. |
| `telegram.botTokenSecretRef` | `TELEGRAM_BOT_TOKEN` | Только из Secret. |
| `telegram.allowedUserIDs` | `TELEGRAM_ALLOWED_USERS` или `gateway.platforms.telegram.extra.allow_from` | Это ID отправителей, не group chat IDs. |

Источники: [model schema](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/website/docs/user-guide/configuring-models.md#L129-L138), [custom credentials](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/hermes_cli/runtime_provider_backends.py#L118-L174), [agent budgets](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/hermes_cli/config_defaults.py#L49-L62), [terminal](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/hermes_cli/config_defaults.py#L259-L294), [memory](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/hermes_cli/config_defaults.py#L1215-L1228), [Telegram toolsets](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/run_turn.py#L2161-L2165).

Универсальное поле `agent.maxTokens` пока не предлагается: общего подтверждённого mapping для gateway не установлено. Provider-specific request limits и параметры compression остаются кандидатами для `extraConfig`.

## System prompt и изменяемый контекст

По уточнению пользователя SOUL и персональная настройка принадлежат пользователю. Предлагавшиеся `agent.systemPrompt` и `agent.soul` исключены из списка принудительно управляемых полей CR. `agent.system_prompt`, `display.personality`, пользовательские определения personalities и содержимое `SOUL.md` должны оставаться доступными для персонализации и сохраняться при startup merge.

Факт upstream: `display.personality` имеет приоритет над `agent.system_prompt`. Это поведение нужно объяснить в руководстве по персонализации, а не решать запретом пользовательского выбора. [Resolver](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/hermes_cli/personality.py#L118-L124).

`SOUL.md` читается независимо как базовая identity. Он, workspace `AGENTS.md` и `.hermes.md` являются изменяемым пользовательским контекстом. Пользовательские переключатели `memory.memory_enabled` и `memory.user_profile_enabled` также не требуется автоматически блокировать административным слоем; отдельно задаются сервисные лимиты. [SOUL loader](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/agent/prompt_builder.py#L1452-L1483).

## Telegram DM-only и группы

Generic `group_policy: disabled` не является подтверждённым запретом групп Telegram в этом релизе. Действующий фильтр — непустой `gateway.platforms.telegram.extra.allowed_chats`; он отбрасывает неуказанные группы, но не личные сообщения. Пустой список снимает этот фильтр. `guest_mode` должен быть выключен, чтобы упоминание бота не обходило ограничение. [Group gate](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/plugins/platforms/telegram/adapter.py#L5815-L5843).

Для opt-in групп достаточно реальных разрешённых group chat IDs и отдельно sender allowlist. `group_allowed_chats` нельзя автоматически использовать вместо ограничения мест ответа: он может авторизовать всех участников группы. [Allowlist semantics](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/website/docs/user-guide/messaging/telegram.md#L1038-L1075).

Для DM-only исследуется непустой внутренний нечисловой маркер в `allowed_chats`, который не может совпасть с Telegram chat ID. Это вывод из кода, а не документированный boolean API upstream. До включения в контракт обязательны проверки text, command, media, callback и неавторизованных отправителей. Если их нельзя пройти без изменения upstream, требуется отдельное решение пользователя.

## Остальное состояние и bootstrap

- `gateway.json` используется legacy loader как fallback; `profiles/*` содержат другие config/env. Для одной управляемой инсталляции предлагается явно отключить multiplex profiles и проверить все fallback-пути. [Loader priority](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/config.py#L766-L790).
- При прямом non-root запуске без Docker bootstrap начальную синхронизацию bundled skills нужно выполнить штатным upstream helper, сохранив его правила `.bundled_manifest`. [Skills sync](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/tools/skills_sync.py#L363-L377).
- Cron jobs остаются изменяемым состоянием. Дополнительные настройки cron задаются через CR/extraConfig; `cron.allow_agent_scheduling` нельзя трактовать как глобальное выключение cron, поскольку он относится к agents, запущенным cron. [Cron defaults](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/hermes_cli/config_defaults.py#L1643-L1719).

## Runtime corrections 2026-09-18

[Official-image verification](runtime-verification.md) confirms `CUSTOM_BASE_URL` precedes YAML endpoint selection. The bundle must own/reset it, and own `GATEWAY_MULTIPLEX_PROFILES=false`; saved profile directories themselves remain untouched. Selected custom credential pools and matching legacy `custom_providers` credentials can replace the YAML model key. Bootstrap resets only the selected endpoint/provider credentials, retains unrelated entries, and verifies behavior through the native resolver. `agent.reasoning_overrides` should be explicitly owned as an empty map when not specified.

The synthetic real-handler matrix confirms text/command/media sender and chat filtering. Pending inline picker callbacks from an authorized sender bypass `allowed_chats` in this release; unknown senders remain denied. The user accepted that v1 limitation. The internal DM-only marker is consequently a verified text/command/media gate, not an unconditional callback gate. Reasoning defaults to `xhigh` by user decision; an explicit CR value overrides it.

## Implemented renderer matrix (Task 3, 2026-09-18)

The renderer currently supports **provider `custom`, API mode `chat_completions`**, with `APIKey` or `None`. Omitted API mode is explicitly restored as `chat_completions`. `openrouter`, `anthropic`, other providers, and other modes return validation errors until their mapping is verified. In the official native resolver, `custom` + `responses` on an arbitrary endpoint canonicalizes to `codex_responses` and is then discarded unless the endpoint has a recognized Responses host. The renderer rejects that combination instead of accepting an ignored setting. `None` reads no model Secret; `no-key-required` is the upstream SDK placeholder, not a generated credential.

`internal/config/validate.go` contains the pinned path/env exclusions and platform inventory. All rendered base leaves are owned even when optional settings are absent: absent context/memory/toolset limits are null removals, disabled toolsets are an empty list, and reasoning overrides are an exact empty map. Empty objects in extraConfig acquire no ownership. Unknown safe extra leaves are retained; known credential fields accept only references to declared `credentials.env` names. Errors report field paths and dependency identity without Secret values.

Native integration exposed additional precedence sources: root `telegram` aliases can override `gateway.platforms.telegram`; legacy `extra` dictionaries merge rather than replace; `TELEGRAM_GUEST_MODE`/`TELEGRAM_ALLOWED_CHATS` env can supersede YAML; `gateway.proxy_url`/`GATEWAY_PROXY_URL` routes turns through another inference service. Renderer owns the relevant aliases/env and disables the pinned release's other platform entrypoints. The custom resolver also prioritizes `OPENROUTER_API_KEY` on an OpenRouter-hosted custom URL, so this alias carries the declared model credential rather than a saved unrelated key.

First adoption clears only the version-specific administrative `agent.reasoning_overrides` map before generic leaf merge. This permits an empty CR map to replace a locally selected model override and prevents stale members of a nonempty map from surviving. Generic extra-path/personal-descendant ownership collision checks remain strict. Per-channel model/provider aliases are normalized while preserving channel system prompts.

Persisted Telegram pairing approvals are a separate authorization union, unaffected by `dm_policy=allowlist`. Startup filters Telegram approvals to the CR sender list and clears pending Telegram codes in **both** `pairing/` and `platforms/pairing/`, because the native PairingStore merges both layouts. Other platform grants, rate limits, allowed-user metadata and personal state remain. The adapter uses the pinned JSON schema with strict parsing and atomic writes; the native revoke API would mutate ambient dotenv and misses alternate-layout resurrection. No upstream code is changed.

Run the offline Go-renderer → bootstrap → native provider/gateway/authorization test with `HERMES_RUNTIME_TEST=1 go test ./internal/config -run TestOfficialRuntime -v` using the pinned Docker image. The test has `--network=none`, synthetic credentials, and never starts a polling process. It covers default/explicit/no-auth, legacy config/env and custom-provider pools, actual previously-paired unknown-sender rejection, allowed-sender acceptance, and extra-key retirement while preserving personal sibling keys. This proves local arm64 adapter compatibility, not real inference, Telegram delivery or Kubernetes rollout behavior.
