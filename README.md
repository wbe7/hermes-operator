# Hermes Operator

Проект самостоятельного Kubernetes-оператора для декларативного запуска [Hermes Agent](https://github.com/NousResearch/hermes-agent).

Репозиторий содержит API `v1alpha1`, контроллер, Helm chart, runtime adapter и проверяемые примеры. Публикация первого release image самого оператора и полная кластерная приёмка выполняются отдельно. Образ агента `wbe7/hermes:v2026.9.14` опубликован для amd64/arm64; [проверки и границы](docs/research/agent-image-validation.md).

## Согласованный первый релиз

- Одна инсталляция описывается namespaced custom resource, который создаёт администратор или уполномоченный backend.
- Одна установка оператора обслуживает весь кластер; ресурсы каждой инсталляции и её Secret/PVC references остаются в её namespace.
- Оригинальный Hermes без изменений исходного кода; образ `wbe7/hermes` на базе официального с готовым браузером, офисными/PDF инструментами и OCR для amd64/arm64.
- Личные сообщения Telegram как основной канал. Группы могут быть отдельным опциональным расширением; веб-интерфейс отложен.
- Эталонные модель, reasoning, Telegram-подключение и другие настройки задаются CR; credentials хранятся в Secrets. Runtime config writable, а перед каждым запуском заданные параметры восстанавливаются.
- Первичная персональная настройка, SOUL, личность и пользовательские инструкции доступны пользователю внутри агента и сохраняются на PVC.
- Восстановление конфигурации не сбрасывает SOUL, skills, память, историю, workspace и пользовательские ключи config.
- Workspace и всё изменяемое состояние агента сохраняются на persistent storage.
- После удаления инсталляции созданный оператором PVC сохраняется по умолчанию; его удаление включается явно. Подключённый существующий PVC остаётся внешним ресурсом.
- Изменение управляемой конфигурации, используемых credentials или версии Hermes автоматически перезапускает агента, сохраняя пользовательскую персонализацию.
- Стандартные ограничения привилегий Kubernetes и NetworkPolicy; публичный интернет открыт, внутренние назначения требуют явных исключений по IP.
- Автоматические backups/snapshots и восстановление не входят в первую версию; к ним вернутся отдельно.

## Быстрый старт

Начните с [руководства по установке](docs/guides/install.md). Перед Helm install нужно указать реальные cluster CIDR/DNS и подтвердить NetworkPolicy enforcement. Затем используйте [Secret template](examples/hermes-secret.yaml) и один из проверяемых примеров:

- [минимальная инсталляция](examples/hermes-minimal.yaml);
- [local inference без model key](examples/hermes-local-inference.yaml);
- [существующий PVC](examples/hermes-existing-pvc.yaml).

API и defaults перечислены в [справочнике v1alpha1](docs/reference/hermes-v1alpha1.md). Ротация, storage и failed rollout описаны в [operations guide](docs/guides/operations.md), границы изоляции — в [security guide](docs/guides/security.md), сохранение пользовательской настройки — в [personalization guide](docs/guides/personalization.md).

Состав инструментов, сборка `wbe7/hermes`, проверка браузера/документов и публикация
multiarch image описаны в [руководстве по образу агента](docs/guides/agent-image.md).

Для разработки:

```bash
make test-unit test-envtest test-runtime lint-chart verify-docs verify-generated
```

`test-runtime` требует заранее загруженный официальный Hermes image по digest. `test-envtest` использует временный API server, а не live cluster.

<a id="updating-hermes-image"></a>

## Обновление образа Hermes

Версия сейчас закреплена в нескольких файлах; единого параметра версии для всего
репозитория нет. Тег `wbe7/hermes` совпадает с тегом официального Hermes. Оператор
выбирает image по **multiarch digest** из каталога поддерживаемых релизов: одного
push нового тега для обновления инсталляций недостаточно.

Публикация запускается вручную: локально либо через `Hermes image` с
`publish=true`. PR и push в `main` запускают только сборку и проверки. Первая
сборка `v2026.9.14` опубликована локально; для публикации из CI нужны repository
secrets `DOCKERHUB_USERNAME` и `DOCKERHUB_TOKEN` с правом push в `wbe7/hermes`.

### 1. Выбрать тип обновления и согласовать закреплённые значения

При **пересборке той же версии Hermes** (например, обновлении Chromium или PDF
библиотек) сохраняйте тег, обновляйте нужные зависимости и регистрируйте новый
digest после проверки. Для **нового upstream-релиза** сначала проверьте изменения
config, credentials, Telegram, хранилища состояния и health API. Текущий runtime
поддерживает только `v2026.9.14`; новый тег требует поддержки в операторе.

Выберите тег и получите digest официального manifest list:

```bash
HERMES_TAG=v2026.9.14 # замените на выбранный тег upstream
HERMES_IMAGE="wbe7/hermes:${HERMES_TAG}"
docker buildx imagetools inspect "nousresearch/hermes-agent:${HERMES_TAG}"
```

Верхний `Digest` — значение для `FROM` и `UpstreamImageDigest`; убедитесь, что
manifest содержит amd64 и arm64. Digest одной архитектуры здесь не подходит.

| Где | Что проверить или обновить |
| --- | --- |
| [Dockerfile](images/hermes/Dockerfile) | Официальный `FROM`: тег и digest manifest list с amd64/arm64; version и upstream-digest labels. |
| [Image workflow](.github/workflows/hermes-image.yml), [Makefile](Makefile), [smoke runner](images/hermes/run-smoke.sh) | Все build/test/push tags и defaults должны указывать на целевую версию. |
| [Каталог релизов](internal/runtimecatalog/catalog.go) и [его тесты](internal/runtimecatalog/catalog_test.go) | Версия, `ImageDigest` нашего образа, `UpstreamImageDigest` официального образа, adapter, UID/GID. |
| [Основной CI](.github/workflows/ci.yml), [runtime runner](test/runtime/run.sh), [native resolver test](internal/config/runtime_test.go) | Digest официального образа в pull и fallback-настройках тестов. Сохраните покрытие старых поддерживаемых релизов. |
| [Образ и зависимости](images/hermes/) | При смене библиотек обновите соответствующие `.in`/`.lock` и `package.json`/`package-lock.json`; Python lock-файлы содержат hashes и учитывают constraints исходного Hermes. При смене релиза согласуйте также version metadata npm-пакета. |
| [Примеры](examples/), [API reference](docs/reference/hermes-v1alpha1.md), [compatibility](docs/reference/compatibility.md), [руководство по образу](docs/guides/agent-image.md), README | Актуальные версии, digest и границы поддержки. Старые датированные отчёты сохраняйте как историческое доказательство. |

Для нового релиза дополнительно расширьте [Go validation](internal/config/validate.go),
[bootstrap](runtime/bootstrap.py), [probes](runtime/probe.py),
[adapters](runtime/adapters/), [выбор runtime assets](internal/workload/bootstrap_assets.go)
и [список генерируемых файлов](internal/workload/generate_assets.py). Обновите их
тесты, сохраните поддержку ранее зарегистрированных релизов и выполните
`make generate-runtime-assets`. Редактируются исходники в `runtime/`; копии в
`internal/workload/assets/` генерируются. Hermes upstream остаётся без patchset.

Чтобы найти оставшиеся привязки, выполните `rg -n --hidden 'v2026\.9\.14|v20260914|99641e57|feecb5d7' README.md Makefile .github images internal runtime test examples docs`;
при следующем обновлении замените поисковые значения на текущие. Каждое совпадение
нужно отнести к новому релизу, сохранённой поддержке старого или историческому отчёту.

### 2. Собрать и проверить обе архитектуры

При публикации через CI сборка и эти проверки выполняются самим workflow из шага 3.
Локальный вариант требует Docker с containerd image store и поддержкой запуска обеих
платформ (native или через эмуляцию; [требования Docker](https://docs.docker.com/build/building/multi-platform/)).
После обновления файлов из шага 1, с тем же `HERMES_IMAGE`:

```bash
(
  set -e
  docker buildx build --platform linux/amd64,linux/arm64 --load \
    --tag "$HERMES_IMAGE" --file images/hermes/Dockerfile images/hermes
  for arch in amd64 arm64; do
    DOCKER_DEFAULT_PLATFORM="linux/$arch" HERMES_TOOLS_IMAGE="$HERMES_IMAGE" make test-hermes-image
    DOCKER_DEFAULT_PLATFORM="linux/$arch" HERMES_RUNTIME_IMAGE="$HERMES_IMAGE" make test-runtime
    DOCKER_DEFAULT_PLATFORM="linux/$arch" HERMES_RUNTIME_IMAGE="$HERMES_IMAGE" \
      HERMES_RUNTIME_TEST=1 go test -count=1 -run TestOfficialRuntime -v ./internal/config
  done
)
```

Продолжайте после успешного завершения **всех** проверок обеих архитектур.
В CI те же проверки выполняются на отдельных native runners. Если добавлен новый
runtime adapter, дополнительно проверьте восстановление конфигурации и сохранение
персонализации/истории при обновлении реального тестового агента.

### 3. Опубликовать проверенную сборку

**Локально:** после успешного шага 2 выполните `docker login`, затем
`docker push "$HERMES_IMAGE"`. Публикуйте именно проверенный локальный image;
повторная сборка перед push может дать другой результат из-за обновлений apt.

**Из CI:** настройте repository secrets выше и доступ к environment `release`.
Workflow должен присутствовать в default branch ([правило GitHub](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/manually-run-a-workflow)).
В Actions → Hermes image → Run workflow выберите ветку с подготовленным обновлением
и включите `publish`. Параметр выбирает публикацию, **версия берётся из файлов
выбранной ветки**. CI заново собирает и проверяет каждую платформу, публикует
проверенные варианты и объединяет их в общий тег. Дождитесь успеха job `publish`.

После любого способа выполните `docker buildx imagetools inspect "$HERMES_IMAGE"`:
registry должен отдавать amd64 и arm64. Сохраните верхний digest manifest list для
`ImageDigest`, а официальный digest — отдельно для `UpstreamImageDigest`.

### 4. Зарегистрировать digest и обновить инсталляции

Запишите опубликованный digest в каталог релизов и его тесты, обновите compatibility
reference и отчёт о проверке. Выполните
`make test-unit test-envtest lint-chart verify-docs verify-generated`, добейтесь
зелёного CI и выпустите оператор с обновлённым каталогом. Для новой версии измените
`spec.version` CR; для нашего образа repository должен быть `docker.io/wbe7/hermes`.
Явный `spec.image.digest` должен соответствовать поддерживаемому digest выбранного
релиза, либо поле можно убрать для выбора из каталога.

**Обновление каталога для существующей версии может перезапустить все использующие
её инсталляции без явного digest.** Перемещение тега в Docker Hub само по себе этого
не делает. Сначала проверьте обновление на тестовой инсталляции: `Ready=True`,
`status.resolvedImage`, Telegram, браузер/документы и сохранность прежнего PVC,
SOUL, skills, памяти и истории. Это критерий завершения обновления. Возврат старого
image требует соответствующего каталога/CR; совместимость данных при откате
проверяется отдельно — см. [эксплуатацию](docs/guides/operations.md#диагностика-rollout).

## Документация проектирования

- [Термины](CONTEXT.md).
- [Принятые требования и история решений](docs/design/interview.md).
- [Спецификация API v1alpha1 и критерии приёмки](docs/design/v1alpha1.md).
- [Установка, Helm, RBAC и эксплуатационные границы](docs/design/deployment.md).
- [План реализации: восемь проверяемых этапов](docs/superpowers/plans/2026-09-18-hermes-operator.md).
- История проектов примеров: [минимальный](docs/design/examples/hermes-minimal.yaml), [локальный inference](docs/design/examples/hermes-local-inference.yaml), [существующий PVC](docs/design/examples/hermes-existing-pvc.yaml). Применяемые проверяемые копии находятся в `examples/`.
- [Управление конфигурацией](docs/adr/0001-declarative-configuration-authority.md).
- [Оригинальный upstream и границы изоляции](docs/adr/0002-upstream-and-security-boundary.md).
- [Основные поля CR и дополнительные настройки Hermes](docs/adr/0003-typed-core-and-upstream-config.md).
- [Проверенные предпосылки и источники](docs/research/upstream-and-isolation.md).

## Проверки и границы готовности

[Итоговая проверка исходников и Berger Apps](docs/research/final-validation.md) фиксирует развёрнутый коммит, пройденные проверки и обнаруженные границы. Полная [release-приёмка](docs/research/v1-acceptance.md) остаётся открытой.

## Работа над проектом

Задачи и спецификации ведутся в [GitHub Issues](https://github.com/wbe7/hermes-operator/issues). Правила инженерных skills настроены через [AGENTS.md](AGENTS.md): [issue tracker](docs/agents/issue-tracker.md), [triage labels](docs/agents/triage-labels.md), [domain docs](docs/agents/domain.md).
