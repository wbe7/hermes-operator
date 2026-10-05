# Распознавание речи (STT)

Доступно начиная с operator/chart `0.3.0`. При обновлении с `0.2.0` сначала
примените CRD нового релиза, затем обновите оператор. Версия самого Hermes остаётся
`v2026.9.14`; пересборка образа агента для этой функции не нужна.

## Минимальная настройка

```yaml
spec:
  stt:
    enabled: true
    model: qwen3-asr-1.7b
```

Этот фрагмент добавляется в существующий CR с `model` и `storage`; полный пример —
[hermes-stt.yaml](../../examples/hermes-stt.yaml). Распознавание использует
`spec.model.baseURL` и тот же Secret/key, что основная модель. Имя STT-модели
задаётся отдельно. Язык по умолчанию `ru`, показ расшифровки перед ответом — включён.

Поддерживается внешний OpenAI-compatible `POST /audio/transcriptions`. В `baseURL`
указывается API prefix, например `https://inference.example.com/v1`, без суффикса
`audio/transcriptions`. Endpoint должен поддерживать выбранную ASR-модель, а ключ —
иметь право её вызывать. Одного доступа ключа к LLM недостаточно.

## Отдельный endpoint или ключ

```yaml
spec:
  stt:
    enabled: true
    model: transcription-model
    baseURL: https://speech.example.com/v1
    apiKeySecretRef:
      name: maria-speech
      key: STT_API_KEY
    language: auto
    echoTranscripts: false
```

Secret `maria-speech` создаётся администратором в namespace CR. Оператор не
генерирует STT-ключи и не изменяет исходный Secret. Если явно задать
`apiKeySecretRef: {}`, используются основной Secret инсталляции и ключ
`STT_API_KEY`. Без `apiKeySecretRef` наследуется именно LLM reference, включая
нестандартные имя Secret и ключ; отдельный `STT_API_KEY` тогда не требуется.

URL и ключ переопределяются **независимо**: при изменении только URL основной
LLM-ключ отправляется новому endpoint. Чтобы выбрать другой ключ, явно задайте
`apiKeySecretRef`. Отсутствующий/пустой явно выбранный ключ даёт
`DependenciesReady=False`; автоматического возврата к LLM-ключу нет.

## Авторизация и язык

| `auth` | Выбор credentials |
| --- | --- |
| `Inherit` (default) | Явный STT Secret, если задан; иначе способ авторизации и Secret LLM |
| `APIKey` | Явный STT Secret либо наследование LLM-ключа. Если LLM имеет `auth: None`, ссылка STT обязательна |
| `None` | Реальный ключ не используется; `apiKeySecretRef` запрещён |

`None` подходит серверу, игнорирующему авторизацию: SDK Hermes всё равно может
отправлять `Authorization: Bearer no-key-required`. Это не режим удаления самого
HTTP-заголовка. Доступ к private endpoint требует обычного явного исключения
`network.allowPrivate`; STT не меняет сетевую политику автоматически.

`language` принимает lowercase код из двух/трёх букв (`ru`, `en` и т. п.) либо
`auto`. Для `auto` оператор убирает языковую подсказку из запроса: буквальная
строка `auto` в upstream API не передаётся. Поддержка конкретного языка зависит
от ASR-модели. `echoTranscripts: false` скрывает отдельное сообщение с расшифровкой,
но агент продолжает получать распознанный текст.

## Голосовые сообщения и ограничения

- Telegram voice messages распознаются автоматически. Обычные audio/document
  attachments остаются файлами, с которыми агент работает по запросу.
- Голосовой ответ (TTS), локальные STT-модели и иные протоколы провайдеров эта
  типизированная секция не настраивает.
- В pinned Hermes remote upload cap — 25 MiB, обычный Telegram download cap —
  20 MiB. SDK timeout — 30 секунд. Оператор не обещает его настройку через CR.
- После ошибки remote STT штатный gateway может попробовать уже установленный
  локальный backend. Upstream не изменён; строгая гарантия отсутствия fallback
  не предоставляется.
- `Ready=True` подтверждает готовность workload, а не доступность внешней
  ASR-модели или её квоты. HTTP 403 от inference часто означает отсутствие модели
  в allowlist ключа; проверьте реальный STT-запрос, а не только `/models`.

## Обновления, выключение и миграция

Изменение STT CR или используемого Secret запускает обычный rollout. При каждом
старте восстанавливаются заданные endpoint/key/model/language/echo, включая старые
STT aliases и флаги в `gateway.json`. SOUL, skills, память, workspace и история
не сбрасываются. Пользователь может временно менять runtime config; следующий
запуск снова применяет CR.

**Изменение прежнего поведения:** отсутствие `spec.stt` теперь означает
выключенный STT, а не upstream autodetect. Удаление секции или `enabled: false`
выключает распознавание при следующем запуске; отдельный STT Secret больше не
требуется. Это не запрет запускать собственные инструменты из workspace.

Перед обновлением проверьте CR и перенесите используемые настройки из
`extraConfig.stt` в `spec.stt`. Управляемые `enabled`, `provider`, `use_gateway`,
`language`, `echo_transcripts`, секция `openai`, aliases `stt_enabled`/
`stt_echo_transcripts` и их `gateway`-формы конфликтуют с typed API. Несовместимый
CR получает `InvalidConfiguration`; разрешённые дополнительные листья, например
`extraConfig.stt.prompt`, остаются доступны в пределах поддержки Hermes.

Удалите конкурирующие env `HERMES_STT_API_KEY`, `HERMES_LOCAL_STT_LANGUAGE`,
`STT_OPENAI_BASE_URL` из `extraEnv`/`credentials.env`. Общие ключи других инструментов,
например `OPENAI_API_KEY`, оператор для STT не перезаписывает.

Выкатывайте CRD перед оператором. Обновление оператора меняет startup bundle даже
для CR без STT: запланируйте рестарты инсталляций. Источники и результаты проверок:
[STT research и smoke](../research/stt-2026-10-05.md).

### Whisper model aliases

Pinned Hermes rewrites `whisper-large-v3`, `whisper-large-v3-turbo` and
`distil-whisper-large-v3-en` to `whisper-1` on its OpenAI-compatible backend,
including custom endpoints. The CRD and controller reject these exact names.
Configure a different server-side model alias (for example `asr-whisper-turbo`)
and put that alias in `spec.stt.model`. The alias must actually exist on the
server and be permitted by the selected API key. `whisper-1` itself is supported.
