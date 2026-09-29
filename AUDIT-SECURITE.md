# Audit de sécurité & architecture — maintenant

Audit : 2026-07-29 · Dernière mise à jour : 2026-08-06 · Portée : backend Go (~60 k lignes), frontend Vue 3 (~35 k lignes), déploiement (Docker/compose/CI).
Méthode : revue manuelle ciblée (auth, injection, SSRF, crypto, secrets) + scanners (`govulncheck`, `gosec`, `npm audit`).

> **Ce fichier ne doit pas être commité.** Le dépôt est public et ce document donne le fichier, la ligne et le scénario d'exploitation des vulnérabilités encore ouvertes. Le garder hors du répertoire de travail ou dans `.gitignore`.

---

## 1. Résumé exécutif

Le code applicatif est **de bonne qualité et écrit avec soin** : handshake agent ed25519 solide, OAuth MCP correct (PKCE, comparaisons en temps constant, secrets hachés), SQL entièrement paramétré, XSS frontend maîtrisé, CI/CD durcie (SHA-pinning, SLSA, cosign, SBOM), images et manifests K8s non-root. `gosec` ne remonte que 4 faux positifs (code généré), `npm audit` est à zéro.

À l'audit, les risques réels étaient **concentrés sur deux axes** :

1. **Le plan de supervision multihost (gRPC agents)** — une faille critique de déni de service non authentifié, et une faille élevée d'usurpation d'identité d'agent. Ce sont les deux seuls défauts qui donnaient un gain concret à un attaquant. **Corrigés.**
2. **Le déploiement par défaut** — l'API n'a volontairement aucune authentification propre (déléguée au reverse proxy), mais le `compose.yml` de démarrage rapide ne met pas ce proxy en place et monte le socket Docker en direct. Le modèle est documenté et assumé ; le quickstart le contredit. **Traité par choix produit** (avertissements explicites, cf. #4).

S'y ajoutait une **dette de mise à jour de la toolchain Go** (23 vulnérabilités de la bibliothèque standard remontées par `govulncheck`), **corrigée** par le bump en 1.26.5.

**État au 2026-08-06 :** 9 findings sur 15 sont fermés (#1, #2, #3, #6, #7, #8, #9, #12) ou traités par choix produit (#4). Il ne reste plus rien de coté au-dessus de Faible, hormis #5 (dépendance moby sans correctif amont, hors de notre contrôle). Le #10 (tokens de la page de statut), initialement coté Moyenne, est ramené à Faible après vérification : la même table stocke les emails en clair par nature, donc hacher ses tokens ne protège pas ce qui a le plus de valeur. Restent surtout deux variantes du même défaut, faire confiance à un en-tête forwardé non validé (#11, #13).

| # | Sévérité | Vulnérabilité | Emplacement |
|---|----------|---------------|-------------|
| 1 | **Critique** | DoS distant non authentifié (panic `ed25519.Verify` sur l'agent sentinelle) | `internal/agentserver/auth.go:69` |
| 2 | **Élevée** | Usurpation d'`agent_id` (identité gRPC authentifiée ignorée) | `internal/agentserver/dispatcher.go:97` |
| 3 | **Élevée** | Toolchain Go non patchée — 23 vulns stdlib (TLS DoS, x509, XSS template) | `go.mod`, `Dockerfile` |
| 4 | **Élevée** | Quickstart non sécurisé : API `0.0.0.0` sans auth + socket Docker monté | `compose.yml:9-12` |
| 5 | Moyenne | Dépendance `docker/docker` v28.5.2 vulnérable (AuthZ bypass, sans correctif amont) | `go.mod` |
| 6 | ~~Moyenne~~ | MCP en *fail-open* (endpoint ouvert si credentials OAuth absents) | `internal/app/http.go:107,138` |
| 7 | ~~Moyenne~~ | Tokens d'enrollment agents stockés en clair en base | `internal/store/sqlite/agents.go:216` |
| 8 | ~~Moyenne~~ | `Access-Control-Allow-Origin: *` codé en dur sur les flux de logs | `internal/api/v1/logs_stream.go:151,244` |
| 9 | ~~Moyenne~~ | Aucun en-tête de sécurité HTTP (clickjacking, pas de HSTS/CSP) | `internal/app/http.go:34-67` |
| 10 | ~~Moyenne~~ → Faible | Tokens confirm/unsub de la page de statut stockés en clair | `internal/store/sqlite/subscribers.go` |
| 11 | Faible | Rate-limit `/status/subscribe` contournable via `X-Forwarded-For` forgé | `internal/status/handler.go:216` |
| 12 | ~~Faible~~ | `/api/` seule surface sans rate limiting | `internal/app/http.go:154` |
| 13 | Faible | `X-Forwarded-Host` non validé → URL d'installation agent falsifiable | `internal/agentserver/publicurl.go:51` |
| 14 | Faible | Consommation non atomique du code OAuth MCP (race, atténuée par PKCE) | `internal/store/sqlite/mcp_oauth.go:40` |
| 15 | Faible | Traefik `insecureSkipVerify` vers backend gRPC | `compose.prod.yml:97` |

### Statut des correctifs

**2026-07-29**

- ✅ **#1 corrigé** — garde de longueur de clé + rejet de la sentinelle dans `Verify`, intercepteurs gRPC de recovery, validation de clé à l'enrôlement. Tests de non-régression ajoutés.
- ✅ **#2 corrigé** — `Dispatch` utilise l'identité authentifiée `ag.AgentID` et rejette tout `agent_id` usurpé. Tests ajoutés.
- ✅ **#3 corrigé** — toolchain bumpée en Go 1.26.5 (`go.mod`, `Dockerfile`) ; `govulncheck` confirme désormais **0 vuln stdlib** (les 20 étaient un artefact d'un toolchain local non patché). Correction de la reco initiale : **le gate `govulncheck` existait déjà en CI** (`ci.yml`, job `audit`). Voir #5 pour les 3 advisories restantes.
- ⚠️ **#4 traité par choix produit** — le quickstart garde `"8080:8080"` (accessible immédiatement, indispensable pour un essai sur VM/serveur) avec un **avertissement de sécurité visible** au-dessus du port et sur le montage du socket. Le bind localhost cassait l'UX du quickstart (app injoignable à distance sans tunnel/proxy). Le durcissement complet (bind restreint + socket-proxy) reste documenté dans `SECURITY.md` et livré par `compose.prod.yml`.

**2026-08-06** — correctifs #6, #8, #9 et #12 (détail en §2)

- ✅ **#6 corrigé** — `Config.ValidateHTTP()` refuse de démarrer si `MAINTENANT_MCP=true` sans credentials OAuth. Opt-out explicite `MAINTENANT_MCP_ALLOW_UNAUTHENTICATED=true`, qui laisse un `WARN` à chaque démarrage. Le contrôle est dans `Start()` et pas dans `New()` : `--mcp-stdio` partage `New()` et n'écoute jamais sur le réseau.
- ✅ **#8 corrigé** — les deux `ACAO: *` supprimés ; le middleware `cors()` applique de nouveau la politique configurée (aucun en-tête par défaut, donc same-origin).
- ✅ **#9 corrigé** — middleware `SecurityHeaders` sur toutes les réponses : `X-Content-Type-Options`, `Referrer-Policy`, `X-Frame-Options: DENY` et une CSP. `script-src` contient le sha256 du script inline d'`index.html`, calculé au démarrage depuis l'asset embarqué, jamais `'unsafe-inline'`. La page de statut publique garde `frame-ancestors *` (elle est documentée comme intégrable). HSTS reste au proxy.
- ✅ **#12 corrigé** — limiteur dédié sur `/api/` à 50 req/s, burst 200. Plafond anti-flood, jamais atteint par le dashboard (le bucket public à 10/20 l'aurait rejeté).
- ✅ **#7 corrigé** — `enrollment_tokens.token` remplacé par `token_hash` (sha256) + `token_prefix` (affichage). Reprise des bases existantes par rebuild one-time, tokens en cours préservés, effacement sécurisé des pages libérées. Détail en §2.
- ⏳ #5, #10, #11, #13, #14, #15 non traités.

**Régression revue au passage.** La correction du #9 telle que décrite dans l'audit initial (« CSP de base sur les réponses HTML ») aurait cassé deux choses si elle avait été appliquée littéralement : le script inline d'`index.html` (thème et densité, appliqués avant le bundle) que `script-src 'self'` bloque, et l'intégration en iframe de la page de statut que `frame-ancestors 'none'` interdit. Les deux sont traités ci-dessus. Vérifié dans un vrai navigateur : dashboard et page de statut chargés, zéro violation CSP en console.

---

## 2. Findings détaillés

### 🔴 1 — Critique — DoS distant non authentifié via panic `ed25519.Verify`

**Fichiers :** `internal/agentserver/auth.go:69`, `internal/agentserver/enrollment.go:146-155`, `internal/store/sqlite/uuid_schema.sql:33-34`

Le runtime local est enregistré en base comme un agent « sentinelle » (`uid.LocalAgent` = `00000000-0000-0000-0000-000000000000`) avec **`public_key = NULL`** et **`status = 'active'`**. Or le handshake gRPC charge l'agent à partir de l'`agent_id` **fourni par le client** :

```go
ag, err := impl.deps.AgentStore.Get(stream.Context(), resp.GetAgentId())   // enrollment.go:146
...
if err := Verify(resp, challenge.Nonce, ag, time.Now()); err != nil {      // enrollment.go:155
```

`Verify` ne vérifie ni que l'agent n'est pas la sentinelle, ni que la clé fait la bonne taille :

```go
if !ed25519.Verify(ed25519.PublicKey(ag.PublicKey), payload, req.GetSignature()) {   // auth.go:69
```

`ed25519.Verify` **panique** quand la clé publique ne fait pas exactement 32 octets (ici NULL → 0 octet). Le serveur gRPC est créé sans intercepteur de recovery (`grpc.NewServer(opts...)`, `server.go:75`, aucun `recover()` dans le paquet), donc la panic **fait crasher tout le binaire**.

**Scénario d'exploitation :** un attaquant ayant seulement un accès réseau au port gRPC des agents (TLS côté serveur uniquement, pas de mTLS) envoie un `AuthResponse` avec `agent_id = 00000000-…`. Le clock-skew passe (l'attaquant contrôle le timestamp), et `ed25519.Verify` panique. **Une seule requête, sans aucune authentification, arrête le service ; l'attaquant reboucle → crash-loop permanent.** Applicable à tout déploiement `server`/Pro qui expose le port agents (c'est-à-dire tout déploiement multihost, par conception).

**Correctif (défense en profondeur, 3 couches) :**
1. Dans `Verify`, rejeter avant l'appel : `if len(ag.PublicKey) != ed25519.PublicKeySize { return ErrBadSignature }`.
2. Exclure explicitement la sentinelle `uid.LocalAgent` du chemin d'authentification distant (elle ne doit jamais s'authentifier via gRPC).
3. Installer un intercepteur de recovery gRPC (`grpc.ChainStreamInterceptor` + `grpc.ChainUnaryInterceptor`) qui `recover()` toute panic de handler et la convertit en `codes.Internal`, pour qu'aucune panic future ne puisse tuer le process.

---

### 🟠 2 — Élevée — Usurpation d'`agent_id` (l'identité authentifiée est jetée)

**Fichiers :** `internal/agentserver/dispatcher.go:97`, `internal/agentserver/enrollment.go:322`

Le handshake établit une identité forte (`ag.AgentID`, prouvée par signature ed25519). Mais elle n'est **jamais transmise au dispatcher** : celui-ci relit l'`agent_id` depuis le champ de l'événement, une donnée entièrement contrôlée par le client.

```go
dispatcher.Dispatch(stream.Context(), evt)     // enrollment.go:322 — ag.AgentID non transmis
...
func (d *Dispatcher) Dispatch(ctx, evt) {
    agentID := evt.GetAgentId()                // dispatcher.go:97 — valeur du fil, non authentifiée
```

L'incohérence est visible dans le même fichier : le rate-limit et `IncrEvents` utilisent bien `ag.AgentID` authentifié, seul le chemin des **données** utilise la valeur du client.

**Scénario d'exploitation :** un attaquant qui contrôle un seul hôte surveillé (ou un seul token d'enrollment) authentifie son agent normalement, puis émet des événements portant l'`agent_id` **d'une autre machine** :
- **Effacement d'inventaire à distance** — `HandleAgentInventory` archive tous les conteneurs absents du snapshot fourni ; envoyer un inventaire bidon au nom de l'agent B efface l'inventaire réel de B (y compris l'hôte local via la sentinelle).
- **Détournement d'attribution** d'un conteneur existant (`agent_event.go:66-68`).
- **Étouffement d'alertes** — forger de faux « tout va bien » (heartbeats, sondes, ressources) pour les monitors d'un autre hôte, ou fabriquer de fausses pannes.

C'est une escalade latérale dans le plan de supervision : compromettre un hôte de faible confiance permet d'aveugler toute la flotte.

**Correctif :** passer l'identité authentifiée en paramètre — `Dispatch(ctx, ag.AgentID, evt)` — et supprimer `evt.GetAgentId()`. En complément, rejeter le flux (`codes.PermissionDenied`) si `evt.GetAgentId()` est non vide et diffère de `ag.AgentID`, et marquer le champ comme informatif dans `proto/ingest.proto:213`.

---

### 🟠 3 — ~~Élevée~~ → Moyenne (revue à la baisse après vérification) — Toolchain Go non patchée

**Fichiers :** `go.mod`, `Dockerfile` — ✅ **corrigé** (bump Go 1.26.5).

`govulncheck` remontait initialement 23 vulnérabilités atteignables, presque toutes dans la stdlib (DoS TLS 1.3 `GO-2026-4870`, contournement x509 `GO-2026-4866`, XSS `html/template`, parsing IPv6 `net/url`).

**Correction de l'analyse initiale, après vérification :**
- Le scan avait tourné avec un Go **1.26.0 local non patché**, alors que la release compilait avec `golang:1.25-alpine` (dernier 1.25.x, patché). Les 20 vulns stdlib étaient donc un **artefact du toolchain local**, pas une faille du binaire livré — la sévérité réelle est Moyenne (dette de version), pas Élevée.
- Le gate `govulncheck` **existait déjà en CI** (`.github/workflows/ci.yml`, job `audit`) : call-graph aware, bloquant sur les advisories atteignables ET corrigeables, report-only sinon. La reco « ajouter le gate » était erronée.

**Correctif appliqué :** bump `go.mod` → `go 1.26.5` et `Dockerfile` → `golang:1.26-alpine`. Après bump, `govulncheck` confirme **0 vuln stdlib** ; ne restent que les 3 advisories `docker/moby` sans correctif amont (voir #5), report-only dans le gate.

---

### 🟠 4 — Élevée — Le démarrage rapide expose tout, sans proxy ni protection du socket

**Fichiers :** `compose.yml:9-12`, `compose.local.yml:21`

Le modèle de sécurité **délègue l'authentification au reverse proxy** — c'est explicite et bien documenté (`SECURITY.md:74-85`, `docs/security.md`). Mais le `compose.yml` versionné (celui que suit un nouvel utilisateur) :
- publie `8080:8080` sur **toutes les interfaces**, sans le proxy d'auth → quiconque atteint le port lit conteneurs, logs et posture, et peut créer/modifier webhooks, alertes et **tokens d'enrollment agents** (l'API masque les tokens existants via `maskToken`, mais rien n'empêche d'en créer un neuf et d'en lire la réponse) ;
- monte **`/var/run/docker.sock` en direct** (`:ro` ne bloque pas les écritures de l'API Docker) → une RCE dans l'app = root sur l'hôte.

`compose.prod.yml` fait pourtant les bonnes choses (docker-socket-proxy en lecture seule, réseau interne, basicauth Traefik). C'est le **quickstart** qui montre le mauvais pattern, en contradiction avec `SECURITY.md` qui dit l'inverse.

**Correctif :** dans `compose.yml`, binder `127.0.0.1:8080:8080`, ajouter un commentaire d'avertissement, et généraliser le pattern socket-proxy. Idéalement fournir un quickstart « sécurisé par défaut ».

---

### 🟡 Findings moyens

**5 — Dépendance `docker/docker` v28.5.2 vulnérable.** `govulncheck` remonte GO-2026-4887 (bypass de plugin AuthZ sur gros corps de requête), GO-2026-4883 et GO-2026-5668 — **sans correctif amont disponible** à ce jour. Impact réel surtout lié au montage du socket (finding 4). *Correctif :* suivre les releases moby et bumper dès qu'un fix sort ; entre-temps, ne jamais exposer le socket en direct.

**6 — MCP en fail-open.** ✅ **corrigé (2026-08-06).** `internal/app/http.go:107` : si `MAINTENANT_MCP=true` mais que `MCP_CLIENT_ID`/`MCP_CLIENT_SECRET` sont vides, `/mcp` était monté **sans aucune auth** (simple log `Info` « MCP server enabled without auth »). Comme `docs/security.md:43` demande explicitement que `/mcp` contourne l'auth du proxy, un `.env` incomplet exposait logs/conteneurs/alertes en lecture publique.

*Correctif appliqué :* `Config.ValidateHTTP()` (`internal/app/config.go`) renvoie `ErrMCPUnauthenticated`, appelé en tête de `Start()`. Le binaire s'arrête avec un message qui nomme les deux variables manquantes. Opt-out `MAINTENANT_MCP_ALLOW_UNAUTHENTICATED=true` pour l'usage local, qui déclenche un `WARN` à chaque démarrage au lieu de l'ancien `Info`.

Deux détails de placement qui comptent : le contrôle est dans `Start()` et pas dans `New()`, parce que `--mcp-stdio` passe par `New()` et n'écoute jamais sur le réseau (le refuser là aurait cassé un client stdio local sans rien protéger) ; et le `WARN` n'est émis que si l'opt-out est réellement posé, sinon un démarrage refusé loguait « enabled WITHOUT auth » juste avant de refuser, ce qui se lit à l'envers. Tests : `TestConfigValidateHTTP_MCP`.

**7 — Tokens d'enrollment stockés en clair.** `enrollment_tokens.token` est persisté tel quel et recherché en clair (`agents.go:216,251,283,305`), alors que le module OAuth MCP du même dépôt stocke `sha256(token)` (`internal/mcp/oauth/crypto.go:22`, `HashToken`) pour les codes, les refresh tokens et jusqu'au secret client.

**Portée réelle, vérifiée le 2026-08-06 :** le risque est **au repos uniquement**. `HandleListEnrollmentTokens` et `HandleGetEnrollmentToken` passent tous deux par `maskToken()` — l'API ne divulgue jamais un token existant. Le clair ne sort qu'une fois, dans la réponse de création, ce qui est inévitable. Ce qui expose, c'est toute lecture du fichier SQLite : sauvegarde, volume `maintenant-data`, copie du `.db`. Chaque token ni consommé ni expiré y est directement rejouable pour enrôler un agent.

Le hash est **déjà calculé** : `agents_handler.go:105-106` fait `sha256(tokenStr)` et le tronque à 16 caractères hex pour fabriquer l'id (le schéma le documente : `id -- hex(sha256(token))[:16]`). Il est donc stocké *à côté* du clair, tronqué à 64 bits, au lieu de le remplacer.

✅ **Corrigé le 2026-08-06.**

*Correctif appliqué :* la colonne `token` devient `token_hash` (sha256 hex) accompagnée de `token_prefix`. `agent.NewToken()` (`internal/agent/token.go`) mint le token et renvoie d'un coup le clair, le hash, l'id et le préfixe ; le clair n'est jamais passé au store. Les trois lookups (`EnrollAtomic`, `classifyTokenFailure`, `GetByToken`) hachent la valeur reçue avant de chercher.

*Arbitrage d'affichage, tranché :* `token_prefix` stocke les 14 premiers caractères, exactement ce que rendait `maskToken` — l'UI est inchangée, la liste distingue toujours deux tokens. Ce sont `mnt_enr_` plus six caractères base32, soit 30 bits sur 256 : sans valeur pour qui ne détient que le hash, ce qui est précisément le modèle de menace.

*Reprise des bases existantes :* `rebuildEnrollmentTokensForHashing`, one-time, sur le modèle de `transform_cert_sni.go` et `transform_endpoint_degraded.go` — gardé par la présence de `token_hash` dans le DDL, donc une installation fraîche le saute (le schéma sort déjà correct d'`uuid_schema.sql`). Le hash est calculé depuis le clair stocké via une fonction SQL enregistrée sur la connexion, donc **les tokens déjà distribués restent valables** : personne n'a à en réémettre.

*Pages libérées :* `DROP TABLE` marque les pages libres sans écraser leur contenu, donc le clair pourrait rester lisible dans l'espace mort du fichier. Le rebuild pose `PRAGMA secure_delete=ON` avant et `PRAGMA incremental_vacuum` après, ce qui est le mécanisme documenté par SQLite pour que le contenu libéré soit remis à zéro.

**Ce qui a été mesuré, et ce que ça ne prouve pas.** Sur un vrai fichier, après arrêt propre, `strings` et un `grep` binaire remontent 2 occurrences du token avant migration et 0 après, sans `-wal` ni `-shm` résiduel. Cela établit que cette suite d'octets n'est plus dans ce fichier. Cela n'établit pas que le secret est irrécupérable :

- Les copies faites **avant** la migration gardent le clair (sauvegardes, snapshots de volume, `.db` copiés pour déboguer). La migration ne les atteint pas.
- Sur un FS copy-on-write (btrfs, ZFS) ou un SSD avec wear-levelling, les anciens blocs peuvent survivre sous le niveau que SQLite contrôle.
- Le test couvre l'arrêt propre. Après un crash, un `-wal` persistant peut contenir d'anciennes images de page ; ce cas n'a pas été testé.
- **`secure_delete` n'est pas démontré :** le run sans ce pragma donnait déjà 0 occurrence. Il est posé sur la foi du mécanisme documenté, pas d'une mesure différentielle.

La seule chose qui ferme réellement le sujet pour un token déjà distribué est de le révoquer. Atténuation réelle : ces tokens sont à usage unique et plafonnés à 7 jours de TTL, donc le stock exposé s'épuise seul.

*Tests :* `enrollment_token_hash_test.go` (rebuild, idempotence, absence de clair, schéma frais, aller-retour store) et `agents_token_handler_test.go` (le clair sort une fois à la création et dans les templates d'installation, jamais sur les routes de lecture).

**8 — `ACAO: *` codé en dur sur les logs.** ✅ **corrigé (2026-08-06).** `logs_stream.go:151` et `:244` forçaient le joker CORS sur la route la plus sensible (contenu des logs), court-circuitant la politique CORS fermée par défaut. Sur un déploiement sans auth, n'importe quel site visité par un opérateur pouvait `fetch()` et lire les logs.

*Correctif appliqué :* les deux lignes supprimées, chemin local et chemin agent distant. Le middleware `cors()` (`router.go:709`) reprend la main : sans `MAINTENANT_CORS_ORIGINS`, aucun en-tête CORS n'est émis, donc same-origin strict. Vérifié : une requête avec `Origin: https://evil.example` sur `/api/v1/containers` ne reçoit aucun `Access-Control-Allow-Origin`.

Cotation revue : l'audit initial classait ce point en Moyenne. Il méritait mieux, parce que le déploiement où il mord est exactement le quickstart assumé au #4 (pas d'auth, donc pas de credentials à envoyer, donc le joker suffit à lire). C'est aussi le meilleur rapport gain/effort de la liste : deux lignes.

**9 — Aucun en-tête de sécurité HTTP.** ✅ **corrigé (2026-08-06).** Ni `X-Frame-Options`, ni CSP sur `index.html`, ni HSTS (`http.go:34-67`). Clickjacking du dashboard possible ; aucune défense en profondeur si un XSS apparaissait.

*Correctif appliqué :* middleware `SecurityHeaders` (`internal/app/http.go`), posé **à l'extérieur** du `TimeoutHandler` pour que les en-têtes partent aussi sur son 503, et avec `Set` pour qu'une route ayant sa propre politique plus stricte (`status_personalization.go:209`) puisse encore écraser. Il pose `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`, `X-Frame-Options: DENY` et une CSP `default-src 'self'` avec `object-src 'none'`, `base-uri 'self'`, `form-action 'self'`.

Deux pièges que la reco initiale (« CSP de base sur les réponses HTML ») aurait fait sauter :

1. **`index.html` contient un script inline** (thème et densité, appliqués avant le bundle pour éviter un flash). Un `script-src 'self'` sec le bloque et l'app s'affiche sans thème. La CSP est donc construite au démarrage à partir de l'asset embarqué : `inlineScriptHashes` extrait chaque script inline et met son sha256 dans `script-src`. Hasher ce qui est réellement embarqué plutôt que figer un digest garde la politique juste à chaque rebuild du front. `'unsafe-inline'` n'est jamais posé sur `script-src` (il l'est sur `style-src`, Vue et uPlot posant des styles à l'exécution).
2. **La page de statut publique est documentée comme intégrable** (`docs/security.md`, section CORS). Un `frame-ancestors 'none'` global l'aurait cassée sans bruit. `/status` et `/status/*` reçoivent donc `frame-ancestors *` et pas de `X-Frame-Options` ; tout le reste, dashboard et `/api/v1/*`, est en `'none'` + `DENY`. `/api/v1/status/` est de l'admin malgré son nom et reste protégé.

HSTS n'est volontairement pas posé par l'app : TLS termine au proxy, et déduire « c'était du HTTPS » d'un en-tête forwarded revient à faire confiance à quelque chose que le client forge (c'est la classe de bug du #13). Documenté et ajouté à la checklist de `docs/security.md`. Tests : `internal/app/http_test.go`, dont le digest d'un script connu calculé indépendamment via `openssl`.

**11 — Rate-limit contournable via XFF.** `status/handler.go:216` fait confiance à `X-Forwarded-For` sans condition ; exposé en direct, un attaquant forge un XFF par requête et contourne la limite 5/h → spam d'emails de confirmation + croissance non bornée de la map. *Correctif :* n'honorer XFF que derrière un proxy de confiance déclaré.

### 🟢 Findings faibles

- **12** — ✅ **corrigé (2026-08-06).** `/api/` était la seule surface montée sans rate limiting (`http.go:154`). Un limiteur dédié `a.apiRL` la couvre désormais, à **50 req/s, burst 200**, séparé du bucket public (10/20). Le bucket public aurait 429 l'usage normal : un chargement de dashboard part en dizaines d'appels parallèles. C'est un plafond anti-flood, pas un quota, et il ne doit jamais être atteint par l'interface. Mesuré : 260 requêtes concurrentes → 207 passent, 53 en 429.
- **13** — `X-Forwarded-Host` non validé (`publicurl.go:51`) : l'URL d'installation agent (qui contient le token en clair) est falsifiable → fuite de token par ingénierie sociale. Ne jamais dériver `grpc://` (non chiffré) d'un en-tête.
- **14** — Consommation non atomique du code d'autorisation OAuth (`mcp_oauth.go:40-78`) : race permettant deux échanges du même code, atténuée par PKCE.
- **15** — `insecureSkipVerify=true` Traefik → backend gRPC (`compose.prod.yml:97`) : MITM théorique limité au réseau Docker interne. *Correctif :* pinner le cert via `serversTransport.rootCAs`.

---

## 3. Points vérifiés sains

- **Injection SQL** : aucune. Toutes les requêtes sont paramétrées ; les rares `fmt.Sprintf`/concaténations ne construisent que des placeholders `IN (?, …)` et des noms de table issus d'une liste interne fixe.
- **SSRF** : garde `ssrf.ValidateURL`/`ssrf.NewHTTPClient` correctement appliquée sur webhooks et canaux d'alerte (URLs fournies par l'utilisateur). Les sondes endpoint/cert dialent des cibles arbitraires, mais **c'est la fonction même de l'outil** (SSRF « by design », pas une faille) — protégé par l'auth du proxy.
- **Handshake agent** : challenge/réponse ed25519, nonce serveur 32 octets par flux (anti-rejeu), payload signé liant nonce+agent_id+timestamp, contrôle de dérive d'horloge 300 s, révocation vérifiée. Réenrôlement anti-takeover (INSERT non-upsert, transaction atomique).
- **OAuth MCP** : PKCE S256, secret client et PKCE comparés en temps constant, codes/tokens stockés hachés, usage unique + expiration.
- **Licence** : signature ed25519 vérifiée au cache disque et à la réponse serveur, clé publique injectée au build, cache en `0600`.
- **XSS frontend** : les 4 seuls `v-html` affichent du markdown sanitizé côté serveur (goldmark + bluemonday, schémas http/https uniquement). Aucun token d'auth en `localStorage` (modèle sans token côté client). Pas de secret dans le bundle, pas de sourcemap prod.
- **CI/CD & supply chain** : workflows `permissions: {}`, actions pinnées par SHA, `persist-credentials: false`, pas de `pull_request_target`, Trivy + poutine en gate, provenance SLSA + signature cosign + SBOM CycloneDX attestés.
- **Conteneur** : Dockerfile drop vers uid 65534 via `setpriv`, `read_only` + `no-new-privileges` + tmpfs `noexec`, manifests K8s `runAsNonRoot`/`readOnlyRootFilesystem`.
- **Scanners** : `gosec` = 4 faux positifs (unsafe dans `ingest.pb.go` généré) ; `npm audit` = 0 vulnérabilité.

---

## 4. Analyse d'architecture

**Vue d'ensemble.** Binaire Go unique + SPA Vue embarquée (`embed.FS`), 3 modes (`embedded`, `server`, `agent`). `main.go` est volontairement mince (164 l.) ; la composition se fait dans `internal/app/` (DI manuelle explicite, ports définis côté domaine). Idée forte du multihost : **le runtime local est un agent comme les autres** (sentinelle `uid.LocalAgent`), donc toutes les vues lisent les mêmes tables quelle que soit l'origine des données.

**Forces :**
- Composition root explicite et lisible, aucun SQL dans les handlers, ports en interfaces côté domaine.
- **Écrivain SQLite unique très soigné** (`store/sqlite/writer.go`) : goroutine d'écriture unique, pool de lecture, PRAGMAs pensés (WAL borné, `auto_vacuum`, `busy_timeout`), portabilité Postgres anticipée.
- Modèle de données cohérent (UUIDv7 événements, UUIDv5 déterministes pour les clés naturelles), rétention étagée raw → hourly → daily.
- Protocole agent gRPC robuste (stream bidi, causes d'annulation typées, single-writer sur le stream, backoff+jitter, plafond de commandes en vol).
- Arrêt et dégradation gérés (`signal.NotifyContext`, `shuttingDown atomic.Bool`, superviseur runtime Docker).

**Faiblesses structurelles :**
1. **Système d'événements faiblement typé** (la plus grosse fragilité) : les événements inter-services passent par des callbacks `(string, any)` avec des `map[string]any` et des casts à l'exécution ; `wiring.go` (718 l.) est ~400 lignes d'accès non typés. Une clé renommée = bug silencieux, zéro aide du compilateur. Le paquet `internal/event` existe mais ne contient que des constantes.
2. **Croissance non bornée de la composition root** : la struct `App` (~80 champs), `HandlerDeps` (~80 champs) et `mcp.Services` — toute feature touche les trois. C'est le vrai « god object ».
3. **Couches incohérentes** : container/endpoint/heartbeat/certificate/resource ont un vrai Service ; alerts/agents/swarm/kubernetes sont exposés en `*sqlite.XxxStore` concrets dans les handlers → couplage `api/v1` ↔ `store/sqlite`, logique métier dans des handlers de 500-700 lignes. Frein principal à une future migration Postgres.
4. **Ordre d'arrêt non déterministe** : `context.Background()` capturé en ~7 endroits du wiring ; le writer peut s'arrêter avant des producteurs → dernières écritures perdues (« writer is stopped »).
5. **Contention potentielle sur l'écrivain unique** : samples ressources, checks, ingest K8s/Swarm **et** écritures HTTP interactives partagent le même canal (256) sans priorité ni métrique de profondeur — l'API peut ralentir silencieusement derrière l'ingestion sur une grosse flotte.
6. **Pas de spool côté agent** : serveur injoignable = trous d'historique ; événement rate-limité = jeté. Les `seq`/acks du protocole ne sont pas exploités pour rejouer.
7. **Couverture de tests inégale** : swarm 1 test/13 sources, heartbeat (deadline checker, chemin d'alerte critique) 1/5, `ratelimit`/`retry`/`event` = 0.

**Recommandations priorisées :**
1. Typer les événements (structs par événement dans `internal/event`, petit bus typé) — supprime la première source de bugs silencieux.
2. Uniformiser la couche service pour alerts/agents/swarm/kubernetes (interfaces côté domaine) — prérequis Postgres.
3. Découper `app.New` en modules de wiring par domaine.
4. Ordre d'arrêt explicite (errgroup, writer arrêté en dernier après drain).
5. Observabilité de l'écrivain (profondeur de file exposée) — l'outil se monitore lui-même.

---

## 5. Plan d'action recommandé

**Fait (2026-07-29) :**
1. ~~**Finding 1** — garde de longueur de clé + exclusion sentinelle + intercepteur recovery gRPC.~~
2. ~~**Finding 2** — passer `ag.AgentID` authentifié au dispatcher.~~
3. ~~**Finding 3** — bumper la toolchain Go.~~ *(la reco disait aussi « ajouter `govulncheck` en gate CI » : erronée, le gate existait déjà — voir §2)*
4. ~~Sécuriser le `compose.yml` quickstart~~ — traité par choix produit (avertissements), finding 4.

**Fait (2026-08-06) :**
5. ~~MCP fail-closed — finding 6.~~
6. ~~Supprimer les deux `ACAO: *` des logs — finding 8.~~
7. ~~Middleware d'en-têtes de sécurité — finding 9.~~
8. ~~Rate limiting sur `/api/` — finding 12.~~

9. ~~Hacher les tokens d'enrollment — finding 7.~~

**Reste à faire, court terme :**
10. N'honorer `X-Forwarded-For` que derrière un proxy de confiance déclaré — finding 11, et même classe de problème que le 13.
11. Suivre les releases moby pour le finding 5 ; pinner le cert Traefik du finding 15.
12. Hacher confirm/unsub de la page de statut — finding 10, cohérence surtout. La même table stocke les **emails** en clair par nature : une fuite de base expose la liste d'abonnés de toute façon, donc hacher les tokens ne protège pas ce qui a le plus de valeur ici.

**Fond (dette structurelle) :**
13. Typage des événements + uniformisation de la couche service (voir §4).
14. Renforcer les tests sur swarm, heartbeat deadline checker, et le mapping événement→alerte.
