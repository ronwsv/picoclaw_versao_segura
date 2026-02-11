# ============================================
# PicoClaw - Dockerfile com isolamento de seguranca
# ============================================

# --- Stage 1: Build ---
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o /picoclaw ./cmd/picoclaw

# --- Stage 2: Runtime minimo ---
FROM alpine:3.21

# Instalar ferramentas para o agente poder trabalhar
RUN apk add --no-cache ca-certificates bash curl python3 py3-pip dos2unix git jq && \
    pip3 install --break-system-packages requests beautifulsoup4

# Criar usuario nao-root dedicado
RUN addgroup -S picoclaw && \
    adduser -S -G picoclaw -h /home/picoclaw -s /bin/bash picoclaw

# Criar estrutura de diretorios
RUN mkdir -p /home/picoclaw/.picoclaw/workspace/memory \
             /home/picoclaw/.picoclaw/workspace/skills \
             /home/picoclaw/.picoclaw/sessions \
             /sandbox && \
    chown -R picoclaw:picoclaw /home/picoclaw /sandbox

# Copiar binario
COPY --from=builder /picoclaw /usr/local/bin/picoclaw
RUN chmod +x /usr/local/bin/picoclaw

# Copiar skills built-in
COPY --chown=picoclaw:picoclaw skills/ /home/picoclaw/.picoclaw/workspace/skills/

# Copiar templates de personalidade (Frostfree) para workspace e backup
COPY --chown=picoclaw:picoclaw workspace-templates/SOUL.md /home/picoclaw/.picoclaw/workspace/SOUL.md
COPY --chown=picoclaw:picoclaw workspace-templates/AGENTS.md /home/picoclaw/.picoclaw/workspace/AGENTS.md
COPY --chown=picoclaw:picoclaw workspace-templates/USER.md /home/picoclaw/.picoclaw/workspace/USER.md
COPY --chown=picoclaw:picoclaw workspace-templates/IDENTITY.md /home/picoclaw/.picoclaw/workspace/IDENTITY.md
COPY --chown=picoclaw:picoclaw workspace-templates/MEMORY.md /home/picoclaw/.picoclaw/workspace/memory/MEMORY.md
# Backup dos templates (entrypoint copia para workspace se nao existirem)
COPY --chown=picoclaw:picoclaw workspace-templates/ /opt/picoclaw-templates/

# Copiar config example como base
COPY --chown=picoclaw:picoclaw config.example.json /home/picoclaw/.picoclaw/config.json

# Copiar entrypoint e converter line endings Windows -> Linux
COPY --chown=picoclaw:picoclaw docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN dos2unix /usr/local/bin/docker-entrypoint.sh && chmod +x /usr/local/bin/docker-entrypoint.sh

# Rodar onboard para inicializar workspace com templates
USER picoclaw
RUN picoclaw onboard 2>/dev/null || true

WORKDIR /sandbox
ENV HOME=/home/picoclaw

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["agent"]
