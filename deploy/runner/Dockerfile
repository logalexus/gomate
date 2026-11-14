FROM python:3.12-slim-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends bash curl binutils ca-certificates && rm -rf /var/lib/apt/lists/*
COPY deploy/runner/requirements.txt /tmp/requirements.txt
RUN pip install --no-cache-dir -r /tmp/requirements.txt && rm /tmp/requirements.txt
ENV HOME=/tmp TERM=xterm PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PWNLIB_NOTERM=1
WORKDIR /tmp
USER 65532:65532
CMD ["python3", "--version"]
