# Hermes Operator

Проект самостоятельного Kubernetes-оператора для декларативного запуска [Hermes Agent](https://github.com/NousResearch/hermes-agent).

Требования согласованы, спецификация первой версии и план реализации подготовлены. Реализации контроллера, CRD и установочного пакета ещё нет; контейнерные и кластерные испытания ещё не выполнялись.

## Согласованный первый релиз

- Одна инсталляция описывается namespaced custom resource, который создаёт администратор или уполномоченный backend.
- Одна установка оператора обслуживает весь кластер; ресурсы каждой инсталляции и её Secret/PVC references остаются в её namespace.
- Оригинальный Hermes без изменений исходного кода; официальный контейнерный образ.
- Личные сообщения Telegram как основной канал. Группы могут быть отдельным опциональным расширением; веб-интерфейс отложен.
- Эталонные модель, reasoning, Telegram-подключение и другие настройки задаются CR; credentials хранятся в Secrets. Runtime config writable, а перед каждым запуском заданные параметры восстанавливаются.
- Первичная персональная настройка, SOUL, личность и пользовательские инструкции доступны пользователю внутри агента и сохраняются на PVC.
- Восстановление конфигурации не сбрасывает SOUL, skills, память, историю, workspace и пользовательские ключи config.
- Workspace и всё изменяемое состояние агента сохраняются на persistent storage.
- После удаления инсталляции созданный оператором PVC сохраняется по умолчанию; его удаление включается явно. Подключённый существующий PVC остаётся внешним ресурсом.
- Изменение управляемой конфигурации, используемых credentials или версии Hermes автоматически перезапускает агента, сохраняя пользовательскую персонализацию.
- Стандартные ограничения привилегий Kubernetes и NetworkPolicy; публичный интернет открыт, внутренние назначения требуют явных исключений по IP.
- Автоматические backups/snapshots и восстановление не входят в первую версию; к ним вернутся отдельно.

## Документация проектирования

- [Термины](CONTEXT.md).
- [Принятые требования и история решений](docs/design/interview.md).
- [Спецификация API v1alpha1 и критерии приёмки](docs/design/v1alpha1.md).
- [Установка, Helm, RBAC и эксплуатационные границы](docs/design/deployment.md).
- [План реализации: восемь проверяемых этапов](docs/superpowers/plans/2026-09-18-hermes-operator.md).
- Проекты примеров CR: [минимальный](docs/design/examples/hermes-minimal.yaml), [локальный inference](docs/design/examples/hermes-local-inference.yaml), [существующий PVC](docs/design/examples/hermes-existing-pvc.yaml). Они станут применяемыми после реализации CRD.
- [Управление конфигурацией](docs/adr/0001-declarative-configuration-authority.md).
- [Оригинальный upstream и границы изоляции](docs/adr/0002-upstream-and-security-boundary.md).
- [Основные поля CR и дополнительные настройки Hermes](docs/adr/0003-typed-core-and-upstream-config.md).
- [Проверенные предпосылки и источники](docs/research/upstream-and-isolation.md).

## Работа над проектом

Задачи и спецификации ведутся в [GitHub Issues](https://github.com/wbe7/hermes-operator/issues). Правила инженерных skills настроены через [AGENTS.md](AGENTS.md): [issue tracker](docs/agents/issue-tracker.md), [triage labels](docs/agents/triage-labels.md), [domain docs](docs/agents/domain.md).
