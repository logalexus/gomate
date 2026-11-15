FROM python:3.12-slim-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends bash curl binutils ca-certificates && rm -rf /var/lib/apt/lists/*
COPY deploy/runner/requirements.txt /tmp/requirements.txt
# Scripts use Python bindings and shared libraries; the Unicorn static archive
# is only needed to link a separate native application. Clean in the install
# layer so the removed files never contribute to the final image size.
RUN pip install --no-cache-dir --no-compile -r /tmp/requirements.txt \
    && rm /tmp/requirements.txt \
        /usr/local/lib/python3.12/site-packages/unicorn/lib/libunicorn.a \
    && find /usr/local/lib/python3.12/site-packages -type f \
        \( -name '*.so' -o -name '*.so.*' \) -exec strip --strip-unneeded {} +
ENV HOME=/tmp TERM=xterm PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PWNLIB_NOTERM=1
WORKDIR /tmp
USER 65532:65532
CMD ["python3", "--version"]
