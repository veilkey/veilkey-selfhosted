# veil CLI — Usage

## Resolve a secret

```bash
veilkey-cli resolve VK:LOCAL:yyyyyyyy
# Output: actual-secret-value
```

## Execute with ref replacement

```bash
veilkey-cli exec echo VK:LOCAL:yyyyyyyy
# Output: actual-secret-value (ref replaced in args)
```

## PTY masking session

```bash
# Native CLI
veil

# Docker Compose
docker compose exec veil veilkey-cli wrap-pty bash
```

Inside the veil shell:
- Real secrets in output are replaced with `VK:LOCAL:xxx` — AI cannot see them
- Processes receive the actual value — your app works normally
- `echo $MY_SECRET` shows `VK:LOCAL:yyyyyyyy` on screen, but the process sees `actual-secret-value`

## Verify masking

```bash
veilkey-cli wrap-pty sh -c "echo actual-secret-value"
# Screen output: VK:LOCAL:yyyyyyyy  (masked!)
```

## Check connection

```bash
veilkey-cli status
# Shows: API URL, connection status, pattern count
```

## Scan for secrets

```bash
veilkey-cli scan file.env
# Detects secrets using 222 built-in patterns
```

## Run services with VK refs

### Direct / systemd / cron

Wrap with `veil` — VK refs in env vars are auto-resolved:

```bash
# Direct
veil node app.js

# systemd
ExecStart=/usr/local/bin/veil /usr/bin/node app.js

# cron
* * * * * /usr/local/bin/veil /path/to/script.sh
```

`.env` contains only VK refs, never plaintext.

### Docker

Use the entrypoint wrapper — resolves VK refs at container start:

```dockerfile
COPY docker-entrypoint-veilkey.sh /usr/local/bin/
COPY --from=veilkey-veil:dev /usr/local/bin/veilkey-cli /usr/local/bin/
ENTRYPOINT ["docker-entrypoint-veilkey.sh"]
CMD ["node", "app.js"]
```

See [`examples/docker-entrypoint-veilkey.sh`](../../../examples/docker-entrypoint-veilkey.sh) and [`examples/Dockerfile.veilkey-app`](../../../examples/Dockerfile.veilkey-app).
