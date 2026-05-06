# Audit : Feature "Alert Escalation"

**Date :** 2026-05-05  
**Branche :** `main` (a70b170)  
**Contexte :** Vérification de l'existence et du fonctionnement de la feature "Alert Escalation" annoncée comme Pro dans la matrice produit.

---

## 1. Cartographie du code

### Fichiers principaux

| Fichier | Rôle |
|---------|------|
| `internal/alert/model.go` | Définit les interfaces `Escalator` (ligne 151-153) et `EscalationAction` (ligne 156-159) |
| `internal/alert/engine.go` | Implémente la logique d'escalation : `runEscalationEvaluator` (ligne 181-195), `evaluateEscalations` (ligne 197-247), `escalateAlert` pour escalation de sévérité (ligne 362-394) |
| `internal/extension/alert.go` | `NoopEscalator` — implémentation CE par défaut qui retourne toujours `nil` (ligne 21-26) |
| `internal/store/sqlite/alerts.go` | Méthodes CRUD : `SetEscalatedAt` (ligne 244), `ListUnacknowledgedActiveAlerts` (ligne 255), `AcknowledgeAlert` (ligne 232) |
| `internal/store/sqlite/migrations/5_alert_ack_escalation.up.sql` | Migration ajoutant `acknowledged_at`, `acknowledged_by`, `escalated_at` |

### Modèle de données

**Table `alerts` :**
- `acknowledged_at` (DATETIME, nullable) : timestamp de l'acquittement
- `acknowledged_by` (TEXT, nullable) : identité de l'acquitteur
- `escalated_at` (DATETIME, nullable) : timestamp de l'escalation par policy

**Pas de table `escalation_policies` ou équivalent.**

### Architecture

Deux mécanismes d'escalation distincts :

1. **Escalation de sévérité** (`engine.go:362-394`)  
   Quand une alerte active existante reçoit un événement avec sévérité supérieure (`warning → critical`), l'alerte est mise à jour et re-notifiée. **Fonctionne actuellement.**

2. **Escalation par policy** (`engine.go:197-247`)  
   Worker qui tourne toutes les 60 secondes (`defaultEscalationInterval`), liste les alertes non-acquittées et non-escaladées, appelle `Escalator.Evaluate()` pour chacune, et si une `EscalationAction` est retournée, envoie une notification au canal secondaire et marque l'alerte comme escaladée.  
   **Worker démarré uniquement si un vrai `Escalator` est injecté (ligne 176-178).** Comme seul `NoopEscalator` existe, ce worker ne tourne jamais en pratique.

---

## 2. Vérification fonctionnelle

### Tests

- **Test unitaire :** `internal/extension/extension_test.go:34-42` — vérifie que `NoopEscalator.Evaluate()` retourne `nil`.
- **Tests d'intégration :** Aucun.
- **Tests end-to-end (alerte non-acquittée → déclenchement escalation → notification secondaire) :** Aucun.

### Pipeline fonctionnel

| Composant | État |
|-----------|------|
| `ListUnacknowledgedActiveAlerts()` | ✅ Implémenté. Filtre `status='active' AND acknowledged_at IS NULL AND escalated_at IS NULL` |
| `SetEscalatedAt()` | ✅ Implémenté. UPDATE de la colonne `escalated_at` |
| `runEscalationEvaluator` | ✅ Code présent, mais **jamais démarré** (car seul NoopEscalator existe) |
| `Escalator.Evaluate()` | ⚠️ Seulement `NoopEscalator` qui retourne toujours `nil` |
| Dispatch notification secondaire | ✅ Code présent (ligne 217-232 de engine.go), mais jamais exécuté |

**Conclusion :** Le pipeline est câblé, mais non-opérationnel faute d'implémentation concrète de l'Escalator.

---

## 3. Vérification du gating Pro

### Feature gate

**Fichier :** `internal/api/v1/router.go:597`

```go
"alert_escalation": isEnterprise,
```

✅ **Bien gated derrière Enterprise.**

### Points de vérification

| Aspect | État |
|--------|------|
| Feature flag exposé dans `/api/v1/edition` | ✅ Oui (`alert_escalation: true` si Enterprise) |
| Implémentation gated au runtime | ⚠️ Pas d'implémentation concrète à gater |
| Endpoints API gated | ❌ Aucun endpoint pour configurer les policies |
| UI gated | ❌ Aucune UI |

**Note :** Le gating est présent, mais il n'y a **rien à gater** — aucun endpoint REST ou MCP pour créer/lire/modifier/supprimer des escalation policies.

### Comparaison avec `acknowledge_alert`

Le bug récent sur `acknowledge_alert` (MCP handler non-implémenté mais exposé) ne se reproduit pas ici : il n'y a **aucun handler MCP ou REST pour l'escalation**, donc pas de risque d'exposition accidentelle.

---

## 4. Description du comportement implémenté

### Ce qui existe réellement

#### 4.1 Modèle de données

- **Colonne `escalated_at`** dans `alerts` : permet de tracker *qu'une alerte a été escaladée*, mais pas *par quelle policy*.
- **Colonnes `acknowledged_at` / `acknowledged_by`** : permettent de tracker l'acquittement (qui interrompt l'escalation).

Aucune table pour stocker :
- Les policies d'escalation (délai, canaux primaire/secondaire, niveaux multiples)
- Les règles de routing vers canaux secondaires
- L'historique des escalations

#### 4.2 Configuration

**Impossible de configurer l'escalation.**

Aucun endpoint :
- Pas de `POST /api/v1/escalation-policies`
- Pas de `GET /api/v1/escalation-policies`
- Pas de MCP tool `create_escalation_policy`
- Pas de labels Docker/Kubernetes pour définir les policies

#### 4.3 Cycle de vie

**Cycle implémenté (mais non-opérationnel) :**

1. Alerte créée → notification primaire envoyée (via routing rules standard)
2. Worker `runEscalationEvaluator` tourne toutes les 60s (si Escalator réel injecté)
3. Liste les alertes `status='active' AND acknowledged_at IS NULL AND escalated_at IS NULL`
4. Pour chaque alerte, appelle `Escalator.Evaluate(alertID, elapsed)` où `elapsed = now - fired_at`
5. Si `EscalationAction` retournée → envoie notification au `ChannelID` spécifié, marque `escalated_at = now`
6. **Acquittement interrompt l'escalation** : les alertes acquittées ne sont plus listées par `ListUnacknowledgedActiveAlerts()`

**Limitations :**
- Aucune implémentation de policy (délai seuil, canaux, niveaux multiples)
- Aucun moyen pour `Escalator.Evaluate()` de consulter une policy configurée (elle n'existe pas en BDD)
- Pas de résolution automatique ni de fin de chaîne d'escalation

#### 4.4 Escalation de sévérité (fonctionnelle)

**Distincte de l'escalation par policy.**

Si une alerte active existante (ex: `warning: CPU > 80%`) reçoit un nouvel événement avec sévérité supérieure (`critical: CPU > 95%`) :
- `engine.escalateAlert()` est appelée (ligne 362-394)
- La sévérité et le message sont mis à jour en BDD
- L'alerte est re-broadcastée en SSE
- Les notifications sont re-envoyées aux canaux matchant la nouvelle sévérité

**Fonctionne actuellement.** Ce n'est pas gated Pro.

#### 4.5 Intégration acknowledge

✅ **Lien bien établi.**

- `AcknowledgeAlert()` met à jour `acknowledged_at` et `acknowledged_by`
- `ListUnacknowledgedActiveAlerts()` filtre `acknowledged_at IS NULL`
- Une alerte acquittée **ne sera plus évaluée pour escalation**

Le fix récent sur `acknowledge_alert` (MCP handler) garantit que l'acquittement fonctionne bien en Enterprise, donc l'interruption de l'escalation est opérationnelle.

#### 4.6 Limitations connues / TODO

| Item | Statut |
|------|--------|
| Implémentation concrète de `Escalator` | ❌ Manquant (seulement `NoopEscalator`) |
| Table BDD pour policies | ❌ Manquant |
| Endpoints REST/MCP pour CRUD policies | ❌ Manquant |
| UI pour configurer les policies | ❌ Manquant |
| Tests d'intégration | ❌ Manquant |
| Documentation utilisateur | ❌ Manquant |
| Support de niveaux multiples (escalation en cascade) | ❌ Non-implémenté |
| Support de canaux multiples par niveau | ❌ Non-implémenté |
| Métriques/logs pour débug escalation | ⚠️ Logs présents (ligne 240-245), mais jamais exécutés |

**Aucun TODO/FIXME dans le périmètre du code d'escalation.**

---

## Conclusion

### Verdict

**L'escalation d'alerte est une feature stub (placeholder Pro).**

- ✅ **Architecture complète** : interfaces propres, injection de dépendances, séparation CE/Pro, hooks bien placés
- ✅ **Code de dispatching fonctionnel** : le worker et la logique d'envoi sont prêts à fonctionner
- ✅ **Schéma BDD minimal** : les colonnes pour tracker l'état existent
- ✅ **Feature gate** : bien exposé comme Enterprise
- ❌ **Implémentation concrète** : aucune policy configurable, aucun moyen pour un utilisateur Pro de définir des règles d'escalation
- ❌ **Tests** : seulement un test unitaire pour le no-op
- ❌ **UI/API** : aucun moyen de créer/lire/modifier/supprimer des policies

### État actuel

La feature **ne fonctionne pas** au sens utilisateur : un client Enterprise ne peut pas configurer d'escalation. Le code est prêt à recevoir une implémentation, mais celle-ci n'existe pas.

La seule escalation opérationnelle est **l'escalation de sévérité** (warning → critical), qui est automatique, non-configurable, et disponible en CE.

### Comparaison matrice produit vs. réalité

**Matrice produit annoncée :** *"Si une alerte n'est pas acquittée en X minutes, escalade vers un canal secondaire."*

**Réalité du code :**
- Le délai X n'est pas configurable (pas de table policies)
- Le canal secondaire n'est pas configurable (pas d'endpoints)
- L'évaluateur ne tourne jamais (car seul NoopEscalator existe)
- Le feature flag est exposé, mais la feature est vide

**Recommandation :** Soit retirer `alert_escalation` du feature gate (ou le mettre à `false`), soit implémenter la feature complète (policies BDD + Escalator réel + API + UI).

---

## Annexe : Fichiers analysés

```
internal/alert/model.go (211 lignes)
internal/alert/engine.go (806 lignes)
internal/extension/alert.go (42 lignes)
internal/extension/extension_test.go (111 lignes)
internal/store/sqlite/alerts.go (285 lignes)
internal/store/sqlite/migrations/5_alert_ack_escalation.up.sql (4 lignes)
internal/store/sqlite/migrations/5_alert_ack_escalation.down.sql (4 lignes)
internal/api/v1/router.go (691 lignes)
frontend/src/pages/AlertsPage.vue (recherché, aucune mention d'escalation)
frontend/src/stores/alerts.ts (recherché, seulement commentaires sur escalation de sévérité)
```

**Total lignes de code lié à l'escalation (hors tests/migrations) :** ~200 lignes (interfaces + worker + no-op).

**Aucun code mort identifié** : tout le code écrit est cohérent et prêt à être utilisé, il manque juste l'implémentation concrète.
