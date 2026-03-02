# TLS Certificate Monitoring

Automatic certificate detection from your HTTPS endpoints, plus standalone monitors for any domain. Never get surprised by an expired certificate again.

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

## Alert Events

| Event | Description | Severity |
|-------|-------------|----------|
| `expiring` | Certificate approaching expiry (30, 14, 7, 3, 1 day thresholds) | Critical |
| `expired` | Certificate has expired | Critical |
| `chain_invalid` | Certificate chain validation failed | Critical |

---

## Managing Certificate Monitors

### Standalone Monitors

Create a monitor for any domain:

```bash
# Create a certificate monitor
POST /api/v1/certificates
{
  "domain": "example.com",
  "port": 443
}
```

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
  maintenant.certificate.domain: "api.example.com"
  maintenant.certificate.port: "443"
```

See the [Docker Labels Reference](../guides/docker-labels.md) for the full list of certificate-related labels.

---

## Related

- [Endpoint Monitoring](endpoints.md) — HTTPS endpoints get automatic certificate monitoring
- [Alert Engine](alerts.md) — Certificate expiry alerts
- [Docker Labels Reference](../guides/docker-labels.md) — Certificate labels
