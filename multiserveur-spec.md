# Feature Specification: Architecture Multi-Serveur Pro (gRPC)

**Branch**: `012-multiserver-pro` | **Spec**: [spec.md](multiserveur-spec.md)
**Created**: 2026-04-20
**Status**: Draft
**Licence**: Enterprise uniquement (CE reste en mode embedded single-process)
**Input**: User description: "mise en place multiserver — le serveur sert le front et l'API et le MCP, des agents découvrent la stack (docker, swarm, k8s) et transmettent les données au serveur via gRPC. Push only. Un agent = un runtime. Un seul binaire. Identité SHM-style (Ed25519 persistée localement). Multi-serveur réservé à la version Pro."

---

## Vue d'ensemble

Maintenant est aujourd'hui un binaire auto-suffisant qui lit directement les runtimes (Docker/Swarm/K8s) via leurs SDK locaux. Cette feature introduit, **uniquement en édition Enterprise**, une architecture distribuée : le binaire peut fonctionner dans trois modes (`embedded`, `server`, `agent`), et des agents déployés sur des hôtes distants poussent leurs observations vers un serveur central via gRPC streaming.

Le serveur central conserve l'intégralité de la surface utilisateur (UI Vue embarquée, API REST, serveur MCP) et devient le point d'agrégation unique pour l'ensemble du parc. Les agents sont silencieux : ils collectent et pushent, ne servent aucune interface, n'acceptent aucune commande entrante.

**En CE, seul le mode `embedded` est accessible.** L'expérience reste strictement inchangée (binaire unique, runtime local, pas de gRPC).

---

## Contraintes d'architecture fondamentales

1. **Un seul binaire** (`maintenant`) avec flag `--mode=embedded|server|agent`. Par défaut : `embedded`.
2. **Push-only** — les agents pushent des événements observés ; ils ne reçoivent aucune commande du serveur (pas de reconfiguration à chaud, pas d'exec probe, pas de pilotage). Si l'admin veut ajouter un monitor HTTP ou un certificat à surveiller, il le fait depuis l'UI du serveur, qui l'exécute lui-même si l'endpoint est accessible, ou qui demande à l'agent correspondant via sa config.
3. **Un agent = un runtime** — l'agent détecte Docker, Swarm OU Kubernetes, pas deux simultanément. La détection échoue si aucun ou plusieurs runtimes sont présents et exige un flag explicite.
4. **Transport gRPC sur TLS standard** — pas de mTLS (PKI interne trop lourde pour un produit self-hosted).
5. **Authentification applicative Ed25519** — pattern SHM : chaque agent génère sa paire de clés au premier boot, s'enregistre avec un enrollment token, et prouve la possession de sa clé privée en signant un nonce au reconnect.
6. **MCP reste centralisé** — un seul serveur MCP par déploiement Maintenant, exposé par le serveur central. Il voit l'ensemble du parc comme une seule source unifiée.
7. **Gating Enterprise double** — UI + API : les modes `server`/`agent` refusent de démarrer sans licence valide, les endpoints gRPC refusent toute connexion avec `PRO_REQUIRED`.
8. **SQLite reste le seul store** sur le serveur. Pas de base distribuée. Le serveur est un point central par design.

---

## User Scenarios

### User Story 1 — Enrôlement d'un nouvel agent (Priority: P1)

Un administrateur souhaite monitorer un hôte Docker distant (VM, serveur dédié, etc.) depuis son instance Maintenant Pro centrale. Depuis l'UI, section "Agents", il clique "Generate enrollment token". Le serveur affiche un token à usage unique (`mnt_enr_<random>`, TTL 24h) et une commande copier-coller du type `maintenant agent --server=grpcs://... --enrollment-token=mnt_enr_xxx`. L'admin se connecte à l'hôte distant, installe le binaire, lance la commande. L'agent génère sa paire Ed25519, la sauvegarde dans `/var/lib/maintenant/identity.json` (perms 0600), ouvre un stream gRPC vers le serveur, envoie son `RegisterRequest`. Le serveur valide le token, consomme, persiste la pubkey, répond OK. L'agent apparaît dans l'UI avec statut `active`.

**Why this priority**: Sans enrôlement sécurisé, l'architecture distribuée n'est pas exploitable. C'est le point d'entrée unique pour le déploiement d'un nouvel agent.

**Independent Test**: Lancer un serveur et un agent sur deux machines (ou deux containers), générer un token via API, lancer l'agent avec ce token, vérifier qu'il apparaît en `active` dans la liste.

**Acceptance Scenarios**:
1. **Given** un serveur Pro en fonctionnement et un token d'enrôlement valide, **When** un agent exécute sa commande avec ce token, **Then** l'agent apparaît en status `active` dans l'UI, et sa paire de clés est persistée localement.
2. **Given** un token déjà consommé ou expiré, **When** un agent tente le register, **Then** le serveur rejette avec un code gRPC explicite (`FailedPrecondition: enrollment_token_invalid`) et l'agent écrit l'erreur en log avant d'arrêter.
3. **Given** un agent qui crashe après avoir persisté son identité, **When** il redémarre, **Then** il ne fait PAS un nouveau register (car déjà enregistré), il fait un `Authenticate` avec sa clé existante et reprend le stream normalement.
4. **Given** deux runtimes présents sur l'hôte (ex: Docker + minikube), **When** l'agent démarre sans flag explicite, **Then** il refuse de démarrer avec un message indiquant les runtimes détectés et suggérant `--runtime=docker|kubernetes`.

---

### User Story 2 — Collecte et push en temps réel (Priority: P1)

Un agent enregistré collecte en continu les événements de son runtime local — démarrages/arrêts de containers, résultats de probes d'endpoints, pings de heartbeats, métriques de ressources, certificats TLS détectés — et les pousse au fil de l'eau via son stream gRPC vers le serveur. Le serveur reçoit, désérialise, dispatch aux services métier existants (container, endpoint, heartbeat, resource, certificate) qui persistent en SQLite et émettent sur le SSE bus. L'UI voit les événements en quasi temps réel comme si l'agent était local.

**Why this priority**: C'est la raison d'être de l'agent. Sans collecte fonctionnelle, l'enrôlement est inutile.

**Independent Test**: Après enrôlement, démarrer un nouveau container sur l'hôte de l'agent. Vérifier qu'il apparaît dans l'UI du serveur en moins de 5 secondes avec le bon tag d'agent.

**Acceptance Scenarios**:
1. **Given** un agent Docker actif, **When** un container démarre sur l'hôte de l'agent, **Then** l'événement apparaît dans l'UI du serveur en moins de 5 secondes, associé à l'`agent_id` correspondant.
2. **Given** un agent dont le stream drop (réseau coupé, serveur redémarre), **When** la connexion est restaurée, **Then** l'agent se reconnecte automatiquement avec backoff exponentiel (base 1s, max 60s), ré-authentifie avec sa clé, et reprend le push.
3. **Given** un agent qui tente de pusher sans authentification valide (clé privée corrompue, status revoked), **When** le stream est ouvert, **Then** le serveur refuse avec `PermissionDenied` et l'agent logge l'erreur sans retry aveugle.

---

### User Story 3 — Gestion et révocation depuis l'UI (Priority: P2)

L'administrateur consulte la page "Agents" du serveur : liste de tous les agents enregistrés avec `agent_id`, hostname, runtime détecté (Docker/Swarm/K8s), version binaire, dernière activité, statut (`active`/`revoked`/`disconnected`). Il peut révoquer un agent en un clic : son statut passe à `revoked`, son stream actif est fermé, la prochaine tentative d'auth est refusée.

**Why this priority**: La visibilité et le contrôle sont essentiels en exploitation, mais l'enrôlement et la collecte doivent fonctionner d'abord.

**Acceptance Scenarios**:
1. **Given** plusieurs agents enregistrés, **When** l'admin ouvre la page "Agents", **Then** il voit la liste complète avec statut temps-réel (via SSE).
2. **Given** un agent `active`, **When** l'admin clique "Revoke", **Then** le stream actif de cet agent est fermé immédiatement côté serveur, et l'agent (s'il tente de se reconnecter) reçoit `PermissionDenied: agent_revoked`.
3. **Given** un agent qui n'a plus pushé depuis > 60 secondes (configurable), **When** on consulte la liste, **Then** il est marqué `disconnected` (distinct de `revoked`).
4. **Given** un agent révoqué dont on veut réactiver l'identité, **When** l'admin clique "Delete" puis re-génère un enrollment token, **Then** l'agent doit refaire un register complet avec nouvelle identité.

---

### User Story 4 — Filtrage par agent dans l'UI (Priority: P3)

Un utilisateur navigue dans l'interface et peut filtrer toutes les vues (containers, endpoints, heartbeats, certificates, resources) par agent ou voir une vue agrégée de tout le parc. Chaque entité affiche un badge avec le nom de l'agent source.

**Why this priority**: Améliore significativement l'UX multi-hôtes mais n'est pas bloquant pour la valeur de base.

**Acceptance Scenarios**:
1. **Given** des données provenant de 3 agents différents, **When** l'utilisateur sélectionne un filtre sur un agent spécifique, **Then** seules les données de cet agent sont affichées.
2. **Given** un utilisateur en mode agrégé, **When** il consulte la liste des containers, **Then** chaque ligne affiche un badge compact avec le nom de l'agent source.

---

### Edge Cases

- **Déduplication si embedded + agent sur même infra** : si un administrateur active le mode `server` sur une instance qui a aussi un runtime local accessible, il peut soit désactiver la découverte locale (`--no-local-runtime`), soit laisser Maintenant la traiter comme un runtime "virtuel" équivalent à un agent local. Décision : **le serveur en mode `server` n'exécute PAS de découverte de runtime local par défaut** — s'il veut aussi monitorer son hôte, il lance un second processus en mode `agent` ou utilise le flag `--embedded-agent`.
- **Versions différentes agent/serveur** : incompatibilité majeure → refus strict au register avec message explicite. Incompatibilité mineure → accepté avec warning log.
- **Hostname conflict** : deux agents avec le même hostname sont autorisés — l'`agent_id` (UUID) est la clé primaire. Le hostname est un label display.
- **Enrollment token réutilisé** : one-time consumption atomique en DB (transaction). Deuxième usage → refusé.
- **Agent dont la clé privée fuit** : révocation admin → ferme le stream existant et refuse toute future auth avec cet `agent_id`. L'admin doit re-enrôler avec nouvelle identité.
- **Clock skew entre agent et serveur** : le nonce signé inclut un `timestamp` unix vérifié avec tolérance ±5 minutes. Au-delà → `DeadlineExceeded: clock_skew`.
- **Volume anormal** : un agent qui pousse plus de N events/seconde (configurable, défaut 1000/s) est rate-limited côté serveur avec `ResourceExhausted`. Il met en pause son émission pendant le delai indiqué.
- **Serveur redémarre pendant que des agents streament** : les streams drop côté agent, backoff exponentiel prend le relais, les agents re-authentifient dès que le serveur revient. Aucune perte d'identité.

---

## Requirements

### Functional Requirements

#### Modes et binaire

- **FR-001** : Le binaire `maintenant` DOIT supporter un flag `--mode=embedded|server|agent`, défaut `embedded`.
- **FR-002** : Le mode `embedded` DOIT être **strictement équivalent** au comportement actuel (single-process, runtime local, UI + API + MCP dans le même binaire, aucune dépendance réseau sortante vers un serveur Maintenant tiers). Ce mode DOIT fonctionner en CE et en Pro.
- **FR-003** : Les modes `server` et `agent` DOIVENT refuser de démarrer sans licence Enterprise valide (vérification au boot, message d'erreur explicite).
- **FR-004** : En mode `server`, le binaire DOIT démarrer : (a) serveur HTTP (UI + API REST + MCP), (b) serveur gRPC d'ingestion, (c) planificateur interne. Il NE DOIT PAS exécuter de découverte runtime locale sauf si `--embedded-agent` est explicitement passé.
- **FR-005** : En mode `agent`, le binaire DOIT démarrer : (a) détection du runtime, (b) collecte locale, (c) client gRPC vers le serveur configuré. Il NE DOIT PAS exposer d'UI, d'API HTTP, ni de MCP.

#### Détection runtime

- **FR-006** : L'agent DOIT détecter un seul runtime parmi Docker, Swarm, Kubernetes. En cas d'ambiguïté, il DOIT refuser de démarrer et suggérer un flag `--runtime=` explicite.
- **FR-007** : L'agent DOIT échouer au démarrage avec un message clair si aucun runtime n'est détecté.

#### Identité et enrôlement

- **FR-008** : L'agent DOIT générer une paire de clés Ed25519 au premier démarrage et la persister dans un fichier `identity.json` avec permissions 0600.
- **FR-009** : Le fichier d'identité DOIT contenir : `agent_id` (UUID v4), `private_key` (hex), `public_key` (hex), `created_at` (ISO 8601).
- **FR-010** : Le serveur DOIT permettre à un admin authentifié de générer un enrollment token via l'UI et via l'API (`POST /api/v1/agents/enrollment-tokens`).
- **FR-011** : Un enrollment token DOIT être à usage unique (consumed atomiquement), avec TTL de 24h par défaut (configurable).
- **FR-012** : L'enrôlement DOIT être un échange gRPC unaire `RegisterAgent(RegisterRequest) → RegisterResponse` où l'agent envoie `{agent_id, public_key, enrollment_token, hostname, os_arch, agent_version, detected_runtime}` et reçoit `{server_time, agent_config}`.
- **FR-013** : Après un register réussi, l'agent DOIT persister la confirmation localement (`registered: true` ou timestamp) pour éviter de re-tenter un register au prochain boot.

#### Authentification et stream

- **FR-014** : L'ouverture d'un stream gRPC de push DOIT commencer par un handshake d'auth : le serveur envoie un `AuthChallenge{nonce: 32 bytes random}`, l'agent répond `AuthResponse{agent_id, timestamp, signature: Ed25519(nonce || agent_id || timestamp)}`.
- **FR-015** : Le serveur DOIT vérifier la signature avec la pubkey enregistrée, vérifier `status=active`, et vérifier `|server_time - timestamp| < 5min`. Toute vérification échouée DOIT fermer le stream avec le code gRPC approprié (`PermissionDenied`, `DeadlineExceeded`).
- **FR-016** : Une fois le handshake validé, le stream DOIT rester authentifié pour sa durée de vie. Pas de re-signature par message.
- **FR-017** : L'agent DOIT se reconnecter automatiquement en cas de drop du stream, avec backoff exponentiel (base 1s, facteur 2, plafond 60s, jitter).

#### Collecte et push

- **FR-018** : L'agent DOIT pusher les événements suivants via son stream : container state (created/started/stopped/removed), endpoint probe results, heartbeat pings reçus localement, resource metrics (CPU/RAM/disk/network), certificate info détectée.
- **FR-019** : Chaque message poussé DOIT inclure `agent_id`, `event_id` (UUID), `observed_at` (timestamp de l'observation côté agent).
- **FR-020** : L'agent DOIT coalescer les événements haute fréquence (ex: métriques de ressources) selon un schedule configurable (défaut : 10s pour resources, temps-réel pour containers/endpoints/heartbeats).
- **FR-021** : Le serveur DOIT rejeter avec `ResourceExhausted` un agent dépassant 1000 events/s (configurable), en indiquant un delai de rétention.
- **FR-022** : Le serveur DOIT dispatcher les événements reçus vers les services existants (container, endpoint, heartbeat, resource, certificate) qui persistent en SQLite et émettent sur le SSE bus pour l'UI.

#### Gestion et révocation

- **FR-023** : Le serveur DOIT exposer des endpoints REST pour gérer les agents : `GET /api/v1/agents`, `GET /api/v1/agents/:id`, `POST /api/v1/agents/:id/revoke`, `DELETE /api/v1/agents/:id`, `GET /api/v1/agents/enrollment-tokens`, `POST /api/v1/agents/enrollment-tokens`.
- **FR-024** : La révocation d'un agent DOIT prendre effet immédiatement : (a) marquer status=revoked en DB, (b) fermer le stream gRPC actif s'il existe, (c) refuser toute future auth.
- **FR-025** : Un agent sans push depuis plus de 60 secondes (configurable) DOIT être marqué `disconnected` dans l'UI sans toucher son status DB (reste `active`).
- **FR-026** : La suppression d'un agent (DELETE) DOIT être un hard delete (cohérent avec les préférences du projet) : l'agent_id et la pubkey disparaissent ; les événements historiques peuvent soit être purgés soit gardés avec `agent_id` orphelin (décision en Phase 6).

#### UI et filtrage

- **FR-027** : L'UI DOIT exposer une section "Agents" (réservée Pro, gatée via `FeatureGate` en CE) avec : liste, detail slideover, enrollment flow, révocation.
- **FR-028** : Les vues existantes (containers, endpoints, heartbeats, certificates, resources) DOIVENT supporter un filtre par agent en mode Pro multi-agent. En mode embedded, ce filtre est invisible.
- **FR-029** : Chaque entité liée à un agent DOIT afficher un badge compact avec le nom de l'agent source dans les listes.

#### MCP

- **FR-030** : Le serveur MCP DOIT rester centralisé côté serveur. Les tools MCP existants (`list_containers`, `list_endpoints`, etc.) DOIVENT voir l'ensemble du parc comme une seule source unifiée. Des filtres optionnels `agent_id` peuvent être ajoutés aux tools multi-source.

#### Sécurité et gating

- **FR-031** : Le serveur gRPC DOIT exiger TLS (pas de plaintext). L'agent DOIT vérifier le certificat serveur. Option `--insecure-skip-tls-verify` disponible uniquement pour debug (warning log au boot).
- **FR-032** : Tous les endpoints REST de gestion d'agents DOIVENT être derrière `requireEnterprise()`.
- **FR-033** : Le serveur gRPC d'ingestion DOIT refuser toute connexion en CE avec un code `PermissionDenied: pro_required`.

### Key Entities

- **Agent** : `agent_id` (UUID, PK), `public_key` (32 bytes hex), `hostname`, `os_arch`, `agent_version`, `detected_runtime` (docker|swarm|kubernetes), `status` (active|revoked), `last_seen_at`, `created_at`.
- **EnrollmentToken** : `token` (random 32 bytes, PK), `created_by` (user), `expires_at`, `consumed_at` (nullable), `consumed_by_agent_id` (nullable).
- **AgentEvent** (in-flight, pas persisté tel quel) : wrapper protobuf qui contient `agent_id` + oneof des types d'events (container, endpoint, heartbeat, resource, certificate). Dispatche vers les stores existants.
- **StreamSession** (runtime only, pas en DB) : représente un stream gRPC actif ; mappe `agent_id → grpc.Stream`. Utilisé pour fermer le stream lors d'une révocation.

---

## Success Criteria

- **SC-001** : Un nouvel agent passe de "binaire installé" à "actif et poussant des events" en moins de 2 minutes, y compris la génération du token côté serveur.
- **SC-002** : Latence push-to-UI (événement de container → affichage UI) < 5 secondes en conditions normales.
- **SC-003** : Le serveur supporte au moins 100 agents simultanés connectés sans dégradation perceptible de l'UI (p95 < 1s sur les requêtes principales).
- **SC-004** : La révocation d'un agent ferme son stream et prend effet en moins de 2 secondes.
- **SC-005** : L'empreinte mémoire d'un agent reste sous 50 Mo en charge nominale (< 100 containers monitorés).
- **SC-006** : Le mode `embedded` ne présente aucune régression de performance ni de fonctionnalité par rapport à la version précédente (benchmark runtime-dashboard sur même hardware).
- **SC-007** : Un agent déconnecté (réseau coupé, serveur down) se reconnecte automatiquement dès que le serveur revient, sans intervention manuelle, avec un délai d'au plus 60 secondes.
- **SC-008** : La consommation d'un enrollment token est atomique : aucune double-consommation possible même en cas de race condition (test de charge concurrent).

---

## Assumptions

- **Un seul binaire Go** conservé — pas de split en deux packages. Les modes `server`/`agent`/`embedded` partagent 80% du code (runtime detection, collect logic, domain types).
- **gRPC TLS** est terminé soit au reverse proxy (Caddy, Traefik, Nginx) soit par le binaire lui-même. Les deux DOIVENT être supportés.
- **SQLite reste le seul store** côté serveur. Pas de Redis, Postgres, ou base distribuée.
- **Frontend reste inchangé architecturalement** — Vue + Pinia + SSE. Seules s'ajoutent la section "Agents" et les filtres par agent.
- **Pas de buffering côté agent** en cas de perte de connexion — les events pendant un outage sont perdus (acceptable car on pousse aussi des "state snapshots" périodiques, pas seulement des deltas).
- **Un agent surveille UN runtime** — installer deux agents sur une même machine est la solution pour un hôte multi-runtime (rare).
- **Le fichier `identity.json` est protégé par perms 0600** (pas de chiffrement au repos type AES) — cohérent avec SHM. L'option "wrap avec clé dérivée de machine-id" est un stretch goal hors scope.
- **Enterprise licensing** : les vérifications sont à la charge du paquet `internal/license` existant. Aucun contournement en CE.
- **Upgrade path** : passer de CE à Pro n'implique pas de migration SQL — les tables agents/enrollment_tokens sont créées à la volée au premier démarrage en mode `server`.

---

## Non-Goals

- **Pilotage agent depuis le serveur** : pas d'exec probe, pas de reload config à chaud, pas de commande runtime (exec dans container, etc.). Hors scope. Si l'admin veut ajouter un monitor, il le fait depuis l'UI serveur qui s'exécute lui-même ou via config statique de l'agent.
- **High-availability / load balancing des serveurs** : un seul serveur pour l'instant. Le HA serveur est une feature future séparée.
- **Sharding** des agents entre plusieurs serveurs : non supporté.
- **Proxy / relay d'agents** (agent-qui-relais-pour-d'autres-agents) : non supporté.
- **Distribution automatique du binaire agent** (auto-upgrade, CDN, etc.) : hors scope — l'admin installe manuellement via package manager, docker pull, etc.
- **mTLS et PKI** : non, TLS standard + auth applicative Ed25519.
- **Chiffrement des données at-rest côté serveur** : hors scope (SQLite reste en clair, comme actuellement).
- **Notifications push vers l'agent** : l'agent est en push-only strict.
- **Multi-tenancy** (plusieurs orgs sur un même serveur) : hors scope.

---

## Clarifications

### Session 2026-04-20

- Q: Modèle de transport agent → serveur ? → A: **gRPC streaming unidirectionnel (agent → serveur)**, avec handshake bidir au début pour l'auth (nonce + signature).
- Q: Push ou bidir ? → A: **Push-only**. L'agent ne reçoit aucune commande du serveur. Si l'utilisateur veut ajouter un endpoint ou un cert manuellement, il le fait sur le serveur principal.
- Q: Un agent peut-il gérer plusieurs runtimes simultanément ? → A: **Non, un agent = un runtime**. Rare d'avoir Docker + K8s sur la même machine ; si c'est le cas, deux agents sont installés.
- Q: Un seul binaire ou deux binaires distincts (`maintenant-server` + `maintenant-agent`) ? → A: **Un seul binaire** avec flag `--mode`.
- Q: Auth mTLS ou JWT ? → A: **Aucun des deux, pattern SHM**. Ed25519 généré par l'agent au premier boot, pubkey persistée côté serveur au register, chaque stream authentifié par signature d'un nonce serveur.
- Q: Multi-serveur disponible en CE ? → A: **Non, Pro only**. En CE, seul le mode `embedded` est accessible — expérience actuelle préservée à l'identique.
- Q: MCP centralisé ou par agent ? → A: **Centralisé côté serveur**. Un seul MCP pour toute la stack.
- Q: Identité agent chiffrée au repos ? → A: Perms 0600 (équivalent SHM). Chiffrement AES/keyring hors scope.
- Q: Enrollment token one-time ou multi-usage ? → A: **One-time**, TTL 24h.
- Q: Comportement si le serveur redémarre pendant que des agents streament ? → A: Reconnect automatique côté agent avec backoff exponentiel. Les agents ré-authentifient avec leur identité persistée.