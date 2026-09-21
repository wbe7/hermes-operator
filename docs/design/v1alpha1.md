# Hermes Operator: спецификация первой версии

Дата: 2026-09-18. Статус: CRD, контроллер, runtime и исполняемый acceptance harness реализованы и прошли review исходников; полная release qualification остаётся открытой. Текущие результаты и незакрытые gates — в [acceptance ledger](../research/v1-acceptance.md).

Продуктовые решения находятся в [интервью](interview.md), причины архитектурных границ — в [ADR](../adr/0001-declarative-configuration-authority.md). Этот документ фиксирует выбранные инженерные defaults и поведение API. Изменения, необходимые по результатам испытаний, отражаются здесь до публикации API. Работа по реализации разбита в [плане](../superpowers/plans/2026-09-18-hermes-operator.md).

## 1. Границы продукта

- Одна установка оператора обслуживает весь кластер. CR, Secret, PVC и workload инсталляции находятся в одном namespace.
- API: `hermes.wbe7.github.io/v1alpha1`, namespaced kind `Hermes`, plural `hermes`, singular `hermes`, short name `hms`. Status — отдельный subresource. Scale subresource отсутствует.
- Один CR представляет одну инсталляцию: один процесс Hermes, один persistent home и один Telegram bot token. Все разрешённые пользователи этой инсталляции разделяют её данные.
- Используется оригинальный официальный образ. Контроллер и стартовый адаптер принадлежат оператору; исходники и установленные пакеты Hermes не патчатся.
- Обязательный канал — Telegram long polling, личные сообщения. Отдельное включение групп предусмотрено ниже и выпускается только после проверки штатного фильтра.
- Не входят в v1: web UI/HTTPRoute, другие мессенджеры, frontend/backend, backups/snapshots/restore, автоматическая миграция данных между PVC, HA одного агента, Kata/gVisor.
- Пользователь вправе менять runtime config и персонализировать агента. CR/Secrets восстанавливают управляемые параметры при каждом старте контейнера. Ограничения Pod и сети действуют постоянно.

## 2. Версии и имена ресурсов

`spec.version` обязателен: нет неявного `latest` и автоматического обновления Hermes. В составе оператора находится каталог поддерживаемых release → image digest → startup adapter. Неизвестная версия даёт `UnsupportedVersion` до запуска workload. В каталог поддержки версия попадает только после runtime-проверок.

Поддерживаемый v1 runtime — `v2026.9.14`, source revision `345cd2b057a452236de401d3534b8502a7465e8d`, официальный multiarch digest `sha256:99641e57ec762c59e54cb44aa6746b7fc68c18b3c5ddb088af54234c613d9294`. Локальные adapter/runtime tests на pinned image выполнены; полная кластерная приёмка остаётся Task 8. См. [runtime verification](../research/runtime-verification.md) и [исследование upstream](../research/upstream-and-isolation.md).

| Поле | Тип / default | Контракт |
| --- | --- | --- |
| `version` | string, required | Точный release из каталога оператора. |
| `image.repository` | string = `docker.io/wbe7/hermes` | Расширенный runtime или его зеркало. Явный `docker.io/nousresearch/hermes-agent` выбирает исходный образ. Не содержит tag или digest. |
| `image.digest` | string, optional | Если задан, обязан совпадать с одним из проверенных digest выбранного release. При отсутствии берётся вариант из каталога по repository. |
| `image.pullPolicy` | `IfNotPresent` / `Always`, default `IfNotPresent` | Workload всегда использует digest. |
| `image.pullSecrets` | list of local names, default `[]` | Передаются как Kubernetes imagePullSecrets. |

Для CR `maria`: StatefulSet и headless Service — `maria-hermes`; Pod — `maria-hermes-0`; созданный PVC — `maria-hermes-data`; ServiceAccount и NetworkPolicy — `maria-hermes`. Имена CR ограничены DNS label длиной до 40 символов с начальной буквой a-z и без точек, чтобы сохранить запас для суффиксов и revision.

Внутренние input bundles получают имя `<name>-hermes-input-<revision>`. Они имеют ownerReference на CR. Чужие одноимённые объекты не усыновляются и не перезаписываются: `ResourceConflict`. Исключения — только явно указанные Secret и existing PVC, которые остаются внешними.

## 3. Credentials и модель

Все ссылки имеют форму `{name?: string, key: string}`, без namespace и без optional. Пропущенный `name` разрешается через `credentials.secretName`, затем через `<metadata.name>-hermes-secret`. Пустое значение обязательного ключа — ошибка зависимости.

| Поле | Тип / default | Поведение |
| --- | --- | --- |
| `credentials.secretName` | string, optional | Общий Secret инсталляции по соглашению выше. |
| `credentials.env` | map env-name → SecretKeyRef, default `{}` | Дополнительные credentials для upstream tools/providers. Доставляются только явно выбранные ключи. |
| `model.provider` | nonempty string, required | Явный provider; `auto` запрещён, чтобы сохранённое состояние не выбирало провайдера вместо CR. |
| `model.name` | nonempty string, required | Mapping: `model.default`. Наличие модели у провайдера проверяется запросом, а не выдуманным enum CRD. |
| `model.baseURL` | absolute HTTP(S) URL, optional | Обязателен для `custom`; без userinfo, query и fragment. Секрет не в URL. HTTP разрешён для локального inference. |
| `model.apiMode` | `chat_completions` / `responses` / `anthropic_messages`, optional | При отсутствии используется default адаптера выбранного provider/release. Неподдержанная комбинация даёт `InvalidConfiguration`. |
| `model.auth` | `APIKey` / `None`, default `APIKey` | `None` разрешён только для явно заданного custom endpoint. Не требует фиктивного Secret с ключом модели. |
| `model.apiKeySecretRef` | SecretKeyRef, default key `MODEL_API_KEY` | Нельзя задать при `auth: None`. Для custom mapping через `HERMES_MODEL_API_KEY`; для built-in — provider-specific env. |
| `model.contextLength` | positive integer, optional | Mapping: `model.context_length`; отсутствие восстанавливает upstream default. |
| `reasoning.effort` | nonempty string = `xhigh` | Mapping: `agent.reasoning_effort`. Default явно выбран пользователем. Отсутствие возвращает xhigh, а не оставляет локально выбранный effort. |
| `reasoning.overrides` | map model-name → effort, default `{}` | Mapping: `agent.reasoning_overrides`; явные per-model исключения. |

Первый обязательный provider path — `custom` с OpenAI-compatible inference. `openrouter` и `anthropic` включаются в матрицу после проверки их штатных resolver. Остальные providers не получают ложной гарантии: новый provider требует mapping credentials в адаптере и теста; до этого они отклоняются как `InvalidConfiguration`. OAuth-входы, interactive login и облачная workload identity не входят в первоначальную матрицу.

Значения reasoning зависят от provider/model. CRD принимает строку, адаптер проверяет известные ограничения версии. Ни успешная валидация CR, ни Ready не означают, что удалённый provider поддерживает запрошенный effort или имеет доступную квоту.

Secret-значения не попадают в ConfigMap, CR/status, Events и логи оператора. Источники не изменяются и не удаляются. Оператор создаёт собственный revision Secret с выбранными ключами для согласованного запуска; он удаляется вместе с CR. Credentials внутри собственного контейнера доступны пользователю, как согласовано.

## 4. Telegram

| Поле | Тип / default | Поведение |
| --- | --- | --- |
| `telegram.botTokenSecretRef` | SecretKeyRef, default key `TELEGRAM_BOT_TOKEN` | Токен выделенного бота. |
| `telegram.allowedUserIDs` | nonempty set of decimal strings, required | Положительные числовые ID отправителей, например `"123456789"`. Не username и не ID групп. |
| `telegram.groups.enabled` | boolean = `false` | При false группы отбрасываются после применения startup config. |
| `telegram.groups.allowedChatIDs` | set of negative decimal strings, default `[]` | При enabled=true обязателен непустой список; при false список должен быть пуст. |

В группе одновременно должны пройти проверку chat ID и sender ID. Включение группы не авторизует всех её участников. `guest_mode` выключен; пустые upstream lists не используются как эквивалент запрета. Способ DM-only для выбранного release должен пройти тесты text/command/media/callback; исследуемый непустой нечисловой marker в `allowed_chats` пока не доказан runtime-тестом.

**Принятое пользователем ограничение v1:** в официальном v2026.9.14 старый pending inline picker может обработать callback разрешённого пользователя в запрещённой группе и изменить модель/reasoning. Обычные text/command/media проходят group gate; неавторизованные отправители по-прежнему отклоняются. Пользователь явно согласовал этот узкий дефект вместо patch upstream. В тестах он фиксируется отдельно и не маскируется обещанием полной блокировки любых callbacks.

Адаптер задаёт Telegram как единственный активный внешний канал при старте, polling и выключенный profile multiplexing. Поведение фильтров и исходные источники: [mapping](../research/config-mapping.md).

Параметры форматирования, доставки и расширенных команд задаются через `extraConfig` в формате выбранной версии. Slash-команды персонализации и смены модели не блокируются оператором. Writable config означает, что намеренное последующее изменение пользователем правил Telegram не является заблокированным сценарием; startup снова применит CR.

Один token нельзя использовать в двух активных инсталляциях или с внешним poller. Автоматическая глобальная блокировка токенов не входит в v1; конфликт Telegram должен проявляться как неготовность и диагностироваться без печати токена.

## 5. Остальные основные настройки

Defaults здесь — выбранные defaults продукта. Они не являются результатами измерений и проверяются нагрузочным smoke до выпуска.

| Поле | Тип / default | Upstream mapping / правило |
| --- | --- | --- |
| `agent.maxTurns` | integer ≥ 0 = `50` | `agent.max_turns`; 0 явно означает unlimited и отображается адаптером в штатное unlimited. |
| `agent.runBudgetSeconds` | integer ≥ 0 = `600` | `agent.run_budget_seconds`; 0 — unlimited. |
| `terminal.timeoutSeconds` | positive integer = `300` | `terminal.timeout`; backend `local`, cwd `/opt/data/workspace`. |
| `tools.enabled` | nonempty set of toolset names, optional | `platform_toolsets.telegram`; отсутствие возвращает штатный набор выбранного release. |
| `tools.disabled` | set of toolset names = `[]` | `agent.disabled_toolsets`; пересечение enabled/disabled отклоняется. |
| `memory.charLimit` | positive integer, optional | `memory.memory_char_limit`; отсутствие — upstream default. |
| `memory.userCharLimit` | positive integer, optional | `memory.user_char_limit`; отсутствие — upstream default. |
| `resources.requests` | Kubernetes ResourceList | Defaults: cpu `100m`, memory `512Mi`, ephemeral-storage `256Mi`. |
| `resources.limits` | Kubernetes ResourceList | Defaults: cpu `2`, memory `2Gi`, ephemeral-storage `2Gi`. |
| `scheduling.nodeSelector` | map = `{}` | Стандартный Kubernetes selector. |
| `scheduling.tolerations` | list = `[]` | Стандартные Toleration; без назначения priorityClass. |
| `scheduling.affinity` | Kubernetes Affinity, optional | Размещение без доступа к произвольному PodSpec. |
| `suspend` | boolean = `false` | StatefulSet replicas=0; PVC и конфигурация сохраняются. |
| `extraConfig` | JSON object = `{}` | Дополнительные несекретные upstream YAML-параметры. |
| `extraEnv` | map env-name → string = `{}` | Дополнительные несекретные env-параметры upstream. |

Resources сливаются с defaults по отдельному ключу; удалить CPU/memory limits пустой картой нельзя. Все значения положительные, request ≤ limit. Пользовательские установки пакетов должны находиться в persistent home; запись в системные каталоги образа закрыта.

Нет CR-полей SOUL, personality, systemPrompt, onboarding profile или содержимого памяти. Первичная персонализация доступна пользователю через штатные возможности Hermes, без обязательного инфраструктурного CLI wizard.

## 6. Extra config и владение ключами

Контракт различает три набора путей:

1. **Базовые управляемые**: пути основных полей и связанные альтернативные источники модели, reasoning, credentials, Telegram authorization. Перед каждым стартом они нормализуются, даже если optional поле в CR отсутствует.
2. **Дополнительные управляемые**: точные leaf paths из extraConfig и имена extraEnv/credentials.env. Оператор хранит предыдущий набор принадлежащих ему путей. После удаления поля из CR прежнее значение удаляется из runtime config/env, возвращая upstream default.
3. **Пользовательские**: остальные ключи и файлы, включая SOUL, personality, system prompt, onboarding flags, memory/profile toggles. Они сохраняются.

Массив — одно значение, заменяется целиком. Объекты сливаются по листьям. `null` в extraConfig означает удалить этот путь при старте; пустой объект не захватывает весь родительский раздел. Замена scalar ↔ object с потерей пользовательских потомков отклоняется как `ConfigurationConflict`.

ExtraConfig не может пересекать основные управляемые пути, менять служебные home/workspace/backend, включать дополнительные каналы/профили/web UI, захватывать персонализацию или задавать credential literals в известных секретных полях. Проверяется пересечение как предка, так и потомка, а не только равенство строки пути. Неизвестные upstream поля сохраняются CRD, но это не доказывает их поддержку Hermes.

ExtraEnv/credentials.env не могут переопределять служебное окружение запуска, generated model/Telegram env или дублировать друг друга. Reserved: `HOME`, `HERMES_HOME`, `HERMES_*` служебного адаптера, `PATH`, `PYTHON*`, `LD_*`, пути XDG и env-пути основных настроек из mapping выбранного release. Каталог адаптера перечисляет точный reserved set; нельзя считать произвольный regex проверкой всех upstream конфигураций.

Дополнительные секреты передаются через credentials.env и штатные env references upstream. Оператор отклоняет известные credential literals, но не заявляет универсальное распознавание секрета в произвольной строке extraConfig. Правило администратора: никаких секретных значений в CR.

Размер входной конфигурации без Secret-значений — максимум 256 KiB; максимальная вложенность extraConfig — 16. allowedUserIDs и allowedChatIDs — максимум по 256 элементов; allowPrivate и additionalBlockedCIDRs — по 128; ports одного исключения — 32; credentials.env и extraEnv — по 64; каждый список toolsets — 128; image.pullSecrets — 16. Ошибки содержат имя поля или путь, но не переданный value.

## 7. Persistent storage

Нужно выбрать ровно один вариант:

| Поле | Тип / default | Правило |
| --- | --- | --- |
| `storage.create` | object | Создать отдельный PVC `<name>-hermes-data`. |
| `storage.create.size` | positive storage quantity = `10Gi` | Увеличение разрешено; уменьшение отклоняется. |
| `storage.create.storageClassName` | string, optional | Отсутствует — default StorageClass; `""` — явный отказ от default class по Kubernetes semantics. |
| `storage.create.accessMode` | `ReadWriteOnce` / `ReadWriteOncePod`, default `ReadWriteOnce` | RWOP требует поддерживающего CSI. |
| `storage.existingClaim` | string | Подключить существующий Bound или ожидающий binding PVC в том же namespace. |
| `storage.deletionPolicy` | `Retain` / `Delete`, default `Retain` | Delete разрешён только с create. |

Filesystem volumeMode обязателен. Storage source, class и access mode неизменяемы после создания CR. Изменение source — новая инсталляция; автоматического копирования данных нет. Оператор не меняет размер, labels или ownerReferences existing claim.

PVC монтируется целиком в `/opt/data`, одновременно `HOME` и `HERMES_HOME`. Workspace — `/opt/data/workspace`; config.yaml, .env, SOUL, skills, memories, cron, sessions, SQLite/WAL/SHM и домашние пользовательские каталоги находятся на этом PVC. Временный `/tmp` — bounded emptyDir 1Gi, не пользовательское хранилище.

Созданный PVC не получает GC ownerReference на CR/StatefulSet. Оператор записывает в него annotation с исходным CR UID и в status — claim name/UID. В Delete finalizer сначала останавливает workload и ждёт удаления его Pod, затем удаляет только PVC с совпадающими name, UID и отметкой создателя. При несовпадении — `StorageIdentityMismatch`, без удаления. Политика удаления после deletionTimestamp фиксируется и не пересчитывается по гоняющимся обновлениям spec.

Retain снимает finalizer после остановки workload, оставляя claim. Повторное создание CR с тем же именем не усыновляет прежний claim автоматически: нужен новый CR с явным existingClaim. Удаление namespace и ручное удаление PVC находятся вне этой гарантии. PV reclaim policy определяет судьбу диска после удаления claim.

Storage expansion требует поддержки StorageClass/CSI; оператор обновляет только request и показывает состояние resize. Pending PVC с WaitForFirstConsumer не должен блокировать создание Pod, необходимого для binding.

Existing PVC должен быть выделен этой инсталляции, writable UID/GID 10000 и поддерживать SQLite locking/fsync. Пустой claim и прежний Hermes home допустимы; чужой layout или ошибки чтения не исправляются очисткой. Одновременное подключение одного existingClaim двум управляемым CR в namespace отклоняется. За внешние workloads с доступом к этому claim оператор не отвечает.

## 8. Startup contract

Revision состоит из immutable ConfigMap с несекретной конфигурацией и immutable Secret с выбранными credentials. Новый Pod получает согласованную пару одной revision. Оператор не меняет input bundle работающего Pod; смена CR/Secret создаёт новую revision и rollout. Старые bundles сохраняются, пока на них ссылается Pod или StatefulSet.

Startup и probe scripts монтируются read-only из отдельного versioned ConfigMap. Python запускается из официального venv с isolated import path; writable workspace не должен подменять импортируемый startup helper. Образ Hermes не пересобирается.

Каждый старт основного контейнера выполняет:

1. Проверку revision, layout и доступа к home. Получение локальной startup-блокировки без force takeover.
2. Чтение YAML/.env и предыдущего manifest управляемых путей. На повреждённом файле — остановка с диагностикой, без нового пустого home.
3. Создание только отсутствующих каталогов/первичных файлов, синхронизацию bundled skills штатным helper с сохранением пользовательских изменений.
4. Запись pending manifest до изменения файлов; точечное применение управляемых путей, очистку удалённых управляемых extra paths и нормализацию альтернативных model/env/profile источников. При повторе учитываются committed, pending и текущие owned paths, чтобы прерванный apply с последующей сменой CR не оставил забытый managed key. Пользовательские ключи/файлы сохраняются.
5. Очистку активных сохранённых session model overrides в основном SQLite routing index и legacy mirror через upstream API; session ID, история, metadata прошлых ответов остаются. Reasoning overrides приводятся к CR.
6. Проверку результатов, atomic replace config/env и запись manifest последней успешной конфигурации. При частичном сбое повторный старт безопасно повторяет операцию; Hermes не запускается до успешного завершения.
7. Передачу управления через exec оригинальному `hermes gateway run --no-supervise`.

Восстановление не ограничено initContainer: обязателен restart приложения в том же Pod. Внутренний /restart или самостоятельно запущенный пользователем процесс не считается Kubernetes restart; обещание относится к запуску через штатную команду контейнера. Нужен тест, что upstream supervision не обходит её.

Input bundle read-only не делает writable config read-only. Runtime изменения не вызывают reconcile/rollout. Источники сохранённых overrides и ограничения API: [startup research](../research/startup-restore.md).

## 9. Workload и security context

Single-replica StatefulSet, RollingUpdate, OrderedReady; headless Service нужен для StatefulSet identity, не публикует web UI. Suspension задаёт replicas=0. Grace period — 60 секунд; пользователь заранее согласовал возможность прерывания запроса.

Обязательные параметры: runAsNonRoot=true, UID/GID/fsGroup=10000, allowPrivilegeEscalation=false, capabilities.drop=[ALL], seccompProfile=RuntimeDefault, readOnlyRootFilesystem=true. fsGroupChangePolicy=OnRootMismatch. Runtime config и пользовательский home при этом writable на PVC.

Не предоставляются hostNetwork/hostPID/hostIPC, hostPath, host ports, device mounts, runtime socket, privileged container или произвольный Pod template. ServiceAccount отдельный, без RoleBindings, automountServiceAccountToken=false и у SA, и у Pod. ImagePullSecrets не монтируются как файлы агента.

Безопасность не основана на неизменяемости Python-конфига или сокрытии per-user API key. Обычный контейнер разделяет kernel узла. Защита host kernel/CNI и отсутствие более широких разрешений других NetworkPolicy — ответственность установщика.

## 10. Сеть

Для каждого CR оператор создаёт NetworkPolicy до разрешения запуска Pod: ingress deny-all, egress — публичные назначения, явно настроенный DNS и индивидуальные исключения. Ошибка создания policy не допускает первоначального запуска. Удалённая policy восстанавливается; стандартный Kubernetes не даёт атомарной гарантии между удалением policy, наблюдением контроллера и применением CNI.

| Поле | Тип / default | Контракт |
| --- | --- | --- |
| `network.allowPrivate` | list = `[]` | Элементы `{ip: string, ports?: [{port: int, protocol?: TCP/UDP/SCTP}]}`. IP — один unicast адрес, не CIDR/hostname. |
| `network.allowPrivate[].ports` | optional nonempty list | Отсутствует — все порты/протоколы данного IP; protocol default TCP. |
| `network.additionalBlockedCIDRs` | set of CIDRs = `[]` | Дополнительные ограничения публичного egress этой инсталляции. |

Сначала публичное разрешение исключает специальные и кластерные диапазоны. Отдельное allowPrivate правило разрешает только указанный /32 или /128 и при необходимости порты; оно может явно открыть IP внутри заблокированной сети. DNS разрешается отдельно только UDP/TCP 53 к указанным resolver destinations.

Базовый IPv4 deny set: `0.0.0.0/8`, `10.0.0.0/8`, `100.64.0.0/10`, `127.0.0.0/8`, `169.254.0.0/16`, `172.16.0.0/12`, `192.0.0.0/24`, `192.0.2.0/24`, `192.88.99.0/24`, `192.168.0.0/16`, `198.18.0.0/15`, `198.51.100.0/24`, `203.0.113.0/24`, `224.0.0.0/4`, `240.0.0.0/4`. Глобально маршрутизируемые исключения IANA внутри special-purpose диапазонов выделяются при компиляции списка, чтобы не закрывать публичные сервисы только из-за широкого агрегата.

Для IPv6 публичный egress ограничен global-unicast диапазоном `2000::/3` с исключением специальных сетей, включая `2001::/23`, `2001:db8::/32`, `2002::/16`, `3fff::/20`; публичные исключения IANA выделяются отдельно. ULA, link-local, multicast, IPv4-mapped и NAT64 transition ranges по умолчанию не разрешаются. NAT64-only окружения в первую матрицу не входят. Политика покрывает обе семьи, чтобы вторую семью нельзя было использовать как обход.

Versioned список special-purpose CIDR и публичных исключений строится из [IANA IPv4](https://www.iana.org/assignments/iana-ipv4-special-registry/) и [IANA IPv6](https://www.iana.org/assignments/iana-ipv6-special-registry/); списки не загружаются динамически в каждом reconcile. К нему добавляются реальные Pod/Service/node/infrastructure CIDR из установки и additionalBlockedCIDRs из CR.

Если inference доступен через ClusterIP, DNAT может потребовать разрешить также backend IP. FQDN exceptions и динамическое разрешение Service selector не входят в v1. Проверка доступа выполняется на выбранном CNI, а не выводится из одного YAML.

NetworkPolicy additive, обработка NAT и node traffic зависит от CNI. Статус `NetworkPolicyReady` означает наличие актуального ресурса, а не доказанный запрет всей инфраструктуры. Ограничения и тесты установки описаны в [deployment contract](deployment.md).

## 11. Reconcile, обновления и ошибки

Контроллер идемпотентен: одинаковые входы не порождают новых revision или бесконечных status writes. Для полей CR, влияющих только на policy/PVC/deletion policy, перезапуск не нужен; version, config, используемые credentials, resources и placement меняют Pod template.

Revision hash вычисляется по нормализованному Pod/config input и выбранным Secret values. Namespace/name/UID источников учитываются; resourceVersion и неиспользуемые Secret keys не учитываются. Digest используется как opaque revision, секретные values и промежуточные хеши не логируются. Metadata-only изменения CR/Secret не вызывают рестарт.

Порядок reconcile: deletion → suspend → доступность желаемых credentials → schema/semantic validation → PVC identity → NetworkPolicy → revision inputs → StatefulSet → status/garbage collection. Перед сохранением прежнего workload при ошибке validation отдельно проверяются credentials его применённых revisions, включая ещё существующий Pod старой revision. Создание Pod не ждёт Bound при delayed binding. Secret create/update/delete и изменения owned resources должны повторно ставить CR в очередь.

При отсутствующем или пустом обязательном Secret: новая инсталляция ждёт; существующий workload останавливается, чтобы удаление credentials не оставляло старый credential snapshot работающим бесконечно. PVC сохраняется. Suspend и deletion должны работать независимо от отсутствующих Secrets.

При семантически некорректном новом spec контроллер не применяет его частично и сохраняет последний корректный workload, показывая `Ready=False`, `ConfigurationReady=False` для новой generation, только пока все его обязательные credentials доступны. Удаление Secret, удаление/опустошение ключа или замена UID источника применённой revision останавливает workload даже при невалидном новом spec и изменённых desired refs. Для этого revision Secret хранит в metadata только namespace-local имена источников, ключи и UID, без значений credentials. Если прежняя revision не содержит этих данных, невалидный spec не позволяет безопасно сохранить её работающей; валидный spec восстанавливает metadata после проверки содержимого immutable snapshot без рестарта Pod. После остановки возобновление требует и доступных credentials, и валидного spec. Инфраструктурные ошибки дают retry/backoff; устойчивые ошибки конфигурации ждут изменения входов. Автоматического отката версии/миграции пользовательской базы нет.

При нормальном rollout старый Pod останавливается до нового; никаких surge replicas. Force-delete Pod и сетевое разделение не дают абсолютного fencing и отдельно описываются в эксплуатации. Неисправный rollout после исправления/возврата spec должен восстанавливаться; если StatefulSet ждёт старый неготовый Pod, контроллер может штатно удалить принадлежащий ему Pod старой revision или с нарушенным управляемым securityContext с UID precondition, без force deletion. Совпадение revision annotation само по себе не подтверждает корректность фактического securityContext.

## 12. Status и health

Поля status: `observedGeneration`, `appliedRevision`, `resolvedImage`, `workloadRef`, `storageRef` (name/UID/origin), `conditions[]` в стандартном metav1.Condition формате. observedGeneration означает рассмотренную generation, а не успешный запуск.

Conditions: `ConfigurationReady`, `DependenciesReady`, `StorageReady`, `NetworkPolicyReady`, `Ready`, `Suspended`, `Degraded`. У каждой condition собственный observedGeneration. Ready=true только для актуальной generation/revision при готовом Pod и успешно прошедшей readiness probe. Suspended=true всегда сопровождается Ready=false.

Стабильные reasons включают InvalidConfiguration, UnsupportedVersion, DependencyNotFound, DependencyKeyMissing, DependencyIdentityUnknown, ResourceConflict, StorageIdentityMismatch, StoragePending, ResizePending, ResizeUnsupported, GatewayNotReady, RolloutInProgress, Suspended, Reconciled и ReconcileError. Неподдерживаемый provider/API mode даёт InvalidConfiguration; startup/bootstrap failure оставляет gateway неготовым, а подробность остаётся в Pod logs. Raw upstream stderr/error_message не копируются в status/Events.

Exec probes используют shipped probe script и штатный runtime status/heartbeat Hermes, не требуют web UI и не тратят model tokens. Проверяется живой PID с совпадающей process identity, heartbeat текущего процесса, gateway state и подключение Telegram. Точная схема состояния проверяется адаптером выбранного release.

Плановые интервалы: startup 10s × 60 попыток; readiness 10s, failureThreshold=3; liveness 30s, failureThreshold=3. Liveness проверяет локальный процесс/event loop; временная недоступность Telegram/model API не должна создавать restart loop. Ready не является проверкой баланса/квоты модели, доставленного ответа или отсутствия злонамеренной подмены health-файла самим владельцем контейнера.

## 13. Приёмка

Первый релиз разрешён только после всех обязательных сценариев:

| ID | Сценарий | Доказательство |
| --- | --- | --- |
| A01 | Две инсталляции в разных namespaces | Разные workload/PVC/credentials, отсутствие cross-namespace refs. |
| A02 | Разрешённый Telegram DM и персонализация | Ответ пользователю, сохранение имени/предпочтений, изменения SOUL/personality. |
| A03 | Чужой sender и запрещённая группа | Неавторизованный sender отклоняется, text/commands/media запрещённой группы не запускают agent turn; старый inline callback разрешённого sender проверяется и документируется как принятое ограничение v1. |
| A04 | Runtime смена модели, provider и reasoning | После same-Pod container restart CR восстановлен в прежней сессии; session ID/history неизменны. |
| A05 | Pod replacement, CR update, Secret rotation | Сохранены SOUL, личный config, memory, skills, cron, workspace, история. |
| A06 | Extra path удалён из CR | Удалено прежнее управляемое значение, пользовательский соседний ключ сохранён. |
| A07 | Retain/Delete/existing | Удаляется только явно выбранный созданный оператором PVC; повтор имени не усыновляет данные. |
| A08 | Missing Secret, bad config, corrupted home | Диагностика без секретов, домашние файлы не очищаются, после исправления запуск восстанавливается. |
| A09 | Security и egress | UID/caps/SA mounts проверены внутри Pod; реальный DNS/public/blocked private/allowed IP тест на CNI. |
| A10 | Rollout, suspension, operator restart | Один штатный poller, resume с тем же состоянием, reconcile после перезапуска оператора. |
| A11 | Storage resize/delayed binding | Нет deadlock WaitForFirstConsumer; expansion без потери данных; shrink отвергнут. |
| A12 | Chart install/upgrade/uninstall | RBAC достаточен и ограничен; CRD upgrade выполняется явно; retained PVC не исчезает при uninstall. |

Проверки с реальным Telegram/провайдером требуют выделенных тестовых credentials; отсутствие credentials не превращает пропущенный тест в passed. Изменение upstream-кода ради прохождения обязательного сценария требует отдельного обсуждения.
