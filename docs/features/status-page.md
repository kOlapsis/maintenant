# Public Status Page

Give your users a clean status page. Component groups, incident management with timeline updates, scheduled maintenance windows.

---

## How It Works

maintenant serves a standalone public status page at `/status/`. It is a server-rendered HTML page (no JavaScript required) with a real-time SSE connection for live updates.

The status page displays:

- **Component groups** — Logical groupings of services (e.g., "API", "Database", "Frontend")
- **Component status** — Operational, degraded, partial outage, major outage
- **Active incidents** — Current issues with timeline updates
- **Scheduled maintenance** — Upcoming planned downtime windows

---

## Component Groups

Organize your monitored services into logical groups:

```bash
# Create a group
POST /api/v1/status/groups
{
  "name": "Core Services",
  "sort_order": 1
}
```

---

## Components

Link monitored resources to status page components. Each component maps to a container, endpoint, heartbeat, or certificate monitor.

```bash
# Create a component linked to a container
POST /api/v1/status/components
{
  "group_id": 1,
  "name": "API Server",
  "monitor_type": "container",
  "monitor_id": 42,
  "sort_order": 1
}
```

Supported `monitor_type` values:

| Type | Source |
|------|--------|
| `container` | Container state and health |
| `endpoint` | HTTP/TCP check status |
| `heartbeat` | Heartbeat ping status |
| `certificate` | TLS certificate validity |

maintenant automatically derives the component status from the linked monitor:

| Monitor State | Component Status |
|---------------|-----------------|
| Running / Up / Valid | Operational |
| Unhealthy / Expiring | Degraded |
| Down / Expired / Stopped | Major Outage |

---

## Status Values

| Status | Meaning |
|--------|---------|
| `operational` | Everything working normally |
| `degraded` | Service is slow or partially impaired |
| `partial_outage` | Some functionality unavailable |
| `major_outage` | Service is down |
| `maintenance` | Planned maintenance in progress |

---

## Public Access

The status page is served at `/status/` and is designed to be publicly accessible without authentication.

!!! warning "Reverse proxy configuration"
    Configure your reverse proxy to allow unauthenticated access to `/status/` paths.
    See [Configuration](../getting-started/configuration.md#public-routes) for details.

The status page includes:

- A real-time SSE stream at `/status/events` for live status updates
- Self-contained HTML with embedded CSS (no external dependencies)
- Responsive design for mobile and desktop

---

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/status/groups` | List all component groups |
| `POST` | `/api/v1/status/groups` | Create a group |
| `PUT` | `/api/v1/status/groups/{id}` | Update a group |
| `DELETE` | `/api/v1/status/groups/{id}` | Delete a group |
| `GET` | `/api/v1/status/components` | List all components |
| `POST` | `/api/v1/status/components` | Create a component |
| `PUT` | `/api/v1/status/components/{id}` | Update a component |
| `DELETE` | `/api/v1/status/components/{id}` | Delete a component |

---

## Related

- [Alert Engine](alerts.md) — Alerts that feed into incident creation
- [Configuration](../getting-started/configuration.md#public-routes) — Public route setup
