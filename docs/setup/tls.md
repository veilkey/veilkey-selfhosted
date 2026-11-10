# TLS Certificate Provisioning

Host CLI clients and standalone LocalVault installers verify TLS by default (`VEILKEY_TLS_INSECURE=0`). A self-signed certificate is not a reason to disable verification. Provision trust before registration, unlock, or any request containing credentials.

## Provision service certificates

1. Issue VaultCenter and LocalVault server certificates from a public CA or your organization's private CA. Include every DNS name or IP address clients actually use in the certificate's Subject Alternative Name (SAN).
2. Transfer certificates through an authenticated administrative channel. Verify the CA certificate's SHA-256 fingerprint against your administrator's independently supplied fingerprint. Never establish trust by downloading a certificate from an unverified HTTPS endpoint.
3. Put the server certificate chain and private key on each service host and set `VEILKEY_TLS_CERT` and `VEILKEY_TLS_KEY` to their paths. Restrict private-key permissions to the service account. Restart the service to load them.
4. Distribute only the public CA certificate/bundle to clients. Never distribute server private keys or store master passwords in certificate directories, `.env`, or password files.

## Linux service and curl trust

For Debian/Ubuntu, install the verified PEM CA certificate (one certificate per `.crt` file):

```bash
sudo install -m 0644 /path/to/verified-ca.pem /usr/local/share/ca-certificates/veilkey-ca.crt
sudo update-ca-certificates
```

Go services use system roots. Alternatively, pass an absolute PEM bundle path to the installer as `SSL_CERT_FILE=/path/to/verified-ca.pem`; standalone installers persist this public path for the service. Set it before `init --token` as well. For an individual curl request, use explicit trust:

```bash
curl --cacert /path/to/verified-ca.pem https://<VC_HOST>:<VC_PORT>/health
```

## Host CLI trust (macOS and Linux)

The Rust CLI uses public roots by default. For a private CA or a verified self-signed service certificate, set its PEM bundle explicitly:

```bash
export VEILKEY_TLS_INSECURE=0
export VEILKEY_CA_CERT=/path/to/verified-ca.pem
veilkey-cli status
```

`VEILKEY_CA_CERT` replaces the CLI's root bundle; include all required CA certificates when contacting more than one service. An unreadable, empty, or invalid bundle fails closed. Pass this variable when running the CLI installer to save its public path in the generated CLI environment file. OS Keychain trust alone does not configure the Rust CLI's private CA bundle.

On macOS, browsers and the system curl can also trust the verified CA via Keychain:

```bash
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain /path/to/verified-ca.pem
```

On Windows, import the verified CA through your organization's certificate-management policy or the Certificates console before using host-side API tools. The Rust CLI still needs `VEILKEY_CA_CERT` for a private CA.

## Existing single-host Docker Compose exception

The bundled `docker-compose.yml` retains `VEILKEY_TLS_INSECURE=1` **only for the three containers communicating inside one single-host Docker network** with generated self-signed certificates. This is an explicit deployment exception, not a host-client default. It does not authenticate peers; anyone controlling that network is inside this trust boundary. Do not use it for remote LocalVaults, host CLI clients, multi-host networks, registration over an external network, or internet-facing service-to-service links.

Before extending the deployment beyond one host, provision CA-signed certificates on both services, mount the verified CA bundle into each container, set `SSL_CERT_FILE` for Go and `VEILKEY_CA_CERT` for the CLI, and set every Compose `VEILKEY_TLS_INSECURE` entry to `"0"`. Validate SANs and health checks with verification enabled before sending credentials. The included `curl -k` container health check is also limited to this local deployment exception; use `--cacert` when replacing it.

## Trusted addresses are not authentication

The root `.env.example` trusts no IP addresses by default. Standalone installers resolve only the VaultCenter address (or accept an explicit operator override), not private-network ranges or loopback. For NAT or multiple VaultCenter addresses, configure only the exact source addresses observed by LocalVault. A trusted address does not replace a registration token, admin session, or agent Bearer token.
