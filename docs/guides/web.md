# Web dashboard через Gateway API

Web выключен по умолчанию. В одном `Hermes` можно объявить Telegram, Web, оба
канала или ни одного. Удаление `spec.telegram` выключает Telegram при следующем
старте; оставшийся ключ бота в Secret не включает его обратно. Без каналов
native gateway продолжает обслуживать cron и внутреннее состояние. `suspend:
true` останавливает инсталляцию целиком.

Функция требует новой версии оператора и CRD из этого изменения: опубликованный
релиз `0.1.0` Web ещё не поддерживает. Перед upgrade применяйте CRD и обновляйте
chart/operator вместе; см. [установку](install.md).

## Инфраструктура

Администратор заранее устанавливает Gateway API v1 CRDs и совместимый Gateway
controller. Без них инсталляции с выключенным Web продолжают работать.
Нужен готовый Gateway с именованным HTTPS listener на порту 443, terminating TLS,
правильным сертификатом и `allowedRoutes`, разрешающим namespace инсталляции.
Оператор не создаёт Gateway, DNS, сертификаты и namespace labels.

- Subdomain: `maria.agents.example.com`, DNS wildcard и сертификат
  `*.agents.example.com`. Сертификат `*.example.com` этот адрес не покрывает.
- Path: `agents.example.com/maria`, DNS и сертификат для `agents.example.com`.

Оба режима подходят для внутреннего или внешнего Gateway. CR указывает его
реальные ingress sources и IP/CIDR прокси; их нельзя копировать между кластерами
без проверки. Для Gateway в Pod network обычно используют одновременно
namespaceSelector и podSelector. Для другой сетевой схемы можно указать точный
`ipBlock.cidr`. `/0`, пустые selectors и unbounded ingress не принимаются.
`trustedProxyCIDRs` должны покрывать адрес прокси, который реально видит
Hermes. Это доверие к forwarded HTTPS metadata, а не разрешение private egress.

## CR

Полные примеры: [Subdomain](../../examples/hermes-web-subdomain.yaml) и
[Path](../../examples/hermes-web-path.yaml). Фрагмент для существующей инсталляции:

```yaml
spec:
  web:
    enabled: true
    routing:
      mode: Subdomain # или Path
      baseDomain: agents.example.com
      # name: maria   # по умолчанию metadata.name
    gatewayRef:
      name: external
      namespace: infra-gateway # по умолчанию namespace Hermes
      sectionName: https-agents
    auth:
      username: admin
    network:
      ingressFrom:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: infra-gateway
          podSelector:
            matchLabels:
              gateway.networking.k8s.io/gateway-name: external
      trustedProxyCIDRs: ["10.42.0.0/16"] # заменить на сеть/адреса прокси
```

Оператор создаёт `<имя>-hermes-web` Service и HTTPRoute, разрешает указанным
источникам TCP 9119 и запускает stock dashboard рядом с native gateway. Оба
процесса работают non-root в той же инсталляции и используют один PVC.
Обязательные процессы контролируются supervisor/probes. Path route удаляет
префикс перед backend и задаёт доверенные forwarded headers; вручную
добавлять отдельный rewrite или proxy не нужно.

`status.web.url` показывает рассчитанный адрес. `WebReady=True` подтверждает
текущие Accepted/ResolvedRefs условия выбранного HTTPRoute parent; общий
`Ready=True` также требует актуальный Ready Pod. Эти условия не проверяют
публичный DNS, доверие браузера к сертификату или доступность интернета.

При ошибочном Web spec, недоступном Gateway, выключении Web, suspend или
удалении CR оператор удаляет свою публикацию. Чужие Service/HTTPRoute не
перезаписываются и не удаляются. Коллизии адресов между Hermes CR отвергаются;
произвольные чужие HTTPRoute должны контролироваться администратором Gateway.

## Credentials и вход

Штатный вход Hermes использует **логин и пароль**, а не bearer token в URL.
Логин — `web.auth.username`, default `admin`. В основном Secret
`credentials.secretName` или `<имя>-hermes-secret` находятся:

| Ключ | Назначение |
| --- | --- |
| `WEB_PASSWORD` | Пароль входа. |
| `WEB_SESSION_SECRET` | Ключ подписи native сессий, минимум 16 байт. |

Оператор использует заданные непустые значения, а отсутствующие генерирует
один раз из 32 случайных байт. Остальные ключи, labels и ownership Secret
сохраняются. Пустое значение или пробелы по краям — ошибка, не команда регенерации. Upstream обрезает пробелы; оператор не меняет исходные значения молча. Immutable Secret
должен содержать оба ключа заранее. Если Secret управляет External Secrets или
другой контроллер, задайте оба ключа в его источнике, чтобы он не удалял их.

Получить пароль локально (не публикуйте вывод в issue/логах):

```bash
kubectl -n hermes-users get secret maria-hermes-secret \
  -o jsonpath='{.data.WEB_PASSWORD}' | base64 --decode
```

Исходный Secret и Web keys переживают рестарты, выключение Web и удаление CR.
Immutable Secret с суффиксом revision — служебный snapshot; не редактируйте его.
Меняйте credentials в основном Secret, оператор выполнит rollout.

**Для отзыва всех старых сессий меняйте одновременно `WEB_PASSWORD` и
`WEB_SESSION_SECRET`.** Одна смена пароля не отзывает ранее подписанные cookie.
Удаление отсутствующего ключа приводит к его генерации на следующем reconcile.
При disable/re-enable сохранённые ключи не меняются, старые cookie могут снова
работать до истечения срока. Для полного отзыва ротируйте signing key.

## Граница доверия

Это полный интерфейс управления своей инсталляцией: файлы, настройки, чат и
инструменты. SOUL, skills, память и история остаются на PVC. Поля, управляемые CR,
восстанавливаются при рестарте, как и при работе через Telegram.

Native password auth разрешена и на публичном Gateway по принятому решению
проекта; дополнительного OAuth/OIDC, WAF или rate limiter оператор не ставит.
Upstream рекомендует password auth для доверенной сети/VPN, а для публичного
доступа — OAuth/OIDC. Учитывайте эту границу при выборе эксплуатации.

**Path не изолирует агентов в браузере:** разные пути одного hostname имеют
общий origin. Используйте Subdomain для взаимно недоверенных клиентов; Path —
для одной доверенной группы. Ни отдельные cookie paths, ни NetworkPolicy не
превращают разные пути в отдельные browser security boundaries.

## Повторить smoke-проверки

На выделенной тестовой инсталляции задайте `E2E_WEB_KUBECONFIG`,
`E2E_WEB_CONTEXT`, `E2E_WEB_NAMESPACE`, `E2E_WEB_HERMES`,
`E2E_WEB_DEDICATED=yes`. Namespace должен начинаться с `hermes-operator-test`.

```bash
python3 hack/e2e-web.py
```

Проверяются настоящий HTTPS, закрытый API, неверный пароль, login, cookie
Secure/Path, frontend assets, WebSocket upgrade и защита от повторного ticket.
Credential читается напрямую из Secret и не выводится. Сам тест не меняет CR.

Дополнительные **изменяющие состояние** проверки запускаются по одной:

```bash
python3 hack/e2e-web-lifecycle.py --restart # пересоздать Pod, проверить прежнюю сессию
python3 hack/e2e-web-lifecycle.py --rotate  # заменить оба Web key, отозвать старые сессии
python3 hack/e2e-web-lifecycle.py --toggle  # Web-only → ни одного канала → Web-only
```

Последние две команды предназначены для временного агента. `--rotate` не
восстанавливает старый пароль: новый остаётся в основном Secret. `--toggle`
проверяет сохранение keys и отсутствие Telegram, не сбрасывает PVC.

Результаты проверки на реальном Gateway: [acceptance 2026-09-28](../research/web-acceptance-2026-09-28.md).
