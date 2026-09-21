# Итоговая проверка исходников и Berger Apps

Дата: 2026-09-21. Проверенный и развёрнутый код: `089fddd06ee8f95ee9ae55d1c4dcaf403b7ac39f`. Исходники готовы к интеграции с указанными эксплуатационными границами; полная [приёмка release](v1-acceptance.md) остаётся открытой. Публичный image/tag не опубликован.

## Ревью и локальные проверки

Одно полное ревью ветки и одно повторное ревью исправлений закрыли F1–F8: отзыв применённых credentials на ошибочных путях reconcile, сохранение повреждённого dotenv, проверку изоляции owned Pod, Service-safe имена CR и четыре замечания к тестам/документации/cwd. Новых дефектов в diff исправлений повторное ревью не установило.

API/envtest, controller, config и workload suites прошли; 23 runtime-теста прошли в оригинальном pinned Hermes image. `go vet ./...`, проверка генерации CRD/runtime assets и документации прошли. Upstream не изменялся.

Образ `hermes-operator:089fddd` собран для arm64 из git-архива точного коммита; image ID `sha256:7a01055a9cde03dd67303c003f475fc5331b04f6f2fe7c308bf9839912fb4b2a`, USER `65532:65532`. Локальный `.env` не попал в build context. Credentials отсутствуют в tracked files и локальной истории Git; `.env` ignored/untracked, mode0600. Предыдущий [аудит зависимостей и образа](build-verification.md) относится к указанной там сборке, а не к повторному сканированию этого image ID.

## Проверки в Berger Apps

Контроллер из точного source archive обновлён в `hermes-operator-system`; CRD применена исходным field manager `hermes-operator-dev`. Это dev source-build deployment, не Helm release. Архив исходников загружается через server-side apply: client-side last-applied annotation превышала 256KiB. Этот отказ затронул dev-упаковку, а не продуктовый Helm chart.

- Основной Hermes `hermes-operator-test/hermes-smoke`: generation6, `Ready=True`, revision `d54c92534496f5f4`, Pod `008e6415-01d1-4765-a911-d72c1b261ecd`, zero restarts. PVC UID `a0a97776-8dd2-436c-b8fc-6e4aade348b9`, 10Gi, сохранился.
- После замены Pod совпали 334 хеша пользовательских файлов и хеш личной конфигурации. Все прежние сообщения сохранились побайтно по хешу исходного префикса истории. Во время проверки добавились 13 новых сообщений; это не потеря данных и не отдельное доказательство всех Telegram acceptance cases. Read-only проверка не выводит содержимое переписки и не создаёт SessionStore.
- Native loader подтвердил модель/reasoning из CR и credential из mounted snapshot без вывода значения. Native terminal accessor подтвердил `/opt/data/workspace`; сохранённый dotenv больше не содержит deprecated cwd variables.
- На отдельной Pending-инсталляции смена desired Secret плюс конфликт ownership Service сохраняли прежний workload, пока старый источник был действителен. Удаление старого источника остановило Pod и установило replicas0, хотя новый Secret продолжал существовать и конфликт не был устранён. Это фактическая API/preflight проверка; отклонённый StatefulSet apply отдельно покрыт envtest с failure interceptor.
- На новом одноразовом PVC повреждённый dotenv дал exit1 с безопасной диагностикой до запуска gateway. Повреждённая строка, SOUL и workspace сохранились. После явного ремонта original-image bootstrap `--restore-only` завершился exit0 и сохранил персональные маркеры. Фиктивный токен не использовался для реального Telegram polling.
- Server-side dry-run отверг dotted и digit-led имена CR и принял имена длиной 1/40 символов. Ресурсы этой проверки не создавались.

Все одноразовые ресурсы этих регрессий очищены; основной CR/Secret/PVC сохранены. Локальные sanitized results: `/tmp/hermes-operator-deploy/final-{ready,persistence,controller-check,corrupt-dotenv,name-admission}.json`. Приватные live-state snapshots не предназначены для публикации.

## Выявленная граница label drift

Live-тест автоматической замены Pod после ручной подмены selector label не прошёл это более сильное ожидание: native StatefulSet controller успел снять ownerReference. Оператор обнаружил ownerless Pod, выставил `Ready=False/ResourceConflict` и отказался удалять чужой для него ресурс. Это не успешный тест автоматического ремонта.

Сохранена граница ownership: автоматическое присвоение ownerless Pod не добавлено. До ручного восстановления label такой Pod может оказаться вне селектора NetworkPolicy; Ready=False само по себе его не изолирует. После проверки происхождения администратор восстанавливает исходный label. На синтетическом Pending Pod это позволило завершить штатную очистку. End user не имеет Kubernetes-прав для изменения labels. Процедура и ограничения описаны в [operations](../guides/operations.md#изменение-управляющих-labels-вручную) и [security](../guides/security.md).

## Незакрытые release gates

Полная проверка разрешённого/постороннего Telegram пользователя, персонального onboarding и групп; две одновременно Ready native инсталляции; native Ready gateway через Helm upgrade; непроверенные platform/CNI/CSI combinations. Доступность публичного IPv6 отсутствовала в unrestricted baseline. Наличие executable harness и успешные Pending-fixture сценарии не закрывают эти gates. Принятое ограничение старых inline callbacks остаётся неизменным.
