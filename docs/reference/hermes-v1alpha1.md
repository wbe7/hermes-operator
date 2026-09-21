# Hermes API v1alpha1

`Hermes` — namespaced resource. CR, исходные Secrets, PVC и созданный workload всегда находятся в одном namespace. Имя CR — DNS label длиной до 40 символов: начинается с латинской буквы нижнего регистра, содержит только a-z, 0-9, дефис, заканчивается буквой или цифрой. Точки запрещены, чтобы производное имя Service было валидно. Единственная проверенная runtime-комбинация v1: Hermes `v2026.9.14`, provider `custom`, API mode `chat_completions`. Другие provider/mode отвергаются до запуска. Значение reasoning по умолчанию — `xhigh`.

## Поля spec

| Поле | Обязательность и default | Применение |
| --- | --- | --- |
| `version` | обязательно | Точный release из каталога оператора; `latest` не используется. |
| `image.repository` | `docker.io/nousresearch/hermes-agent` | Зеркало исходного Hermes image. |
| `image.digest` | digest каталога | Если задан, должен совпадать с каталогом. Workload всегда запускается по digest. |
| `image.pullPolicy` | `IfNotPresent` | `IfNotPresent` или `Always`. |
| `image.pullSecrets[]` | `[]` | Имена Secrets в том же namespace. |
| `credentials.secretName` | `<имя CR>-hermes-secret` | Обычный Secret для model/Telegram refs без явного имени. |
| `credentials.env.<ENV>.name/key` | `{}` | Дополнительные явно выбранные Secret keys для поддерживаемого `extraConfig`. |
| `model.provider` | обязательно | В v1 реализован только `custom`; `auto` запрещён. → `model.provider`. |
| `model.name` | обязательно | → `model.default`. |
| `model.baseURL` | обязательно для `custom` | HTTP(S), без credentials/query/fragment. → `model.base_url`; оператор также очищает более приоритетные endpoint aliases. |
| `model.apiMode` | `chat_completions` для v1 | → `model.api_mode`; другие режимы пока отвергаются. |
| `model.auth` | `APIKey` | `None` только для `custom`; фиктивный model key не нужен. |
| `model.apiKeySecretRef.name/key` | обычный Secret / `MODEL_API_KEY` | Для custom → `HERMES_MODEL_API_KEY` и его YAML reference. Не задаётся с `auth: None`. |
| `model.contextLength` | upstream default | → `model.context_length`. |
| `reasoning.effort` | `xhigh` | → `agent.reasoning_effort`. |
| `reasoning.overrides` | `{}` | → `agent.reasoning_overrides`; пустая map удаляет сохранённые overrides. |
| `telegram.botTokenSecretRef.name/key` | обычный Secret / `TELEGRAM_BOT_TOKEN` | Токен выделенного бота. |
| `telegram.allowedUserIDs[]` | обязательный непустой set | Положительные Telegram sender IDs, не username и не chat IDs. |
| `telegram.groups.enabled` | `false` | Включает перечисленные группы. |
| `telegram.groups.allowedChatIDs[]` | `[]`; непустой при enabled | Отрицательные group chat IDs. |
| `agent.maxTurns` | `50` | → `agent.max_turns`; `0` снимает лимит. |
| `agent.runBudgetSeconds` | `600` | → `agent.run_budget_seconds`; `0` снимает лимит. |
| `terminal.timeoutSeconds` | `300` | → `terminal.timeout`; backend/path задаёт оператор. |
| `tools.enabled[]` | upstream default | → Telegram platform toolsets. |
| `tools.disabled[]` | `[]` | → `agent.disabled_toolsets`; не может пересекаться с enabled. |
| `memory.charLimit` | upstream default | → `memory.memory_char_limit`. |
| `memory.userCharLimit` | upstream default | → `memory.user_char_limit`. |
| `resources.requests` | cpu `100m`, memory `512Mi`, ephemeral-storage `256Mi` | Kubernetes container requests. |
| `resources.limits` | cpu `2`, memory `2Gi`, ephemeral-storage `2Gi` | Kubernetes container limits. |
| `scheduling.nodeSelector/tolerations/affinity` | пусто | Безопасный Kubernetes scheduling subset. |
| `suspend` | `false` | Масштабирует workload до 0, сохраняя PVC. |
| `extraConfig` | отсутствует | Дополнительные безопасные Hermes config leaves. Конфликт с typed/credential/platform/path полями отвергается. |
| `extraEnv` | `{}` | Несекретные env. Зарезервированные operator/runtime names отвергаются. |
| `storage.create.size` | `10Gi` | Создаваемый PVC; размер можно только увеличивать. |
| `storage.create.storageClassName` | cluster default | Immutable после создания. |
| `storage.create.accessMode` | `ReadWriteOnce` | Также поддержан `ReadWriteOncePod`; immutable. |
| `storage.existingClaim` | альтернатива create | Существующий PVC; оператор его не меняет и не удаляет. |
| `storage.deletionPolicy` | `Retain` | `Delete` допустим только для созданного оператором PVC. |
| `network.allowPrivate[].ip` | `[]` | Точное unicast IP-исключение, не CIDR/hostname. |
| `network.allowPrivate[].ports[].port/protocol` | все порты; protocol `TCP` | Ограничивает исключение портами TCP/UDP/SCTP. |
| `network.additionalBlockedCIDRs[]` | `[]` | Дополнительные сети, исключённые из публичного egress. |

Ровно один из `storage.create` и `storage.existingClaim` обязателен. Secret refs и PVC refs не могут пересекать namespace.

## Status

`observedGeneration` означает, что generation рассмотрена, а не запущена. `appliedRevision`, `resolvedImage`, `workloadRef` и `storageRef` фиксируют применённую identity. Conditions: `ConfigurationReady`, `DependenciesReady`, `StorageReady`, `NetworkPolicyReady`, `Ready`, `Suspended`, `Degraded`. `Ready=True` требует актуальный revision и Ready Pod.

Стабильные reasons включают `InvalidConfiguration`, `UnsupportedVersion`, `DependencyNotFound`, `DependencyKeyMissing`, `DependencyIdentityUnknown`, `ResourceConflict`, `StorageIdentityMismatch`, `StoragePending`, `ResizePending`, `ResizeUnsupported`, `GatewayNotReady`, `RolloutInProgress`, `Suspended`, `Reconciled`, `ReconcileError`. Неподдерживаемый provider/API mode относится к `InvalidConfiguration`; startup/bootstrap failure наблюдается как неготовый gateway, а подробность проверяется в Pod logs. Secret values и raw upstream errors в status/Events не записываются.

## Telegram limitation

В запрещённой группе text, command и media отбрасываются, а неизвестный sender остаётся запрещён. У pinned upstream есть принятое ограничение: уже открытый старый inline picker разрешённого sender может изменить model/reasoning callback из такой группы. Оператор не содержит patch upstream для этого случая.
