FROM docker.io/nousresearch/hermes-agent:v2026.9.14@sha256:99641e57ec762c59e54cb44aa6746b7fc68c18b3c5ddb088af54234c613d9294

USER root
LABEL org.opencontainers.image.title="Hermes document and browser runtime" \
      org.opencontainers.image.version="v2026.9.14" \
      org.opencontainers.image.source="https://github.com/wbe7/hermes-operator" \
      io.wbe7.hermes.upstream.digest="sha256:99641e57ec762c59e54cb44aa6746b7fc68c18b3c5ddb088af54234c613d9294"

COPY requirements.lock /opt/hermes-tools/requirements.lock
COPY browser-use.lock /opt/hermes-tools/browser-use.lock
# Constrain every existing Python dependency: tools must not silently upgrade Hermes.
RUN uv pip freeze --python /opt/hermes/.venv/bin/python | sed '/^-e /d' > /tmp/upstream-constraints.txt && \
    uv --no-config pip install --python /opt/hermes/.venv/bin/python --require-hashes \
      --constraint /tmp/upstream-constraints.txt -r /opt/hermes-tools/requirements.lock && \
    rm /tmp/upstream-constraints.txt

RUN uv --no-config venv --python /opt/hermes/.venv/bin/python /opt/hermes-tools/browser-use && \
    uv --no-config pip install --python /opt/hermes-tools/browser-use/bin/python --require-hashes \
      -r /opt/hermes-tools/browser-use.lock && \
    ln -s /opt/hermes-tools/browser-use/bin/browser-use /usr/local/bin/browser-use

RUN apt-get -o Acquire::Retries=3 update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      libreoffice-writer libreoffice-calc libreoffice-impress python3-uno pandoc \
      poppler-utils qpdf ghostscript \
      tesseract-ocr tesseract-ocr-eng tesseract-ocr-rus tesseract-ocr-osd ocrmypdf \
      fonts-dejavu-core fonts-liberation fonts-noto-core fonts-noto-color-emoji \
      jq zip unzip 7zip file && \
    /opt/hermes/.venv/bin/python -m playwright install --with-deps chromium && \
    /opt/hermes/.venv/bin/python -c 'from playwright.sync_api import sync_playwright; import os; p=sync_playwright().start(); os.symlink(p.chromium.executable_path, "/usr/local/bin/chromium"); p.stop()' && \
    ln -s /usr/local/bin/chromium /usr/local/bin/google-chrome && \
    rm -rf /var/lib/apt/lists/* /root/.cache && \
    fc-cache -f

# Hermes can discover the CLI without npx/network access, and CDP uses the same
# full Chromium that was installed for the pinned Python Playwright above.
COPY package.json package-lock.json /opt/hermes-tools/node/
RUN cd /opt/hermes-tools/node && npm ci --ignore-scripts --omit=dev --no-audit --no-fund && \
    chmod 0755 node_modules/agent-browser/bin/agent-browser-linux-* && \
    ln -s /opt/hermes-tools/node/node_modules/.bin/agent-browser /usr/local/bin/agent-browser && \
    npm cache clean --force && \
    mkdir -p /opt/hermes-tools && \
    dpkg-query -W -f='${Package}\t${Version}\n' > /opt/hermes-tools/debian-packages.tsv && \
    uv pip freeze --python /opt/hermes/.venv/bin/python > /opt/hermes-tools/python-packages.txt

ENV AGENT_BROWSER_EXECUTABLE_PATH=/usr/local/bin/chromium \
    AGENT_BROWSER_ARGS="--no-sandbox,--disable-dev-shm-usage" \
    PATH="/opt/hermes-tools/node/node_modules/.bin:${PATH}" \
    HOME=/opt/data \
    HERMES_HOME=/opt/data
COPY smoke.py /opt/hermes-tools/smoke.py
COPY --chmod=0755 recalculate.py /usr/local/bin/hermes-recalculate
USER 10000:10000
WORKDIR /opt/data
ENTRYPOINT ["/opt/hermes/.venv/bin/hermes"]
CMD ["gateway", "run"]
