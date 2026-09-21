# Эксплуатация

## Изменения и ротация

Изменение CR, выбранного Secret key или `spec.version` создаёт новый immutable input revision и выполняет single-replica rollout без surge. Текущий запрос может быть прерван; termination grace period равен 60 секундам. Ротация: обновите тот же Secret или переключите ref на новый Secret, дождитесь `Ready=True`, затем удалите старый источник. Если обязательный Secret/key удалить, workload останавливается, но PVC сохраняется.

Runtime config writable. Изменения пользователя могут работать до следующего штатного старта контейнера; затем typed CR/Secret fields восстанавливаются. Пользовательские SOUL, personality, instructions, memory, skills, history, schedules и workspace сохраняются. Подмена процесса через внутренний `/restart` не является Kubernetes restart и не расширяет это обещание.

Startup restore принимает строковую форму `model: name`, поддерживаемую Hermes, и восстанавливает управляемые поля модели. При ротации ключей учитываются адреса `api`, `url` и `base_url` в `providers` с тем же приоритетом, что у Hermes; credentials других endpoints сохраняются. `OPENROUTER_API_KEY` и `OPENROUTER_BASE_URL` управляются только при основном endpoint на `openrouter.ai` или его поддомене. Для другого inference независимые пользовательские настройки OpenRouter сохраняются. При обновлении с прежней версии ошибочно управляемые OpenRouter aliases удаляются по manifest владения; если нужен отдельный OpenRouter key, его следует настроить заново после такого обновления.

## Storage

Созданный PVC по умолчанию имеет `Retain`; `Delete` удаляет только claim, созданный оператором, после остановки Pod. Existing PVC всегда внешний и не изменяется. Retention не является backup: удаление namespace, PV или storage backend может уничтожить данные.

При delayed binding workload создаётся до `Bound`; `StoragePending` ожидаем до scheduling/provisioning. Увеличение размера требует StorageClass `allowVolumeExpansion`; до фактического расширения виден `ResizePending`. Shrink запрещён. Для существующего PVC заранее обеспечьте UID/GID/fsGroup `10000` write access к Hermes home; оператор не выполняет chown существующего содержимого.

## Чистая тестовая инсталляция без удаления прежних данных

Для сброса smoke используйте новый пустой PVC, а прежний том сохраните.
Рекомендуемый путь: задайте старому CR `spec.suspend: true`, дождитесь полного
исчезновения его Pod и создайте новую инсталляцию с другим именем и новым PVC.
Только после остановки прежнего gateway запускайте новый с тем же Telegram
token. Два polling-процесса одного бота конфликтуют; два независимо работающих
Telegram-агента требуют разных bot tokens.

Если нужно удалить старый CR, например для повторного использования его имени,
сначала проверьте источник storage. При `spec.storage.create` **до отправки
delete** установите `spec.storage.deletionPolicy: Retain` и убедитесь чтением CR
из API, что значение сохранено, а `metadata.deletionTimestamp` ещё отсутствует.
При `existingClaim` том внешний и удаление CR его не удаляет. После этого можно
удалить CR и дождаться завершения удаления CR и его Pod. При `Delete` созданный
оператором PVC будет удалён: такой вариант не подходит для сохранения данных.
Не рассчитывайте изменить политику после начала удаления — finalizer фиксирует
её в аннотации и затем использует зафиксированное значение.

Источник storage immutable: заменить `existingClaim` у прежнего CR нельзя.
Создайте новый CR с нужным storage. При повторном использовании имени CR
учитывайте `<name>-hermes-data`: сохранённый PVC прежнего `storage.create`
принадлежит старому UID и автоматически не усыновляется. Для чистого home выберите
новое имя CR либо заранее создайте новый PVC и укажите его через `existingClaim`.
Для возврата старых данных остановите новый gateway с тем же токеном и
возобновите прежний CR. Если подключаете его PVC к новому CR через `existingClaim`,
сначала безопасно удалите прежний CR по правилам выше и остановите остальные
потребители тома: даже suspended CR сохраняет ссылку
на PVC, а два CR с одним claim запрещены. Не удаляйте namespace, retained PVC или
финализаторы ради сброса.

Этот процесс очищает состояние агента в Kubernetes, но не удаляет переписку из
Telegram. Первое знакомство проверяется по пустой native истории до первого
нового сообщения и по реально сохранённой персонализации после него.

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
- `InvalidConfiguration`, `UnsupportedVersion`: исправить CR. Неподдерживаемый provider/API mode сообщается как `InvalidConfiguration`; последний валидный workload сохраняется только при доступных применённых credentials и соответствующей текущим сетевым требованиям NetworkPolicy. Если сетевые требования нельзя проверить, workload останавливается.
- `NetworkPolicyMissing` / `NetworkPolicyDrift`: при невалидном CR потеря или изменение policy останавливают workload. Исправьте CR; оператор восстановит принадлежащую ему policy перед возобновлением запуска. Чужая или удаляемая policy требует устранить конфликт владения/удаления. PVC сохраняется. Остановка асинхронна: между удалением policy, reconcile и завершением Pod остаётся окно без гарантии изоляции.
- `DependencyIdentityUnknown`: исправить CR и восстановить доступность применённых credentials; контроллер не оставляет workload работать, если identity прежнего источника нельзя безопасно подтвердить.
- NetworkPolicyReady означает актуальный объект, но не доказательство CNI enforcement. Выполните разрешённый и запрещённый egress smoke из того же Pod context.

При failed rollout исправьте CR или верните прежний `spec.version`. Возврат версии не гарантирует rollback данных: upstream мог изменить SQLite/schema. Не force-delete finalizers как штатное решение. Перед uninstall удалите ненужные CR и дождитесь finalizers; chart uninstall не удаляет CRD, CR или retained PVC.

StatefulSet должен использовать `RollingUpdate` без ненулевого partition; оператор восстанавливает эту стратегию после внешних изменений. Прямая замена image у owned Pod обнаруживается и приводит к штатному пересозданию с ожидаемым образом, даже если прежняя revision annotation сохранилась. Kubernetes defaults не считаются изменением управляемой конфигурации.

## Изменение управляющих labels вручную

Не изменяйте `hermes.wbe7.github.io/installation-uid` и ownerReferences созданного Pod. Пока Pod остаётся owned, оператор обнаруживает несовпадение изоляции и удаляет его с UID precondition. Но StatefulSet controller может первым снять ownerReference после изменения selector label. Тогда оператор показывает `Ready=False`, `ResourceConflict` и не удаляет уже ownerless ресурс. Автоматическое восстановление label в этой гонке не гарантируется.

Администратор должен проверить происхождение Pod, его UID, PVC и StatefulSet, затем вернуть исходный installation-uid из metadata.uid соответствующего Hermes. StatefulSet сможет снова принять Pod; после этого проверьте ownerReference, выбор Pod политикой NetworkPolicy и `Ready=True`. Если происхождение не подтверждено, не присваивайте ресурс инсталляции. Не удаляйте PVC и не снимайте finalizers для такого восстановления. До восстановления label NetworkPolicy инсталляции может не выбирать этот Pod; `Ready=False` само по себе не является сетевым запретом. Пользователь агента не имеет Kubernetes-прав для такого изменения.
