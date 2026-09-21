# Образ агента с браузером и документными инструментами

Оператор использует `docker.io/wbe7/hermes:v2026.9.14` для новых инсталляций.
Один тег содержит `linux/amd64` и `linux/arm64`. Он совпадает с версией Hermes;
workload запускается по проверенному digest из `internal/runtimecatalog/catalog.go`,
поэтому перемещение тега не обновляет работающих агентов само по себе.

Образ наследует официальный `nousresearch/hermes-agent` по закреплённому digest.
Исходный код Hermes не изменяется. Системные пакеты устанавливаются при сборке,
агент работает с UID/GID 10000, read-only root filesystem и writable PVC `/opt/data`.
Все установленные в образ инструменты доступны после пересоздания Pod без повторного
скачивания. Пользовательские файлы, память, skills и браузерные данные в home остаются на PVC.

## Состав

| Задача | Инструменты |
| --- | --- |
| Браузер и CDP | Полный Chromium из Playwright, Python Playwright, `agent-browser`, `browser-use`, системные зависимости |
| DOCX и шаблоны | `python-docx`, `docxtpl`, LibreOffice Writer |
| XLSX, CSV и расчёты | `openpyxl`, XlsxWriter, pandas, NumPy, LibreOffice Calc |
| PPTX | `python-pptx`, LibreOffice Impress |
| Конвертация документов | Pandoc; LibreOffice headless для офисных файлов и экспорта в PDF |
| PDF | pypdf, pdfplumber, ReportLab, pdf2image, Poppler (`pdftotext`, `pdftoppm`), qpdf, Ghostscript |
| Сканированные документы | Tesseract, русский/английский, определение ориентации страниц; OCRmyPDF |
| Изображения и графики | Pillow, Matplotlib; шрифты DejaVu, Liberation, Noto и emoji |
| Файлы и архивы | `file`, `jq`, zip/unzip, 7zip; исходные git, curl, ripgrep, ffmpeg, Node/npm, uv |

`openpyxl` и XlsxWriter записывают формулы; пересчёт выполняет LibreOffice Calc.
Для явного пересчёта всех ячеек и сохранения cached values используйте
`hermes-recalculate input.xlsx output.xlsx`. Команда создаёт новый файл, отключает
макросы и обновление внешних ссылок; существующий output не перезаписывается.
Макросы Microsoft Office не являются поддерживаемым сценарием. Совпадение сложной
вёрстки с Microsoft Office зависит от документа и доступных шрифтов: экспорт нужно
проверять визуально. Pandoc установлен без TeX Live; PDF создаётся через LibreOffice,
Chromium или ReportLab, а не неявный LaTeX engine Pandoc.

## Запуск браузера

Один лишь headless-shell из официального образа не гарантирует обнаружение браузера
в `browser_exec`. Наш образ заранее устанавливает оба CLI и задаёт:

```text
AGENT_BROWSER_EXECUTABLE_PATH=/usr/local/bin/chromium
AGENT_BROWSER_ARGS=--no-sandbox,--disable-dev-shm-usage
```

CLI `agent-browser` находится в системном PATH перед пользовательскими каталогами.
Поиск браузера не зависит от `/.dockerenv`, cgroup или runtime-установок через npx.
`browser-use` имеет отдельное закреплённое Python-окружение: его SDK-зависимости не
заменяют библиотеки основного Hermes. Документные библиотеки устанавливаются с
constraints всех исходных Python-пакетов Hermes; несовместимость останавливает сборку.

Внутренняя песочница Chromium отключена явно. Это не отключает Kubernetes seccomp,
non-root, `drop: ALL`, read-only root filesystem или NetworkPolicy. Пользовательский
код и браузер находятся в одной контейнерной границе; отдельной изоляции браузера от
файлов/credentials своей инсталляции этот образ не обещает.

## Сборка и проверки

```bash
make build-hermes-image
make test-hermes-image
HERMES_RUNTIME_IMAGE=wbe7/hermes:v2026.9.14 make test-runtime
```

Для обеих платформ:

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  --tag wbe7/hermes:v2026.9.14 --file images/hermes/Dockerfile images/hermes
```

Для локальной загрузки multiarch manifest добавьте `--load` при использовании
containerd image store. Для публикации добавьте `--push` после проверки обеих платформ.
Build context ограничен `images/hermes` и allowlist в `.dockerignore`; `.env` туда не попадает.

`smoke.py` проверяет реальную навигацию Playwright и agent-browser, вызов upstream
`browser_exec`, скриншот/PDF, DOCX-шаблон, XLSX с пересчётом `12 + 30 = 42`, PPTX → PDF,
PDF extraction/merge/raster, OCR русского/английского скана, график и архив.
Проверки выполняются non-root с read-only root, `drop: ALL`, запретом повышения
привилегий и отключённой внешней сетью. HTTP-страница для теста обслуживается локально.

Python dependencies и хеши записаны в `requirements.lock` и `browser-use.lock`;
npm integrity — в `package-lock.json`. Версии Debian и основного Python-окружения
записаны внутри образа в `/opt/hermes-tools/`. Debian repositories обновляются:
побайтовая воспроизводимость повторной сборки из одного Dockerfile не гарантируется.
Для запуска используется проверенный immutable digest.

## CI и публикация

Workflow `Hermes image` собирает и проверяет каждую архитектуру на отдельном native
runner. PR не публикуют образы. Для ручного `workflow_dispatch` с `publish=true`
нужны repository secrets `DOCKERHUB_USERNAME` и `DOCKERHUB_TOKEN`, разрешающие push
только необходимых repositories. Финальный job использует environment `release`.
Значения секретов не помещаются в Git или build args.

Публикуются проверенные платформенные образы, затем из их digest собирается общий
тег `wbe7/hermes:v2026.9.14`. Повторной сборки между проверкой и публикацией нет.
После публикации новый digest регистрируется в каталоге оператора, проверяется
startup adapter и только затем используется в CR. Тег обновляется при пересборке,
но уже зарегистрированный digest продолжает определять работающие инсталляции.

## Существующие инсталляции

CR с явно указанным `image.repository: docker.io/nousresearch/hermes-agent`
продолжает выбирать исходный официальный digest. Чтобы перейти на расширенный образ,
укажите `docker.io/wbe7/hermes` и удалите старый `image.digest`, если он был указан.
Для зеркала можно задать repository и один из зарегистрированных digest явно.
Произвольные непроверенные digest отклоняются. Смена образа перезапускает Pod;
состояние на PVC сохраняется.
