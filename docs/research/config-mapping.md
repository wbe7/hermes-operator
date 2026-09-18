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
