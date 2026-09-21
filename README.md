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
