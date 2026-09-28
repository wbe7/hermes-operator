# Предпосылки веб-доступа Hermes

Проверено 2026-09-28. Это исследование для [интервью](../design/web-access.md),
а не результат приёмки новой функции. Проверки кластера выполнены read-only;
Secret values, пользовательские файлы и переписка не читались.

## Текущий оператор

Локальный `main` находится на `11ea734`. В API нет поля Web, Telegram обязателен.
В `internal/config/validate.go` web-конфигурация исключена из `extraConfig`.
В `internal/network/policy.go` ingress агента полностью закрыт. Runtime запускает
только `hermes gateway run --no-supervise`. Source Secret читает оператор,
а Pod получает его immutable revision snapshot; автогенерации credentials нет.

## Общие ограничения маршрутизации и браузера

HTTPRoute поддерживает изменение upstream path через `URLRewrite` /
`ReplacePrefixMatch`; это расширенная возможность Gateway API, поддержку которой
нужно проверять у конкретного контроллера. Переписывание request path само по себе
не исправляет URL внутри frontend, redirects и WebSocket-клиента.
[Gateway API: redirects and rewrites](https://gateway-api.sigs.k8s.io/guides/user-guides/http-redirect-rewrite/).

Разные пути одного HTTPS host принадлежат одному browser origin. Разделение
по path не предоставляет изоляцию origins; в частности, `localStorage` общий
для этих путей. Разделение cookies по Path не превращает пути в независимые origins.
Это важно для инсталляций разных пользователей и требует отдельного решения.
[MDN: same-origin policy](https://developer.mozilla.org/en-US/docs/Web/Security/Defenses/Same-origin_policy),
[MDN: Web Storage](https://developer.mozilla.org/en-US/docs/Web/API/Web_Storage_API).

Актуальная документация Hermes описывает вход по логину/паролю и отдельный секрет
подписи сессий. Basic provider рекомендован upstream для доверенной сети/VPN;
для публичного интернета документация рекомендует OAuth/OIDC. Изменяемая документация
не заменяет проверку исходников и runtime закреплённой версии.
[Hermes Web Dashboard](https://hermes-agent.nousresearch.com/docs/user-guide/features/web-dashboard/).

## Закреплённый upstream v2026.9.14

Проверены исходники revision `345cd2b057a452236de401d3534b8502a7465e8d`.
Следующие утверждения относятся к исходникам, не к запущенному dashboard в кластере.

- Dashboard содержит настройки, credentials, sessions и чат через TUI/PTY.
  CLI поддерживает `dashboard --host 0.0.0.0 --port 9119 --no-open --skip-build`.
  Официальный Dockerfile собирает frontend и TUI заранее.
  [CLI](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/hermes_cli/subcommands/dashboard.py#L15-L39),
  [Dockerfile](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/Dockerfile#L270-L281).
- Штатный password provider использует `HERMES_DASHBOARD_BASIC_AUTH_USERNAME`,
  `HERMES_DASHBOARD_BASIC_AUTH_PASSWORD` и `HERMES_DASHBOARD_BASIC_AUTH_SECRET`.
  Последний — отдельный ключ подписи сессий. Без него новый процесс генерирует
  новый ключ, и старые сессии перестают работать. Смена только пароля не отзывает
  ранее выданные сессии; для их отзыва требуется смена ключа подписи.
  [Basic provider](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/plugins/dashboard_auth/basic/__init__.py#L115-L259).
- `HERMES_DASHBOARD_SESSION_TOKEN` относится к local/desktop режиму и не заменяет
  публичный вход. При включённой авторизации frontend использует cookie session,
  а WebSocket — одноразовый ticket. Отсутствие provider при non-loopback bind
  приводит к отказу запуска; `--insecure` не является обходом авторизации.
  [Auth gate](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/hermes_cli/web_server.py#L419-L480),
  [WebSocket](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/hermes_cli/web_server_chat.py#L220-L293).
- Для path prefix upstream предусматривает переписывание assets, frontend base path,
  API/WebSocket URLs и cookie Path. Proxy должен снимать prefix перед backend и
  устанавливать доверенный `X-Forwarded-Prefix`, не пропуская произвольное значение
  клиента. `HERMES_DASHBOARD_PUBLIC_URL` должен содержать полный внешний URL, но сам
  не заменяет этот header.
  [SPA assets](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/hermes_cli/web_server_dashboard.py#L140-L208),
  [API base path](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/web/src/lib/api.ts#L7-L24),
  [cookies](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/hermes_cli/dashboard_auth/cookies.py#L47-L64).
- Корректные Secure cookies зависят от распознанного HTTPS. Для HTTPS termination
  перед Hermes нужны правильный forwarded proto и ограниченный список
  `dashboard.trusted_proxies`. Upstream отвергает безусловное доверие `*` и `/0`.
  [Proxy trust](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/hermes_cli/web_server_lifecycle.py#L432-L477).
- Встроенный auth использует HttpOnly cookies, а не общий localStorage auth token.
  При этом same-origin ограничение разных путей сохраняется: код одной страницы
  способен обращаться к другой инсталляции с cookies, которые браузер имеет для
  той инсталляции. Это следствие браузерной модели, а не утверждение о найденной CVE.
- Upstream запускает Telegram gateway и dashboard отдельными процессами на общем
  home. Наш launcher обходит s6: одно включение `HERMES_DASHBOARD=1` не запускает
  второй процесс. Нужны управление его жизненным циклом и проверка совместной работы.
  [s6 dashboard](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/docker/s6-rc.d/dashboard/run#L28-L59).

Предупреждение про trusted network/VPN присутствует и в pinned документации,
а не только в текущей версии сайта:
[pinned auth docs](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/website/docs/user-guide/features/web-dashboard.md#L720-L727).
По просмотренным исходникам обновление Hermes для базового Web/path механизма
не является обязательным; совместимость нашего image digest ещё не проверена.

## Режим без каналов

Pinned Hermes явно поддерживает gateway с нулём messaging platforms: процесс
продолжает работать для cron, запускает heartbeat и housekeeping, публикует
`gateway_state="running"` и ожидает завершения. Отсутствие каналов не является suspend.
[Zero platforms](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/run_startup.py#L1203-L1206),
[runtime services](https://github.com/NousResearch/hermes-agent/blob/345cd2b057a452236de401d3534b8502a7465e8d/gateway/run.py#L5339-L5347).

Это позволяет сохранить штатный gateway во всех четырёх комбинациях каналов:
без каналов; Telegram; Web; Telegram + Web. Dashboard запускается дополнительно
только при включённом Web. Новая модель idle-worker для этого не нужна.

Текущий operator probe безусловно требует подключённый Telegram; readiness нужно
связать с объявленными в CR каналами. Без каналов проверяется здоровье gateway,
с Web — также dashboard, а Telegram connectivity требуется только при настроенном
Telegram. Это кандидат реализации, подтверждённый исходниками, но ещё не runtime.
Пользовательские cron-задачи без каналов могут продолжить работу; для остановки
всей инсталляции сохраняется `spec.suspend`.

## Berger Apps: Gateway и TLS

Gateway API CRDs: `v1.5.1`, standard channel; HTTPRoute `v1` served/storage.
GatewayClass `istio`, controller `istio.io/gateway-controller`; proxy image
`registry.istio.io/release/proxyv2:1.30.3`.

| Назначение | Gateway namespace/name | Listener | Адрес | Разрешение attachment |
| --- | --- | --- | --- | --- |
| Внутренний | `infra-gateway/public` | `https` | `192.168.0.220` | Все namespaces |
| Внешний | `infra-gateway/external` | `https` | `192.168.0.221` | Namespace label `gateway.berger.dev/external: "true"` |

Оба Gateway имеют `Accepted=True`, `Programmed=True`, hostname listener
`*.s2technologies.ru`. Проверка сертификата, реально отдаваемого внутренним
Gateway, показала SANs `*.s2technologies.ru` и `*.ext.s2technologies.ru`;
окончание действия `2026-10-28T13:16:21Z`.

- TLS hostname verification для `agents.s2technologies.ru` проходит.
- Для `hermes1.agents.s2technologies.ru` получен hostname mismatch: одноуровневый
  wildcard сертификат не покрывает этот адрес. Нужен сертификат для вложенной зоны.
- DNS с рабочей машины для обоих имён и `hermes-smoke.agents.s2technologies.ru`
  возвращает `192.168.0.220`; AAAA отсутствует. Это не проверка публичной достижимости.
- HTTPRoute для этих адресов пока нет.

Namespace `hermes-operator-test` пока не разрешает external Gateway своим label.
Его PSA — `restricted/v1.34`, Istio injection выключен. Оператор не должен молча
менять чужой Gateway, сертификат или namespace permissions ради attachment.
Точная граница автоматизации будет согласована в интервью.

После выбора пользователем внешнего Gateway дополнительно проверены следующие
кандидаты. **Впоследствии пользователь отклонил зону `ext` для тестов**; результаты
ниже сохраняются как история исследования, а не как выбранные адреса:

| Режим | Кандидат URL | Base domain |
| --- | --- | --- |
| Subdomain | `https://hermes-smoke.ext.s2technologies.ru/` | `ext.s2technologies.ru` |
| Path | `https://agents.ext.s2technologies.ru/hermes-smoke` | `agents.ext.s2technologies.ru` |

Оба имени разрешаются в `94.228.243.175`, AAAA нет. TLS hostname verification
к external Gateway `192.168.0.221:443` проходит для обоих; подходящий listener:
`infra-gateway/external`, `sectionName: https-wildcard-ext-s2technologies-ru`.
HEAD по обычному DNS с рабочей машины получил HTTP 404 при успешной TLS verification,
remote IP `94.228.243.175`. Маршрутов для этих имён нет; коллизии с просмотренными
HTTPRoutes не найдены. Это подтверждает путь с рабочей машины через публичный адрес,
но не независимую проверку из другой сети (возможен hairpin NAT).

Для этих отклонённых кандидатов DNS/TLS уже были готовы. Для attachment понадобится
label `gateway.berger.dev/external: "true"` у namespace тестовой инсталляции;
в ходе исследования он не добавлялся.

Итоговый выбор пользователя — `agents.s2technologies.ru` без `ext`:
Path `https://agents.s2technologies.ru/hermes-smoke/` и Subdomain
`https://hermes-smoke.agents.s2technologies.ru/`. Пользователь планирует сменить DNS
на внешний Gateway. Path покрывается сертификатом `*.s2technologies.ru`;
Subdomain требует `*.agents.s2technologies.ru` и соответствующей настройки HTTPS
listener внешнего Gateway. Готовность этих изменений ещё не проверена; выводы
о готовности отклонённых имён из зоны `ext` на выбранные адреса не переносятся.

## Berger Apps: путь трафика

Gateway Pods находятся в `infra-gateway`, без hostNetwork. Их labels:
`gateway.networking.k8s.io/gateway-name: public` или `external`.
В этом namespace NetworkPolicy нет. У smoke ingress сейчас закрыт.

Кандидат для разрешения входа: сочетание namespaceSelector
`kubernetes.io/metadata.name: infra-gateway` и podSelector выбранного Gateway,
только на web TCP port. Это особенность проверенного Istio deployment,
а не универсальное соглашение для любых Gateway implementations.
Прохождение трафика, WebSocket, auth и сохранение private-egress ограничений
не проверены новой политикой: её ещё нет.

## Сохранность тестовой инсталляции

`hermes-operator-test/hermes-smoke` Ready, Hermes `v2026.9.14`, `suspend: false`,
модель `qwen38-27b`, reasoning `xhigh`. Pod `hermes-smoke-hermes-0` Ready,
UID `5e107a37-fa28-439d-b20d-47095e66c9f5`, restart count `0`.

Используемый PVC `hermes-smoke-hermes-data` Bound, 10Gi, retention `Retain`,
UID `af337352-8dbe-4417-b015-a169b51cfc3d`. Старый PVC `runtime-smoke-data`
также сохранён; для работы над Web его удаление или изменение не требуется.

## Ещё не проверено

- Запуск dashboard из нашего образа под текущими securityContext и filesystem mounts.
- Полный вход, чат/PTY/WebSocket и файлы по домену и под префиксом пути.
- Совместное использование состояния с Telegram и поведение рестартов/probes.
- Ротация credentials и отзыв существующих сессий.
- Изоляция двух инсталляций при работе из одного браузера.
- Публичный интернет, NAT и DNS за пределами рабочей машины.
