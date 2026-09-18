# Hermes Operator v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Выпустить самостоятельный Kubernetes-оператор, который по Hermes CR запускает оригинальный Hermes для Telegram, сохраняет личное состояние и восстанавливает управляемые настройки при каждом старте.

**Architecture:** Go controller-runtime reconciler создаёт один StatefulSet, PVC, NetworkPolicy и согласованные revision inputs на инсталляцию. Стартовый Python adapter использует runtime официального образа, точечно применяет CR/Secrets к persistent home и exec-ом запускает исходный gateway. Helm устанавливает оператор cluster-wide; у агента нет Kubernetes credentials.

**Tech Stack:** Go ≥ 1.26.0; controller-runtime v0.25.1; k8s.io modules v0.37.0; controller-tools v0.22.0; Python из официального Hermes image; Helm; envtest; kind + Calico.

**Spec:** [v1alpha1](../../design/v1alpha1.md), [deployment](../../design/deployment.md); прочитать оба документа перед исполнением.

## Global Constraints

- API: `hermes.wbe7.github.io/v1alpha1`, namespaced kind `Hermes`, plural `hermes`, singular `hermes`, short name `hms`. Status — отдельный subresource. Scale subresource отсутствует.
- Один CR представляет одну инсталляцию: один процесс Hermes, один persistent home и один Telegram bot token.
- Используется оригинальный официальный образ. Контроллер и стартовый адаптер принадлежат оператору; исходники и установленные пакеты Hermes не патчатся.
- Пользователь вправе менять runtime config и персонализировать агента. CR/Secrets восстанавливают управляемые параметры при каждом старте контейнера.
- PVC монтируется целиком в `/opt/data`, одновременно `HOME` и `HERMES_HOME`. Workspace — `/opt/data/workspace`.
- Обязательные параметры: runAsNonRoot=true, UID/GID/fsGroup=10000, allowPrivilegeEscalation=false, capabilities.drop=[ALL], seccompProfile=RuntimeDefault, readOnlyRootFilesystem=true.
- Default deletion policy — Retain. Delete разрешён только для PVC, созданного оператором и идентифицированного по UID.
- Не входят в v1: web UI/HTTPRoute, другие мессенджеры, frontend/backend, backups/snapshots/restore, автоматическая миграция данных между PVC, HA одного агента, Kata/gVisor.
- Целевая матрица Kubernetes: 1.35, 1.36, 1.37. Проверенность каждого варианта отмечается только после его испытания.
- Не использовать существующий пользовательский кластер для испытаний. Все автоматические cluster tests создают явно названный временный кластер.

## Порядок, структура и границы

Задачи выполняются последовательно: **1 → 2 → 3 → 4 → 5 → 6 → 7 → 8**. Task 1 — обязательная проверка осуществимости. Если официальный образ не выполняет обязательный контракт без patch, сохранить воспроизводимое доказательство и вынести только это противоречие пользователю; не писать контроллер вокруг недоказанного обхода.

Исходное состояние: только design/research/docs; go.mod, CRD, runtime adapter, controller и chart отсутствуют. Примеры в docs/design/examples — проекты API, а не уже прошедшие schema validation manifests.

| Путь | Ответственность |
| --- | --- |
| `runtime/bootstrap.py`, `runtime/probe.py` | Entry points startup и exec probe в официальном контейнере. |
| `runtime/adapters/v20260914.py` | Изоляция version-specific upstream API/config precedence. |
| `runtime/tests/`, `test/runtime/` | Тесты merge/session/auth/health и smoke официального образа. |
| `api/v1alpha1/` | Go types, schema/defaults/CEL, status conditions. |
| `internal/runtimecatalog/` | Поддерживаемые releases/digests и mapping provider. |
| `internal/config/` | Валидация, Secret refs, config ownership, immutable revision bundle. |
| `internal/network/`, `internal/settings/` | Компиляция policy и проверка install-time network inputs. |
| `internal/workload/` | Pure builders StatefulSet, SA, Service, revision objects. |
| `internal/controller/` | Reconcile, watches, lifecycle PVC, rollout и status. |
| `cmd/manager/main.go` | Scheme, manager, leader election, health, wiring. |
| `config/crd/bases/`, `config/rbac/` | Сгенерированные schema и RBAC manifests. |
| `charts/hermes-operator/` | Установка, values schema, CRD install copy. |
| `test/e2e/`, `hack/`, `.github/workflows/` | Реальная проверка Kubernetes/CNI, воспроизводимые команды, CI/release. |
| `docs/reference/`, `docs/guides/` | Справочник реализованного API, install/operations и compatibility evidence. |

Новые небольшие helper-файлы допустимы внутри указанного модуля. Не объединять version adapter, config renderer, Kubernetes reconciliation и PVC deletion в один файл.

## Task 1: Официальный runtime, сохранность и Telegram gates

**Files**

- Create: `runtime/bootstrap.py`, `runtime/probe.py`, `runtime/adapters/__init__.py`, `runtime/adapters/v20260914.py`.
- Create: `runtime/tests/test_merge.py`, `runtime/tests/test_sessions.py`, `runtime/tests/test_telegram.py`, `runtime/tests/test_probe.py`.
- Create: `test/runtime/run.sh`, `test/runtime/fixtures/config.yaml`, `test/runtime/fixtures/input.json`.
- Create: `docs/research/runtime-verification.md`.
- Modify: `docs/research/startup-restore.md`, `docs/research/config-mapping.md` — заменить гипотезы только полученным доказательством.

**Interfaces**

```python
# bootstrap.py: pure merge + I/O boundary. Values must never appear in exceptions.
def merge_config(current: dict, desired: dict, previous_paths: set[tuple[str, ...]]) -> dict:
    """Preserve personal paths, set desired leaves, delete retired managed leaves."""

def restore(home: "pathlib.Path", bundle: dict, credentials: dict[str, str]) -> None:
    """Idempotent pre-exec restore; raises on error without clearing home."""

# adapters/v20260914.py
def reset_model_overrides(home: "pathlib.Path") -> None:
    """Clear active routing overrides, verify SQLite and mirror; retain session IDs."""

def ready(home: "pathlib.Path") -> bool:
    """Check current gateway identity, heartbeat, session store and Telegram."""

def live(home: "pathlib.Path") -> bool:
    """Check local event-loop health without probing a remote API."""
```

Bundle schema 1: `{schema:1, release:string, config:object, env:object, ownedPaths:array[array[string]], ownedEnv:array[string]}`. Config/env содержат несекретные значения и ссылки на именованные credential entries. Secret values поступают отдельными файлами, а не аргументами CLI.

- [ ] **1. Зафиксировать runtime image и environment.** Выполнить `docker version`, затем pull exact digest из spec. Записать platform, manifest digest и exit results. Docker daemon недоступен — диагностировать prerequisite; не выдавать source inspection за smoke.
- [ ] **2. Написать preservation tests.** Первый тест в test_merge.py:

```python
import unittest
from bootstrap import merge_config

class MergeTests(unittest.TestCase):
    def test_model_reset_preserves_personalization_and_removes_retired_key(self):
        current = {
            "model": {"default": "local-choice"},
            "agent": {"system_prompt": "Speak briefly"},
            "display": {"personality": "my-personality"},
            "compression": {"threshold": 0.5, "personal_note": "keep"},
        }
        desired = {"model": {"default": "cr-model"}}
        result = merge_config(current, desired, {("compression", "threshold")})
        self.assertEqual(result["model"]["default"], "cr-model")
        self.assertEqual(result["agent"]["system_prompt"], "Speak briefly")
        self.assertEqual(result["display"]["personality"], "my-personality")
        self.assertEqual(result["compression"], {"personal_note": "keep"})
        self.assertEqual(merge_config(result, desired, set()), result)
```

- [ ] **3. Подтвердить red.** В официальном image запустить `python -m unittest discover -s runtime/tests -v` с test-only PYTHONPATH runtime; ожидается отсутствие bootstrap implementation. Production launcher использует isolated imports и не наследует этот test-only PYTHONPATH.
- [ ] **4. Реализовать merge и startup transaction.** Обходить mapping по tuple paths; списки считать листьями; null удаляет leaf; scalar/object collision с пользовательскими потомками вызывает ошибку. До mutation записать pending ownership manifest; после проверки — committed manifest. На повторе учитывать union committed/pending/current paths, чтобы interrupted apply и последующая смена CR не оставили забытый managed key. Запись через tempfile в той же директории, fsync и os.replace.
- [ ] **5. Проверить upstream persistence.** Создать сессию штатным SessionStore, записать model_override, добавить историю. Вызвать reset_model_overrides и проверить прежний session ID, историю, primary gateway_routing и JSON mirror. Смоделировать SQLite failure: gateway не запускается и исходные данные не удалены.
- [ ] **6. Проверить альтернативные источники.** Fixtures должны содержать старые .env model endpoints/keys, gateway.json, reasoning_overrides, custom credential-pool запись и сохранённый профиль. После restore эффективный resolver использует CR model/provider/baseURL/key; SOUL, onboarding/profile preferences и сторонние credentials не удалены. Нельзя ограничиться сравнением YAML.
- [ ] **7. Проверить upstream Telegram handlers.** Создать synthetic Update fixtures с sender/chat IDs и text, slash command, media, callback. Вызывать реальные adapter/authz entrypoints выбранного release с fake transport. Считать число agent dispatch calls: разрешённый DM=1, чужой DM=0, запрещённая группа=0. Для enabled groups проверять одновременно sender/chat allowlists. Недокументированный marker допускается только после этой матрицы.
- [ ] **8. Реализовать non-root launcher и probe.** HOME/HERMES_HOME=/opt/data, cwd=/opt/data/workspace. Инициализация bundled skills через штатный helper; exec оригинального gateway без supervisor. Probe читает gateway/status.py и state/gateway.heartbeat: PID/start identity, текущий writer и local event-loop heartbeat; readiness дополнительно проверяет gateway/session/Telegram. Не использовать возраст gateway_state.json как единственный liveness: idle snapshot может не обновляться.
- [ ] **9. Выполнить контейнерный smoke дважды на одном home.** Run как UID/GID 10000, read-only root, cap-drop=ALL, no-new-privileges, /tmp tmpfs; scripts read-only. Сделать временный выбранный model/reasoning и personal fixture, завершить и повторно запустить штатный entrypoint. Проверить эффективную модель, данные и повторяемость restore. Доказательство same-Pod restart дополнительно требуется в Task 8.
- [ ] **10. Записать evidence и gate outcome.** В runtime-verification.md перечислить exact commands, версии, прошедшие tests и отсутствующие runtime доказательства. Реальный Telegram DM с выделенным ботом и provider request — отдельная проверка; synthetic transport не засчитывается как live Telegram. Невыполненные обязательные runtime пункты не дают release-ready статус.

**Review gate:** original image стартует non-root с нужными mounts, merge/session tests проходят, Telegram filtering проверен по всем handler paths. Нет monkey patch/install изменений upstream. Commit unit: `feat(runtime): restore declarative settings without losing user state`.

## Task 2: API types, schema и воспроизводимая сборка

**Files**

- Create: `go.mod`, `go.sum`, `Makefile`, `.gitignore`.
- Create: `api/v1alpha1/groupversion_info.go`, `api/v1alpha1/hermes_types.go`, `api/v1alpha1/zz_generated.deepcopy.go`.
- Create: `config/crd/bases/hermes.wbe7.github.io_hermes.yaml`.
- Create: `api/v1alpha1/schema_test.go`, `internal/testfixtures/hermes.go`, `internal/testfixtures/minimal.yaml`.
- Create: `internal/runtimecatalog/catalog.go`, `internal/runtimecatalog/catalog_test.go`.

**Interfaces**

- Module `github.com/wbe7/hermes-operator`.
- v1alpha1 exports `Hermes`, `HermesList`, `HermesSpec`, `HermesStatus`, `AddToScheme`. Spec fields exactly as in the specification; standard Kubernetes types for quantities/resources/affinity/condition.
- Shared fixture `testfixtures.Hermes(t testing.TB) *v1alpha1.Hermes` embeds minimal.yaml copied from docs/design/examples/hermes-minimal.yaml, unmarshals via sigs.k8s.io/yaml and sets a test UID. No actual credential values.
- `runtimecatalog.Release` fields: `Version string`, `ImageDigest string`, `Adapter string`, `UID int64`, `GID int64`.
- `runtimecatalog.Resolve(version string) (Release, error)`; only runtime-tested entries are supported.

- [ ] **1. Bootstrap Go module and tool targets.** Pin dependency versions from deployment contract. Make targets: `generate` (deepcopy), `manifests` (CRD/RBAC), `test-unit`, `test-envtest`, `test-runtime`, `verify-generated`. Pin controller-gen; no latest in reproducible scripts.
- [ ] **2. Write API-server validation cases before schema implementation.** In envtest create minimal valid CR, then variants with both storage sources, empty sender allowlist, cross-namespace ref field, Delete+existing, unsupported auth combination, group enable without IDs, IP exception containing CIDR, resource request above limit. Expect API Invalid errors for rules covered by structural schema/CEL. Unknown strict fields should be tested with fieldValidation=Strict.
- [ ] **3. Add types/defaults/CEL and bounded collections.** Secret refs contain name/key only; IDs are strings; storage sources immutable, size shrink forbidden. Optional pointers preserve nil/default semantics. extraConfig alone uses preserve-unknown-fields; do not make the whole spec schemaless.
- [ ] **4. Generate CRD/deepcopy; rerun tests.** Apply CRD to envtest, exercise create and update CEL. Verify kube API accepts delayed-binding CR without checking live PVC at admission. Semantic extraConfig/provider validation belongs to controller, not a new webhook.
- [ ] **5. Add release lookup regression test.**

```go
func TestUnknownReleaseIsRejected(t *testing.T) {
    if _, err := Resolve("latest"); err == nil {
        t.Fatal("mutable or unsupported release accepted")
    }
}
```

- [ ] **6. Verify generated copies and examples.** `make generate manifests test-unit test-envtest verify-generated`; validate all three design examples against installed CRD using server dry-run in the test cluster. Record only actual passing environments.

**Review gate:** schema rejects unsafe/ambiguous shapes, defaults are visible via kubectl explain, no webhook/cert-manager dependency. Commit unit: `feat(api): define the Hermes v1alpha1 contract`.

## Task 3: Конфигурация, Secret refs и revisions

**Files**

- Create: `internal/config/validate.go`, `internal/config/render.go`, `internal/config/secrets.go`, `internal/config/revision.go`.
- Create: `internal/config/render_test.go`, `internal/config/validate_test.go`, `internal/config/revision_test.go`.
- Create: `internal/config/testdata/personal-paths.yaml`.

**Interfaces**

```go
type Bundle struct {
    JSON       []byte
    SecretData map[string][]byte
    Revision   string
}
func SecretRefs(h *v1alpha1.Hermes) []corev1.SecretKeySelector
func Validate(h *v1alpha1.Hermes, release runtimecatalog.Release) error
func Render(h *v1alpha1.Hermes, release runtimecatalog.Release,
    secrets map[types.NamespacedName]*corev1.Secret) (Bundle, error)
```

SecretRefs возвращает разрешённые default имена и ключи, без чтения values. Render выдаёт schema 1 bundle Task 1; секретный map содержит только выбранные ключи под generated names. Revision включает нормализованные inputs и identity источников, исключает неиспользуемые ключи и resourceVersion.

- [ ] **1. Add negative tests.** Конфликт extraConfig=model, ancestor agent scalar, personal display.personality/system_prompt, hidden gateway auth override, duplicate env и credential literal. Ошибка указывает путь, не значение. Вставить sentinel secret и проверить его отсутствие в error/JSON.
- [ ] **2. Add default/explicit/no-auth tests.** Для maria refs должны разрешиться в maria-hermes-secret/TELEGRAM_BOT_TOKEN и MODEL_API_KEY; явный name/key перекрывает convention. auth=None custom не запрашивает MODEL_API_KEY. Reference namespace всегда h.Namespace.
- [ ] **3. Implement mapping and ownership.** Основные поля преобразовать по config-mapping.md; отсутствующие managed optional paths включить как удаления/defaults. Compile точный version-specific protected path/env set. Unknown safe extra leaves сохранить; missing Secret/key — typed dependency error без credentials.
- [ ] **4. Add deterministic revision tests.** Перестановка map keys, изменение annotations или неиспользуемого Secret key дают ту же revision; смена model, выбранного key, Secret UID, image/adapter revision — другую. ResourceVersion не входит в input.
- [ ] **5. Check complete renderer → bootstrap integration.** Render fixture в JSON/Secret files, вызвать restore из Task 1 в официальном image, проверить эффективный config resolver. Это тест совместимости формата, а не повторение ожиданий renderer.
- [ ] **6. Run `go test ./internal/config/... ./internal/runtimecatalog/...` и runtime tests.** Пустой config не может превратить personal YAML в upstream defaults tree.

**Review gate:** все основные поля имеют mapping, нет plaintext Secret в ConfigMap/status/error, retirement extra keys и альтернативные model sources покрыты тестом. Commit unit: `feat(config): render versioned startup inputs and secret references`.

## Task 4: NetworkPolicy compiler

**Files**

- Create: `internal/settings/network.go`, `internal/settings/network_test.go`.
- Create: `internal/network/policy.go`, `internal/network/special_ranges.go`, `internal/network/policy_test.go`.
- Create: `internal/network/testdata/iana-ipv4.csv`, `internal/network/testdata/iana-ipv6.csv`.

**Interfaces**

```go
// settings package: validated once at manager startup.
type DNSConfig struct {
    NamespaceLabels map[string]string
    PodLabels       map[string]string
    ResolverIPs     []string
}
type NetworkConfig struct {
    EnforcementConfirmed bool
    PodCIDRs              []string
    ServiceCIDRs          []string
    NodeCIDRs             []string
    InfrastructureCIDRs   []string
    DNS                   DNSConfig
}
func (c NetworkConfig) Validate() error

// network package: pure resource builder, no API calls.
func Build(h *v1alpha1.Hermes, cluster settings.NetworkConfig) (*networkingv1.NetworkPolicy, error)
```

- [ ] **1. Snapshot IANA source tables.** Save exact fetched tables with retrieval date/source in source comments. Classify private/non-global/transition ranges and carve explicitly global special-purpose exceptions; no runtime HTTP fetching from reconciler.
- [ ] **2. Write behavioral address matrix.** Evaluate generated egress rules with a test-only policy matcher for public IP, RFC1918, CGNAT, metadata/link-local, cluster networks, ULA, NAT64, global IPv6, IPv4-mapped IPv6; exact allowed IP+port passes and its neighbor fails. Test globally reachable IANA exceptions are not accidentally denied by aggregate ranges.
- [ ] **3. Implement compiler using net/netip.** Normalize prefixes, remove overlaps where required, split by IP family. Public ipBlock except contains only same-family strict subnets. DNS selectors include both namespace and Pod in the same peer. Exact resolver rules contain only port 53 TCP/UDP.
- [ ] **4. Test input validation.** Reject unspecified/loopback/multicast exception IP, CIDR in single-IP field, invalid port/protocol and empty DNS selection. Explicit private IP exception wins over private deny for that endpoint only.
- [ ] **5. Run `go test ./internal/network/... ./internal/settings/...`.** Check policyTypes contains both Ingress/Egress, ingress is empty, selected labels use immutable CR UID, public egress does not degrade to unrestricted 0.0.0.0/0 + ::/0.

**Review gate:** pure compiler behavior verified; this is not CNI enforcement proof, which belongs to Task 8. Commit unit: `feat(network): isolate agent egress with explicit IP exceptions`.

## Task 5: Workload и startup inputs

**Files**

- Create: `internal/workload/build.go`, `internal/workload/revisions.go`, `internal/workload/bootstrap_assets.go`.
- Create: `internal/workload/build_test.go`, `internal/workload/revisions_test.go`.
- Create: `internal/workload/assets/` — generated copies of runtime scripts with source checksum; Makefile verifies equality.

**Interfaces**

```go
type Resources struct {
    StatefulSet    *appsv1.StatefulSet
    Service        *corev1.Service
    ServiceAccount *corev1.ServiceAccount
    ConfigMap      *corev1.ConfigMap
    Secret         *corev1.Secret
    Bootstrap      *corev1.ConfigMap
}
func Build(h *v1alpha1.Hermes, release runtimecatalog.Release,
    bundle config.Bundle, claimName string) (Resources, error)
```

- [ ] **1. Write hardening tests.** Assert UID/GID/fsGroup, non-root, read-only root, no privilege escalation, ALL dropped, RuntimeDefault seccomp, false SA automount in both places, no host mounts/network/ports, no Service web endpoint.
- [ ] **2. Write restart/volume tests.** Main command invokes startup script before gateway; initContainer alone must fail the test. HOME/HERMES_HOME use full PVC mount; workspace under home. Input config/credentials/scripts read-only, /tmp bounded, grace=60, replicas=1 or 0 when suspended.
- [ ] **3. Build immutable revision resources and StatefulSet.** Both input names carry the same revision; scripts carry operator/adapter digest. Default resource keys merged individually, image is repository@digest. Headless service has no public LoadBalancer/NodePort route.
- [ ] **4. Add probes from Task 1.** startup verifies completed bootstrap/local process; readiness also Telegram/session health; liveness only local event loop. Preserve slow startup budget without restart loops for transient provider/Telegram outage.
- [ ] **5. Run `go test ./internal/workload/...` and dry-run manifests in envtest.** Verify owner references use current CR UID; ServiceAccount gets no RoleBinding. Unreferenced revision cleanup is deferred to controller lifecycle, never to builder.

**Review gate:** actual rendered Pod matches accepted security contract and invokes restore on every container start. Commit unit: `feat(workload): build isolated single-instance Hermes workloads`.

## Task 6: Controller lifecycle и PVC

**Files**

- Create: `cmd/manager/main.go`.
- Create: `internal/controller/hermes_controller.go`, `internal/controller/storage.go`, `internal/controller/dependencies.go`, `internal/controller/status.go`, `internal/controller/revisions.go`.
- Create: `internal/controller/hermes_controller_test.go`, `internal/controller/storage_test.go`, `internal/controller/rotation_test.go`.
- Create: `config/rbac/role.yaml`, `config/rbac/leader-election-role.yaml`.

**Interfaces**

```go
type HermesReconciler struct {
    client.Client
    APIReader client.Reader
    Scheme    *runtime.Scheme
    Network   settings.NetworkConfig
}
func (r *HermesReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
func (r *HermesReconciler) SetupWithManager(mgr ctrl.Manager) error
```

PVC lifecycle — внутренний модуль контроллера, не универсальный storage framework. Все deletes с UID preconditions; ownership checks обязательны до patch/update/delete.

- [ ] **1. Add envtest dependency lifecycle test.** Create CR without Secret: Conditions show DependencyNotFound, no active workload. Add Secret: reconcile creates policy before workload. Fake client alone недостаточен для ownerReferences/resourceVersion/status конфликтов.
- [ ] **2. Implement read/validate/build/apply sequence.** Finalizer записывается до создания ресурсов; все desired inputs валидируются до частичного apply. Чужие resources дают ResourceConflict. APIReader получает current referenced Secret contents; metadata cache и index mapping ставят только affected CR в очередь.
- [ ] **3. Add storage lifecycle tests.** Retain with deleted CR leaves PVC; Delete waits until Pod stopped; existing never mutated/deleted; same-name replacement claim with different UID never deleted; new CR UID cannot auto-adopt retained claim. Two CRs referencing same existing PVC are detected.
- [ ] **4. Implement PVC creation/resize/finalizer.** Create without GC ownerReference; provenance UID annotation; status records claim UID. Delayed binding не ждёт Bound до workload. Expansion изменяет только size; unsupported resize оставляет данные и reason, shrink не проходит validation.
- [ ] **5. Add Secret rotation test.** Unused key/metadata change does not replace Pod template; selected key/UID change produces revision. Source Secret deletion stops workload but preserves home; restored Secret resumes. Suspend and finalizer operate without credentials.
- [ ] **6. Implement status and GC.** observedGeneration advances on considered spec; Ready requires current revision+PodReady; metadata-only reconciliation is idempotent. Revision objects retained while referenced by current Pod/StatefulSet. Log/error paths never dump config/Secret values.
- [ ] **7. Add rollout recovery scenarios.** Invalid spec leaves last valid workload with Ready=false for new generation. Returning to valid spec recovers stuck StatefulSet rollout using ordinary old-Pod delete with UID precondition. Never force delete or create surge replicas.
- [ ] **8. Verify manager restart/leader election.** Reconstruct state from CR/resources/PVC annotations; no indispensable in-memory ownership registry. Inject conflict/transient API errors; bounded backoff and recovery without duplicate claims.
- [ ] **9. Run `make test-unit test-envtest manifests verify-generated`.** Review generated ClusterRole has no cluster-admin, RBAC mutation, Pod exec, Nodes mutation or wildcards.

**Review gate:** lifecycle invariants hold on API server, no unbounded rollout loop, no unsafe ownership adoption. Commit unit: `feat(controller): reconcile Hermes lifecycle and persistent storage`.

## Task 7: Helm, документация и CI

**Files**

- Create: `Dockerfile` — operator image only, not a Hermes fork.
- Create: `charts/hermes-operator/Chart.yaml`, `values.yaml`, `values.schema.json`, `templates/deployment.yaml`, `templates/rbac.yaml`, `templates/settings.yaml`, `crds/hermes.wbe7.github.io_hermes.yaml` within chart directory.
- Create: `docs/reference/hermes-v1alpha1.md`, `docs/guides/install.md`, `docs/guides/operations.md`, `docs/guides/security.md`, `docs/guides/personalization.md`.
- Create: `examples/` (three validated Hermes examples, Secret template, namespace/admin RBAC).
- Create: `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `hack/verify-generated.sh`.
- Modify: `README.md`, `Makefile`.

**Interfaces**

- Chart values and network configuration exactly as deployment.md; CRD artifact is generated from Go API, chart copy checked byte-for-byte.
- Stable developer commands: `make test-unit test-envtest test-runtime lint-chart verify-generated`.
- Release artifacts: operator image, chart archive, CRD YAML, checksums and compatibility report. Publishing credentials are CI secrets, not repository files.

- [ ] **1. Write chart rendering cases.** Missing network confirmation/CIDRs/DNS fails schema/template validation; valid installation renders controller securityContext/RBAC and no public Service. Explicit empty infrastructureCIDRs succeeds. Include normal IPv4 and dual-stack inputs.
- [ ] **2. Implement templates and values schema.** Controller requires network config at startup; chart uses non-root operator image, leader election, no tenant namespace mutation. CRD included for initial install and documented separately for upgrade.
- [ ] **3. Create hermetic generated-file check.** Regenerate CRD/deepcopy/runtime script assets into a temporary directory and compare with committed files. Fail on divergence; no code-generation dirty diff silently accepted in CI.
- [ ] **4. Promote design examples to validated examples.** Add conventional Secret template with unmistakable non-secret sample values. Documentation explains replacement and same-namespace refs; never put real bot/model credentials in fixtures.
- [ ] **5. Write user guides from implemented behavior.** Include every field/default/mapping, model/Secret rotation, no-auth local inference with IP exception, existing PVC permissions, Retain/Delete, delayed binding/resize, failed rollout, personalization, mutable config restoration and data loss boundaries.
- [ ] **6. Configure CI jobs.** Unit/envtest/runtime/Helm/static checks on PR; integration job uses only temporary cluster. Privileged release credentials unavailable to untrusted PR code. Build multiarch operator image, scan dependencies/image and produce SBOM; release publish only on explicitly configured version tag.
- [ ] **7. Run chart lint/template and docs/links/generated checks.** CI success means these commands actually passed. Do not call unexecuted chart/e2e tests successful based on rendered YAML.

**Review gate:** user can follow installation and operations docs on a clean cluster, generated artifacts match source, no hidden manual configuration beyond documented prerequisites. Commit unit: `build: package the operator and document supported workflows`.

## Task 8: End-to-end acceptance и release evidence

**Files**

- Create: `test/e2e/lifecycle_test.go`, `test/e2e/network_test.go`, `test/e2e/persistence_test.go`, `test/e2e/telegram_test.go`.
- Create: `hack/e2e-cluster.sh`, `test/e2e/fixtures/network-targets.yaml`, `test/e2e/fixtures/operator-values.yaml`.
- Create: `docs/reference/compatibility.md`, `docs/research/v1-acceptance.md`.
- Modify: `Makefile`, `.github/workflows/ci.yml`.

**Interfaces**

- `make test-e2e` creates/deletes a temporary `hermes-operator-e2e` kind cluster; refuses unrelated active kube contexts for mutation.
- `make test-e2e-live` requires dedicated test Telegram/provider inputs through environment/temporary Secret files, never CLI literals or logs.
- Acceptance IDs A01–A12 from specification are stable report keys with passed/failed/not-run and evidence paths.

- [ ] **1. Bring up reference CNI and controlled endpoints.** kind default networking disabled, Calico installed from pinned release artifact. Pin compatible image/chart versions and their checksums when building the executable harness. A control Pod demonstrates endpoint reachability before restrictive policies.
- [ ] **2. Run two-namespace install and lifecycle.** CR create → ready → suspend/resume → config/Secret update → controller restart → retention/deletion/existing claim. Validate actual Pods/PVC UIDs, not only desired objects.
- [ ] **3. Force an ordinary app-container restart in the same Pod.** Record Pod UID, container restartCount, session ID and fixture checksums; terminate gateway normally and let kubelet restart it. Pod UID must stay equal, restartCount increase, CR model restored, personal state unchanged.
- [ ] **4. Replace Pod and repeat persistence.** Persist SOUL, personality, user prompt, skill, memory, cron, workspace file, conversation and local model/provider/reasoning change. After replacement validate both actual model resolver/request and all retained state.
- [ ] **5. Run real network probes.** DNS TCP/UDP, reachable public destinations, blocked private/service/node/metadata stand-ins, one allowed IP+port and denied neighbor. Repeat native dual-stack; report CNI/NAT/hosting-node limitations truthfully. Unit policy matcher does not count here.
- [ ] **6. Run live Telegram flow with dedicated identities.** Allowed DM and first-contact personalization; unauthorized sender/group and opt-in authorized group if included. Real provider receives expected model. Use test clients/fixtures under explicit test credentials; no unsolicited messages to real users.
- [ ] **7. Verify non-happy paths.** Missing/deleted Secret, corrupt YAML/SQLite, PVC permissions, Pending/resize, unsupported image version, provider outage and Telegram token conflict. No destructive fallback reset; provider/Telegram outage does not cause liveness restart storm.
- [ ] **8. Verify install/upgrade/uninstall.** Explicit CRD upgrade before Helm upgrade, active CR survives operator upgrade, retained claims remain after normal CR cleanup/uninstall. No automatic data-schema rollback claim.
- [ ] **9. Complete coverage table and release review.** Every A01–A12 has command/run evidence. Smoke both supported architectures and target Kubernetes minors; remove untested rows from claimed support. Runtime gate failures stay blockers, optional enabled-group failure may remove opt-in group support while preserving required DM-only.

**Review gate:** all mandatory acceptance scenarios passed, test secrets absent from logs/artifacts, docs reflect actual tested compatibility. Commit unit: `test(e2e): verify isolation persistence and lifecycle`.

## Покрытие спецификации

| Требование | Task | Приёмка |
| --- | --- | --- |
| Original image, non-root, no patch | 1, 5 | A09 |
| CR fields, default Secret, provider/reasoning, extra config | 2, 3 | A01, A04, A06, A08 |
| Telegram authorization и персонализация | 1, 8 | A02, A03 |
| Full home, startup restore, session model override | 1, 5, 8 | A04, A05 |
| PVC retention, explicit adoption, resize | 6, 8 | A07, A11 |
| Internet/private IP policy и DNS | 4, 8 | A09 |
| Reconcile, revisions, missing dependencies, suspend/status | 5, 6 | A08, A10 |
| Cluster-wide RBAC, Helm, docs, release | 7, 8 | A01, A12 |

## Состояние исполнения

На момент создания плана ни одна задача реализации не выполнена. Проверены согласованность проектных документов и первичные источники, но не runtime продукта. Чекбоксы обновляются только после исполнения соответствующего шага; publication в GitHub Issues/релиз и deployment не подменяются локальным созданием этого плана.
