# Персонализация

После запуска пользователь может настроить SOUL, personality, стиль, инструкции, workspace, skills, memory и schedules через оригинальный Hermes. Эти данные лежат на PVC и не являются управляемой конфигурацией CR.

При каждом штатном старте контейнера оператор восстанавливает административные поля модели, provider, endpoint, reasoning, Telegram authorization, tool/resource limits и разрешённые `extraConfig`/env. Он не заменяет весь home и не сбрасывает первичную персональную настройку. Удалённые из CR управляемые leaves очищаются, чтобы старый override не пережил изменение политики.

`display.personality` в Hermes имеет приоритет над `agent.system_prompt`; `SOUL.md`, workspace `AGENTS.md` и `.hermes.md` добавляют пользовательский контекст. Это штатная персонализация, а не способ изменить Kubernetes securityContext или NetworkPolicy.

Изменение model/reasoning внутри работающего агента может действовать до следующего запуска. При старте CR снова становится источником эталона, а активные session model overrides очищаются без удаления session ID и истории. Повреждённый YAML/.env или недоступный writable home оставляет gateway неготовым; подробность видна в Pod logs, а bootstrap не создаёт пустую замену поверх данных.

Удаление CR с `Retain` оставляет созданный PVC для ручного повторного подключения. Это сохранение данных, а не backup/restore. Автоматические snapshots, перенос данных между PVC и восстановление после потери storage в v1 отсутствуют.
