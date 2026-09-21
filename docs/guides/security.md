# Граница безопасности

Оператор использует отдельный non-root ServiceAccount. Контроллеру нужны cluster-wide CRUD права только на управляемые workload/config/storage/network resources, metadata watch Secrets и status/finalizer Hermes; ему не выдаются cluster-admin, exec/attach, Nodes или RBAC mutation. Hermes ServiceAccount не имеет RoleBinding и не получает API token.

Agent Pod запускается с UID/GID/fsGroup 10000, `runAsNonRoot`, `allowPrivilegeEscalation: false`, `drop: ALL`, `RuntimeDefault` seccomp и read-only root filesystem. Writable home находится на PVC. Это уменьшает риск, но не является абсолютной защитой от kernel/container escape.

NetworkPolicy закрывает ingress и private/special egress, разрешает публичные назначения, заданный DNS и точные `network.allowPrivate` IP. Политики Kubernetes складываются; другая разрешающая policy может расширить доступ. NAT/node traffic и enforcement зависят от CNI. Публичные infrastructure ranges нужно явно включить в `infrastructureCIDRs`.

Для local inference по HTTP используйте `provider: custom`, `apiMode: chat_completions`, `auth: None` и точное IP-исключение, как в [примере](../../examples/hermes-local-inference.yaml). Hostname/CIDR exception и произвольный Responses endpoint в v1 не поддержаны.

Credentials одной инсталляции доступны процессам внутри её собственного контейнера по принятой продуктовой границе. Не используйте общий client key. Secrets должны быть namespaced и выделены инсталляции; оператор копирует только выбранные keys в owned revision Secret и не записывает values в CR, status, Events, ConfigMap или логи.

Не добавляйте credentials в values, manifests, examples или CI variables для untrusted pull requests. Release publication использует GitHub environments/secrets только в tag workflow; PR workflow не получает privileged publishing credentials.

Изоляция предполагает, что доверенный администратор не меняет selector labels и ownership управляемых ресурсов. При гонке с StatefulSet controller изменение label может оставить ownerless Pod вне селектора политики; оператор сообщает `ResourceConflict`, но не присваивает и не удаляет чужой ресурс. Требуется [административное восстановление](operations.md#изменение-управляющих-labels-вручную). Это не действие, доступное конечному пользователю агента.
