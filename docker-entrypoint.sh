#!/bin/bash
# ============================================
# PicoClaw Docker Entrypoint
# Injeta API keys das env vars no config.json
# ============================================

CONFIG_FILE="$HOME/.picoclaw/config.json"

# Garante que o config existe
if [ ! -f "$CONFIG_FILE" ]; then
    picoclaw onboard 2>/dev/null || true
fi

# Garante que o config existe (fallback)
if [ ! -f "$CONFIG_FILE" ]; then
    mkdir -p "$(dirname "$CONFIG_FILE")"
    echo '{}' > "$CONFIG_FILE"
fi

# Injeta API keys das env vars no config.json usando bash puro
# Le o config atual
CONFIG=$(cat "$CONFIG_FILE")

# Funcao para atualizar campo JSON (simples, sem jq)
update_provider_key() {
    local provider="$1"
    local key_var="$2"
    local base_var="$3"
    local key_val="${!key_var}"
    local base_val="${!base_var}"

    if [ -n "$key_val" ]; then
        # Usa python se disponivel, senao sed
        if command -v python3 &>/dev/null; then
            CONFIG=$(echo "$CONFIG" | python3 -c "
import json, sys
cfg = json.load(sys.stdin)
if 'providers' not in cfg: cfg['providers'] = {}
if '$provider' not in cfg['providers']: cfg['providers']['$provider'] = {}
cfg['providers']['$provider']['api_key'] = '$key_val'
if '$base_val': cfg['providers']['$provider']['api_base'] = '$base_val'
json.dump(cfg, sys.stdout, indent=2)
")
        fi
    fi
}

# Tenta usar python3 para manipular JSON de forma segura
if command -v python3 &>/dev/null; then
    python3 << 'PYEOF'
import json, os

config_file = os.path.expanduser("~/.picoclaw/config.json")

with open(config_file) as f:
    cfg = json.load(f)

# Mapear env vars para providers
providers_map = {
    "openai": ("PICOCLAW_PROVIDERS_OPENAI_API_KEY", "PICOCLAW_PROVIDERS_OPENAI_API_BASE"),
    "anthropic": ("PICOCLAW_PROVIDERS_ANTHROPIC_API_KEY", "PICOCLAW_PROVIDERS_ANTHROPIC_API_BASE"),
    "openrouter": ("PICOCLAW_PROVIDERS_OPENROUTER_API_KEY", "PICOCLAW_PROVIDERS_OPENROUTER_API_BASE"),
    "groq": ("PICOCLAW_PROVIDERS_GROQ_API_KEY", "PICOCLAW_PROVIDERS_GROQ_API_BASE"),
    "zhipu": ("PICOCLAW_PROVIDERS_ZHIPU_API_KEY", "PICOCLAW_PROVIDERS_ZHIPU_API_BASE"),
    "gemini": ("PICOCLAW_PROVIDERS_GEMINI_API_KEY", "PICOCLAW_PROVIDERS_GEMINI_API_BASE"),
    "vllm": ("PICOCLAW_PROVIDERS_VLLM_API_KEY", "PICOCLAW_PROVIDERS_VLLM_API_BASE"),
}

if "providers" not in cfg:
    cfg["providers"] = {}

for provider, (key_env, base_env) in providers_map.items():
    key_val = os.environ.get(key_env, "")
    base_val = os.environ.get(base_env, "")
    if key_val:
        if provider not in cfg["providers"]:
            cfg["providers"][provider] = {}
        cfg["providers"][provider]["api_key"] = key_val
        if base_val:
            cfg["providers"][provider]["api_base"] = base_val

# Atualizar modelo e agent defaults
model = os.environ.get("PICOCLAW_AGENTS_DEFAULTS_MODEL", "")
if model:
    if "agents" not in cfg:
        cfg["agents"] = {}
    if "defaults" not in cfg["agents"]:
        cfg["agents"]["defaults"] = {}
    cfg["agents"]["defaults"]["model"] = model

max_tokens = os.environ.get("PICOCLAW_AGENTS_DEFAULTS_MAX_TOKENS", "")
if max_tokens:
    cfg["agents"]["defaults"]["max_tokens"] = int(max_tokens)

temp = os.environ.get("PICOCLAW_AGENTS_DEFAULTS_TEMPERATURE", "")
if temp:
    cfg["agents"]["defaults"]["temperature"] = float(temp)

max_iter = os.environ.get("PICOCLAW_AGENTS_DEFAULTS_MAX_TOOL_ITERATIONS", "")
if max_iter:
    cfg["agents"]["defaults"]["max_tool_iterations"] = int(max_iter)

workspace = os.environ.get("PICOCLAW_AGENTS_DEFAULTS_WORKSPACE", "")
if workspace:
    cfg["agents"]["defaults"]["workspace"] = workspace

# Canais - Telegram
if "channels" not in cfg:
    cfg["channels"] = {}

tg_enabled = os.environ.get("PICOCLAW_CHANNELS_TELEGRAM_ENABLED", "").lower()
if tg_enabled == "true":
    if "telegram" not in cfg["channels"]:
        cfg["channels"]["telegram"] = {}
    cfg["channels"]["telegram"]["enabled"] = True
    tg_token = os.environ.get("PICOCLAW_CHANNELS_TELEGRAM_TOKEN", "")
    if tg_token:
        cfg["channels"]["telegram"]["token"] = tg_token
    tg_allow = os.environ.get("PICOCLAW_CHANNELS_TELEGRAM_ALLOW_FROM", "")
    if tg_allow:
        cfg["channels"]["telegram"]["allow_from"] = [x.strip() for x in tg_allow.split(",")]
    else:
        cfg["channels"]["telegram"]["allow_from"] = []

# Canais - Discord
dc_enabled = os.environ.get("PICOCLAW_CHANNELS_DISCORD_ENABLED", "").lower()
if dc_enabled == "true":
    if "discord" not in cfg["channels"]:
        cfg["channels"]["discord"] = {}
    cfg["channels"]["discord"]["enabled"] = True
    dc_token = os.environ.get("PICOCLAW_CHANNELS_DISCORD_TOKEN", "")
    if dc_token:
        cfg["channels"]["discord"]["token"] = dc_token

# Tools - Web Search
web_search_key = os.environ.get("PICOCLAW_TOOLS_WEB_SEARCH_API_KEY", "")
if web_search_key:
    if "tools" not in cfg:
        cfg["tools"] = {}
    if "web" not in cfg["tools"]:
        cfg["tools"]["web"] = {}
    if "search" not in cfg["tools"]["web"]:
        cfg["tools"]["web"]["search"] = {}
    cfg["tools"]["web"]["search"]["api_key"] = web_search_key

with open(config_file, "w") as f:
    json.dump(cfg, f, indent=2)

tg_status = "ON" if tg_enabled == "true" else "OFF"
search_status = "ON" if web_search_key else "OFF"
print(f"Config updated: model={cfg['agents']['defaults'].get('model', 'N/A')}, telegram={tg_status}, web_search={search_status}")
PYEOF
else
    echo "Warning: python3 not available, config.json may not have API keys from env vars"
fi

# Garante que os templates de personalidade existam no workspace
# (o volume pode sobrescrever os do build, entao copia se nao existirem)
WORKSPACE="$HOME/.picoclaw/workspace"
for f in SOUL.md AGENTS.md USER.md IDENTITY.md; do
    if [ ! -f "$WORKSPACE/$f" ] && [ -f "/opt/picoclaw-templates/$f" ]; then
        cp "/opt/picoclaw-templates/$f" "$WORKSPACE/$f"
    fi
done
if [ ! -f "$WORKSPACE/memory/MEMORY.md" ] && [ -f "/opt/picoclaw-templates/MEMORY.md" ]; then
    mkdir -p "$WORKSPACE/memory"
    cp "/opt/picoclaw-templates/MEMORY.md" "$WORKSPACE/memory/MEMORY.md"
fi

# Executa o PicoClaw com os argumentos passados
exec picoclaw "$@"
