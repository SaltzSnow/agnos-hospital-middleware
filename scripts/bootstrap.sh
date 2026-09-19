#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
if [ -e .env ]; then
    printf '%s\n' '.env already exists; keeping existing configuration.'
    exit 0
fi
command -v openssl >/dev/null 2>&1 || { printf '%s\n' 'openssl is required to generate secrets.' >&2; exit 1; }
umask 077
# noclobber prevents overwriting a file created concurrently.
set -C
{
    printf 'POSTGRES_PASSWORD=%s\n' "$(openssl rand -hex 24)"
    printf 'JWT_SECRET=%s\n' "$(openssl rand -hex 32)"
    printf 'REGISTRATION_KEY=%s\n' "$(openssl rand -hex 32)"
    printf 'HTTP_PORT=8080\n'
} > .env
chmod 600 .env
printf '%s\n' 'Created .env with random local credentials (mode 600).'
