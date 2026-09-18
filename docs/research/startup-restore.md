# Восстановление CR-конфигурации без потери персонализации

Требование: runtime config writable; перед следующим запуском применяются параметры CR/Secrets, а пользовательские данные остаются на PVC. Проверка выполнена по исходникам `v2026.9.14`, commit `345cd2b057a452236de401d3534b8502a7465e8d`. Runtime smoke ещё не выполнен.

## Каждый запуск контейнера

Startup merge должен стоять перед `exec` основного Hermes процесса. Одного обычного initContainer недостаточно: успешно завершённые init containers не выполняются заново при каждом рестарте app container в том же Pod. [Kubernetes init containers](https://kubernetes.io/docs/concepts/workloads/pods/init-containers/#understanding-init-containers).

Bootstrap использует отдельный эталонный bundle и меняет только объявленные оператором YAML/env-пути и связанные служебные overrides. SOUL, личные инструкции, предпочтения, skills, memory, workspace, session IDs и история остаются. Перезаписывать весь config.yaml типовым файлом или очищать state.db нельзя.

## Модель хранится не только в YAML

`/model` сохраняет `model_override` с полями `model`, `provider`, `base_url`. После restart gateway восстанавливает его и применяет с приоритетом выше config/env и channel overrides. `/model --once` в durable override не записывается. Поэтому успешная запись модели из CR в YAML сама по себе не доказывает восстановление модели в прежней беседе. [Сохранение](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/slash_commands_model.py#L238-L259), [rehydration](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/run_agent_cache.py#L141-L183).

Primary storage — `state.db`, таблица `gateway_routing`, JSON-колонка `entry_json`, поле `model_override`; ключ — `(scope, session_key)`, где scope соответствует resolved sessions directory. `sessions/sessions.json` — legacy mirror, одного изменения которого недостаточно. [Routing storage](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/hermes_state_gateway.py#L247-L277), [формат записи](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/session.py#L455-L467), [primary/mirror](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/session_persistence.py#L25-L31).

Кандидат точечного восстановления — upstream Python API `SessionStore.list_sessions()` и `set_model_override(entry.session_key, None)`. Setter меняет поле override, сохраняя session ID и остальные данные. Это внутренняя API библиотеки, не обещанная стабильная CLI-команда; её нужно проверять для поддерживаемых версий. `reset_session()` не подходит, поскольку создаёт новый session ID. [Методы](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/session.py#L1064-L1099), [перечисление](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/session.py#L1156-L1165).

Persistence может использовать JSON fallback при ошибке SQLite. Bootstrap должен подтвердить очистку primary routing index и не запускать gateway со старым override при ошибке. Исторические поля модели в transcript/session metadata не требуется очищать: это история, а не активный routing override. [Fallback](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/session_persistence.py#L436-L466).

## Reasoning

Session-only reasoning override хранится в `ConversationState` в памяти и исчезает с процессом. `--global` пишет `agent.reasoning_effort` в YAML; этот ключ восстанавливается из CR. Mapping `agent.reasoning_overrides` может иметь более высокий приоритет и также должен учитываться при восстановлении, чтобы пользовательское переопределение не отменяло заданный CR режим. [Session state](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/session_state.py#L37-L53), [запись](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/slash_commands_model.py#L582-L614), [приоритеты](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/run_config_loaders.py#L145-L195).

## Первичная настройка и сохранность

Upstream gateway имеет first-contact onboarding: при отсутствии прежних сессий он может предложить знакомство, если `onboarding.profile_build` не выключен. Сведения пользователя записываются memory tool с `target="user"`; флаг предложения находится в пользовательском YAML. Это разговорная инструкция модели, а не гарантированно детерминированный wizard. Startup merge должен сохранить onboarding flags и профиль. [Первый контакт](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/run_turn.py#L1259-L1287), [onboarding](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/agent/onboarding.py#L122-L170).

Обязательный smoke: выполнить персонализацию, изменить модель/provider/reasoning в живой беседе, перезапустить контейнер без замены Pod и затем заменить Pod. После обоих сценариев проверить модель из CR в прежней беседе, неизменный session ID/history, сохранение SOUL, skills, memory, личных настроек и файлов workspace. Повторить после изменения CR/Secret.

## Health probes: дополнительная проверка исходников

В release есть `gateway_state.json` с process identity и состояниями gateway/platform/session store, а также отдельный `state/gateway.heartbeat`. Probe может пользоваться ими без web UI. У runtime status есть PID/start-time helpers; возраст status JSON нельзя делать единственным условием liveness, поскольку idle gateway может не обновлять этот snapshot. Heartbeat пишет PID, timestamp и process start identity. Это установлено чтением исходников, не проверкой контейнера: [runtime status](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/status.py#L805-L909), [heartbeat](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/shutdown_watchdog.py#L165-L208).

Проектные probe semantics и интервалы описаны в [спецификации](../design/v1alpha1.md); тесты current/stale process identity и отсутствия restart loop при внешнем outage входят в [план](../superpowers/plans/2026-09-18-hermes-operator.md).
