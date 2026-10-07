# Синтез речи (TTS)

Доступно в текущей ветке разработки; опубликованный operator/chart 0.3.0 ещё не
содержит `spec.tts`. Перед обновлением оператора примените CRD соответствующего
нового релиза. Hermes остаётся v2026.9.14, ядро и образ агента не меняются.

## Минимальная настройка

```yaml
spec:
  tts:
    enabled: true
    model: fish-s2-pro
    voice: default
```

Без секции или с `enabled: false` штатный TTS выключен при запуске. `model` и
`voice` обязательны при включении. Voice — имя/ID, поддерживаемый вашим сервером;
оператор не регистрирует и не клонирует голоса. На тестовой S2 Pro доступен
`default`; OpenAI-голос `alloy` она не принимает.

## Подключение и Secrets

`baseURL` и ключ независимо наследуются от `spec.model`. Наличие `spec.stt`
никак не влияет на подключение TTS. Можно переопределить только URL, только ключ
или оба:

```yaml
spec:
  tts:
    enabled: true
    model: speech-model
    voice: voice-id
    baseURL: https://speech.example.com/v1
    auth: APIKey
    apiKeySecretRef:
      name: speech-credentials
      key: api-key
```

| Поле/режим | Поведение |
| --- | --- |
| `auth: Inherit` (default) | Явный TTS ref имеет приоритет, иначе наследуются модельные auth/name/key |
| `auth: APIKey` | Явный ref или модельный ключ; при `model.auth: None` ref обязателен |
| `auth: None` | Без Secret ref; SDK получает placeholder `no-key-required` и может отправлять его как Bearer |
| `apiKeySecretRef` отсутствует | Используется фактический Secret/key LLM, а не условный TTS_API_KEY |
| `apiKeySecretRef: {}` | Основной Secret инсталляции, ключ `TTS_API_KEY` |
| Ref без `name` | `credentials.secretName` либо `<имя>-hermes-secret` |
| Ref без `key` | `TTS_API_KEY` |

References ограничены namespace инсталляции. Явный отсутствующий Secret/key
останавливает workload с DependencyNotFound/DependencyKeyMissing; fallback на
LLM credentials не происходит. Ротация ключа создаёт новую revision. Выключенный
TTS не требует своего Secret. Значения credentials не помещаются в CR/status или
открытый ConfigMap. URL не допускает userinfo, query и fragment.

## Голосовые ответы

```yaml
spec:
  tts:
    enabled: true
    model: fish-s2-pro
    voice: default
    responseMode: VoiceOnly
    speed: 1.0
    # language: ru
```

| `responseMode` | Поведение |
| --- | --- |
| `VoiceOnly` (default) | Автоматический голосовой ответ на входящие голосовые Telegram |
| `All` | Автоматическая озвучка всех ответов Telegram |
| `OnRequest` | Автоозвучка выключена; агент может вызвать TTS-инструмент по просьбе пользователя |

Текст сохраняется: короткий текст — подпись к аудио, длинный — отдельное
сообщение. При ошибке синтеза остаётся текстовый ответ. Для распознавания входящего
голосового отдельно включите [STT](stt.md). TTS не включает STT автоматически.
Не добавляются новый web voice UI или новые платформы доставки.

`speed` — число 0.25–4, default 1.0. `language` — необязательная строка без default;
Hermes отправляет её как `lang_code`. Сервер должен поддерживать именно этот
параметр. Для текущей S2 Pro поле не задавайте: она его не использует. При удалении
поля из CR старая управляемая языковая подсказка очищается при следующем запуске.

## Перезапуски и переход со старой конфигурации

Все управляемые параметры, включая изменения `/voice`, восстанавливаются из CR
при запуске. Оператор задаёт штатные per-chat режимы для разрешённых Telegram
user/group IDs и удаляет прежние delivery overrides из `gateway_voice_mode.json`.
Этот файл содержит предпочтения доставки, а не историю. SOUL, skills, память,
workspace и диалоги остаются на PVC.

При выключении оператор добавляет `tts` в disabled toolsets и сбрасывает
автоматические ответы. При включении явный список `tools.enabled` дополняется
`tts`; `tools.disabled: [tts]` конфликтует с включённым TTS. Другие toolsets
сохраняются. Это управление конфигурацией при запуске: пользователь по-прежнему
может временно перенастроить свой агент или вызвать API из произвольного кода.

До обновления перенесите ручные настройки из `extraConfig.tts`, `voice.auto_tts`
и пользовательских env overrides в `spec.tts`: отсутствие новой секции теперь
явно выключает TTS, включая прежние `/voice` overrides. Конфликтующие управляемые
поля в extraConfig/env отклоняются. Неуправляемые siblings сохраняются.

## Совместимость endpoint и проверка

Нужен OpenAI-compatible `/audio/speech`. Нативная доставка Telegram запрашивает
Opus; сервер должен уметь возвращать этот формат. Сам факт присутствия модели в
`/models` не доказывает работу speech endpoint или наличие доступа у вашего ключа.
Private endpoints требуют явных сетевых исключений; TTS URL не открывает сеть.

Ошибка Opus на S2 Pro устранена в inference backend: образ
`wbe7/vllm-omni:0b0d2d7-opus1` преобразует несовместимые частоты в 48 кГц
перед кодированием. Подтверждены HTTP 200 для Opus/MP3/WAV через внешний LiteLLM,
распознавание тестовой речи из Opus и создание voice-compatible Opus штатным
Hermes v2026.9.14 с Telegram-платформой. Ядро Hermes не изменялось.

Это подтверждает API и формат аудио; сквозная отправка в Telegram пока отдельно
не проверена. История ошибки, образ и результаты — в
[исследовании](../research/tts-2026-10-06.md).
