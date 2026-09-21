# Эксплуатация

## Изменения и ротация

Изменение CR, выбранного Secret key или `spec.version` создаёт новый immutable input revision и выполняет single-replica rollout без surge. Текущий запрос может быть прерван; termination grace period равен 60 секундам. Ротация: обновите тот же Secret или переключите ref на новый Secret, дождитесь `Ready=True`, затем удалите старый источник. Если обязательный Secret/key удалить, workload останавливается, но PVC сохраняется.

Runtime config writable. Изменения пользователя могут работать до следующего штатного старта контейнера; затем typed CR/Secret fields восстанавливаются. Пользовательские SOUL, personality, instructions, memory, skills, history, schedules и workspace сохраняются. Подмена процесса через внутренний `/restart` не является Kubernetes restart и не расширяет это обещание.

## Storage

Созданный PVC по умолчанию имеет `Retain`; `Delete` удаляет только claim, созданный оператором, после остановки Pod. Existing PVC всегда внешний и не изменяется. Retention не является backup: удаление namespace, PV или storage backend может уничтожить данные.

При delayed binding workload создаётся до `Bound`; `StoragePending` ожидаем до scheduling/provisioning. Увеличение размера требует StorageClass `allowVolumeExpansion`; до фактического расширения виден `ResizePending`. Shrink запрещён. Для существующего PVC заранее обеспечьте UID/GID/fsGroup `10000` write access к Hermes home; оператор не выполняет chown существующего содержимого.

## Диагностика rollout

```bash
kubectl get hermes -A
kubectl describe hermes maria -n hermes-users
kubectl get statefulset,pod,pvc,networkpolicy -n hermes-users
kubectl logs deployment/hermes-operator -n hermes-system
kubectl logs maria-hermes-0 -n hermes-users
```

- `DependencyNotFound` / `DependencyKeyMissing`: проверить имя Secret, key и namespace; не публиковать значение.
- `StoragePending` / `ResizePending`: проверить StorageClass, Events, binding и expansion.
- `GatewayNotReady` / `RolloutInProgress`: проверить image pull, Pod events, readiness, Pod logs, права PVC, сохранённый config и конфликт второго Telegram polling consumer. Startup/bootstrap failure не заменяет повреждённый home пустым.
- `InvalidConfiguration`, `UnsupportedVersion`: исправить CR. Неподдерживаемый provider/API mode сообщается как `InvalidConfiguration`; последний валидный workload сохраняется только при доступных применённых credentials.
- `DependencyIdentityUnknown`: исправить CR и восстановить доступность применённых credentials; контроллер не оставляет workload работать, если identity прежнего источника нельзя безопасно подтвердить.
- NetworkPolicyReady означает актуальный объект, но не доказательство CNI enforcement. Выполните разрешённый и запрещённый egress smoke из того же Pod context.

При failed rollout исправьте CR или верните прежний `spec.version`. Возврат версии не гарантирует rollback данных: upstream мог изменить SQLite/schema. Не force-delete finalizers как штатное решение. Перед uninstall удалите ненужные CR и дождитесь finalizers; chart uninstall не удаляет CRD, CR или retained PVC.

## Изменение управляющих labels вручную

Не изменяйте `hermes.wbe7.github.io/installation-uid` и ownerReferences созданного Pod. Пока Pod остаётся owned, оператор обнаруживает несовпадение изоляции и удаляет его с UID precondition. Но StatefulSet controller может первым снять ownerReference после изменения selector label. Тогда оператор показывает `Ready=False`, `ResourceConflict` и не удаляет уже ownerless ресурс. Автоматическое восстановление label в этой гонке не гарантируется.

Администратор должен проверить происхождение Pod, его UID, PVC и StatefulSet, затем вернуть исходный installation-uid из metadata.uid соответствующего Hermes. StatefulSet сможет снова принять Pod; после этого проверьте ownerReference, выбор Pod политикой NetworkPolicy и `Ready=True`. Если происхождение не подтверждено, не присваивайте ресурс инсталляции. Не удаляйте PVC и не снимайте finalizers для такого восстановления. До восстановления label NetworkPolicy инсталляции может не выбирать этот Pod; `Ready=False` само по себе не является сетевым запретом. Пользователь агента не имеет Kubernetes-прав для такого изменения.
