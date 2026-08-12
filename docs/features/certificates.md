# TLS Certificate Monitoring

Automatic certificate detection from your HTTPS endpoints, plus standalone monitors for any domain. Never get surprised by an expired certificate again.

![TLS Certificate Monitoring](../screen-captures/4-certificates.png)

---

## How It Works

maintenant monitors TLS certificates in two ways:

1. **Automatic detection** — When you configure an HTTPS endpoint check, maintenant automatically monitors the certificate on that domain.
2. **Standalone monitors** — Add any domain manually through the API, even if it is not part of your monitored stack.

maintenant connects to the domain, performs a full TLS handshake, parses the certificate chain, and records:

- Issuer and subject
- Expiry date
- Days until expiration
- Chain validity
- OCSP staple (Personal, see [OCSP Stapling](#ocsp-stapling) below)

---

## Alert Thresholds

maintenant alerts at multiple thresholds before certificate expiry:

| Days Before Expiry | Alert |
|--------------------:|-------|
| 30 days | First warning |
| 14 days | Second warning |
| 7 days | Urgent |
| 3 days | Critical |
| 1 day | Final warning |
| 0 days | **Expired** |

All certificate alerts are sent with **Critical** severity by default.

---

## Full Chain Validation

maintenant validates the entire certificate chain, not just the leaf certificate:

- **Leaf certificate** — The server's own certificate
- **Intermediate certificates** — Issued by the CA to sign the leaf
- **Root certificate** — The trusted root CA

If any certificate in the chain is invalid, expired, or missing, maintenant fires a `chain_invalid` alert.

---

## Trusting an internal CA

If you run your own PKI — step-ca, Smallstep, an internal Active Directory CA — point maintenant at your root certificate:

```yaml
services:
  maintenant:
    environment:
      MAINTENANT_CA_CERT: /etc/maintenant/ca.pem
    volumes:
      - ./ca.pem:/etc/maintenant/ca.pem:ro
```

The bundle is **added** to the system roots, so public CAs keep working. It applies to endpoint probes, certificate monitoring, webhooks and SMTP alike. A CLI equivalent exists for one-off runs: `--ca-cert /path/to/ca.pem`.

Three things to watch out for:

- **The file must be readable by uid 65534.** The container drops to an unprivileged user, and a root-owned `0600` file — the default output of most CA tooling — will not be readable. `chmod 644` the copy you mount.
- **Provide it to whichever process performs the check.** An endpoint attached to an agent is probed by that agent, on its own host. Setting the variable on the server changes nothing for it.
- **Do not use `SSL_CERT_FILE` for this.** Go treats it as a *replacement* for the system bundle, not an addition, so every public CA disappears the moment you set it. Worse, if the file cannot be read, Go returns an empty trust store with no error at all and every HTTPS check starts failing with "unknown authority" and nothing in the logs. `MAINTENANT_CA_CERT` refuses to start instead.

A bad path, an unreadable file, or a PEM with no certificate in it all stop the process with an explicit message rather than silently degrading every check.

---

## Untrusted certificates are degraded, not down

A host that answers normally but presents a certificate maintenant cannot validate is reported as **degraded** (orange), not **down** (red). The host is not the problem — its chain of trust is. It keeps counting as available in uptime, and raises a `certificate_untrusted` alert at *warning* severity instead of a critical outage.

The certificate is still collected in this state, so expiry monitoring keeps working on internal-PKI hosts even before you configure the CA.

A host that is genuinely unreachable — timeout, DNS failure, connection refused — remains **down**.

---

## Alert Events

| Event | Description | Severity |
|-------|-------------|----------|
| `expiring` | Certificate approaching expiry (30, 14, 7, 3, 1 day thresholds) | Critical |
| `expired` | Certificate has expired | Critical |
| `chain_invalid` | Certificate chain validation failed | Critical |
| `ocsp_revoked` | OCSP responder reports the certificate as revoked (Pro) | Critical |

---

## OCSP Stapling :material-star-four-points:{ title="Personal" }

Expired isn't the only way a certificate goes bad — it can also be **revoked** by the issuing CA while still well within its validity period (private key compromise, mis-issuance, decommissioned service). maintenant catches this by reading the **OCSP staple** delivered by the server during the TLS handshake.

### How it works

On every certificate check — for both standalone monitors and HTTPS endpoints — maintenant inspects the `CertificateStatusResponses` field returned by the TLS handshake, parses the OCSP response, validates its signature against the issuer chain, and persists the result alongside the other check fields.

| Status | Meaning | Behavior |
|--------|---------|----------|
| `good` | OCSP responder confirms the certificate is valid | Silent |
| `revoked` | OCSP responder reports the certificate as revoked | Emits `ocsp_revoked` critical alert |
| `unknown` | No staple presented, or staple is stale (`NextUpdate` in the past) | Silent — avoids false alerts during responder outages |
| `error` | Staple is present but cannot be parsed or validated | Silent on alerts, surfaced in the UI for diagnosis |

The `ocsp_revoked` alert is wired into the standard pipeline, so existing **Alert Triggers**, **Escalation Policies**, **silence rules**, and **acknowledgments** apply without extra configuration. The alert is resolved automatically the moment a subsequent check returns `good` — typically right after a fresh certificate is deployed.

### Persisted fields

The following fields are added to each row in the certificate check history:

| Field | Meaning |
|-------|---------|
| `ocsp_status` | `good`, `revoked`, `unknown` or `error` |
| `ocsp_produced_at` | Timestamp the OCSP response was issued by the responder |
| `ocsp_next_update` | Timestamp until which the staple is considered fresh |
| `ocsp_revoked_at` | Set when the responder reports a revocation (with the revocation timestamp) |
| `ocsp_parse_error` | Set when the staple is present but unparseable (signature mismatch, malformed payload, etc.) |

They are exposed via `GET /api/v1/certificates/{id}/checks` and surfaced in the UI through the **OCSP block** in the certificate slideover and the **History** tab.

!!! note "Out-of-band OCSP queries"
    maintenant only reads staples delivered by the server — it does not actively contact the OCSP responder. Servers that do not staple are reported as `unknown` and never alert. Enable OCSP stapling on your server (Caddy and modern nginx do this by default) to get full coverage.

### Pro gating

OCSP stapling is a **Pro** feature. Capture runs in Community to keep the code path uniform, but persistence, API exposure, and `ocsp_revoked` alert emission are skipped on Community installs. The `ocsp_stapling` feature flag is exposed on `GET /api/v1/edition` so the frontend can render a Pro teaser instead of an empty OCSP block.

---

## Managing Certificate Monitors

### Standalone Monitors

Create a monitor for any domain:

```bash
# Create a certificate monitor
POST /api/v1/certificates
{
  "hostname": "example.com",
  "port": 443
}
```

#### SNI checks (`server_name`)

The optional `server_name` field is presented as **SNI** during the TLS handshake, and the received certificate is validated against it instead of `hostname`. This lets you check which certificate a reverse proxy would serve for a given virtual host — for example, verifying every proxy of a keepalived/failover pair before a failover happens:

```bash
POST /api/v1/certificates
{
  "hostname": "proxy2.lan",
  "port": 443,
  "server_name": "service.example.com"
}
```

Several monitors can target the same `hostname:port` with different `server_name` values — one per virtual host you want covered. `server_name` is fixed at creation time; delete and recreate the monitor to change it. When it is empty, the certificate is validated against `hostname` as before.

### API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/certificates` | List all certificate monitors |
| `POST` | `/api/v1/certificates` | Create a standalone monitor |
| `GET` | `/api/v1/certificates/{id}` | Get certificate details |
| `PUT` | `/api/v1/certificates/{id}` | Update a monitor |
| `DELETE` | `/api/v1/certificates/{id}` | Delete a monitor |
| `GET` | `/api/v1/certificates/{id}/checks` | List check history |

---

## Docker Labels

Add certificate monitoring to a container using labels:

```yaml
labels:
  maintenant.tls.certificates: "api.example.com,dashboard.example.com:8443"
```

Domains are comma-separated. Port defaults to `443` if omitted. See the [Docker Labels Reference](../guides/docker-labels.md) for details.

---

## Related

- [Endpoint Monitoring](endpoints.md) — HTTPS endpoints get automatic certificate monitoring
- [Alert Engine](alerts.md) — Certificate expiry alerts
- [Docker Labels Reference](../guides/docker-labels.md) — Certificate labels
