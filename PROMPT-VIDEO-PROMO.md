# Prompt : vidéo promo de maintenant

---

Crée une vidéo promotionnelle de motion design pour **maintenant**, comme la bande démo d'un motion designer qui veut impressionner : dynamique, précise, sans temps mort, mais lisible par un humain.

## Ce que tu dois livrer

- Un MP4 en 16:9, 1920×1080, 60 images/s, d'environ 75 à 80 secondes.
- Une voix off en anglais et une musique de fond discrète, **sans aucun effet sonore** (pas de boom, de souffle ni de bip).
- Tous les textes à l'écran sont en anglais.
- Tu travailles dans un dossier temporaire, jamais dans le dépôt, et tu me donnes le chemin du fichier final.

## Sources à exploiter

Appuie-toi sur tout ce que tu trouves sur le produit, pas seulement sur le README :

- `README.md` : positionnement, fonctionnalités, éditions, modèle de sécurité.
- `specs/` : toutes les spécifications. Elles décrivent le détail de chaque fonctionnalité.
- `frontend/src/assets/main.css` : les couleurs du design system (fond `#07090D` / `#0B0E13`, surfaces `#12151C`, vert `#3ECF8E`, cyan `#22D3EE`, rouge `#f43f5e`, ambre `#f59e0b`).
- `docs/logos/` : le logo (un « M » en signal et un point « live ») et le logo France 2030.
- Les captures d'écran de `docs/screen-captures/` et de `../maintenant-web/site/static/img/screen-captures/` : c'est la référence visuelle de l'interface.

**Une spec n'est pas une preuve que la fonctionnalité existe.** Certaines décrivent un travail pas encore fusionné, comme la détection d'anomalies. Avant de montrer une fonctionnalité, vérifie dans le code qu'elle est livrée. Vérifie aussi l'édition qui la débloque :

- `internal/extension/capability.go` : l'édition minimale de chaque fonctionnalité et de chaque canal d'alerte.
- `internal/extension/quota.go` : le nombre d'hôtes par édition.
- `internal/extension/history_window.go` : la durée d'historique par édition.

## Exactitude du discours

- Ne promets rien que le produit ne fait pas. Exemple : les endpoints sont contrôlés à intervalle régulier et leur temps de réponse est mesuré en millisecondes. Ne dis pas qu'une panne est détectée « à la milliseconde ».
- Chaque fonctionnalité réservée à une édition payante porte un petit badge : **Personal** (cyan) ou **Pro** (violet), avec un reflet qui passe dessus. Au moment où j'écris ce prompt :
  - **Personal** : contrôle OCSP, enrichissement CVE, score de risque, posture de sécurité, alertes par e-mail et Telegram, jusqu'à 20 hôtes, 30 jours d'historique.
  - **Pro** : Slack, Teams, politiques d'escalade, hôtes illimités, 90 jours d'historique.
  - **Community** : l'assistant IA via MCP et les alertes de fin de support des OS.
- **Licence** : la Community est sous **Apache 2.0**. Personal et Pro sont des licences commerciales, mais la vidéo met en avant Apache 2.0.
- **Hyper Open X** : écris « **Selected for Hyper Open X** · France 2030 ». N'écris jamais « funded », « winner » ni « laureate ».

## Rythme et lisibilité

- Un spectateur doit pouvoir lire chaque texte deux fois. Vise 13 à 17 caractères par seconde, et garde au moins 0,5 s de lecture une fois l'animation d'entrée terminée.
- Les entrées, impacts et transitions restent rapides. Ce sont les temps de pause après les textes qui sont longs, et pendant ces pauses rien n'est figé : logs qui défilent, paquets qui circulent, voyants qui clignotent.
- Chaque écran de fonctionnalité dure environ 3,5 s. Son contenu (tableaux, graphiques, compte à rebours, texte tapé) se construit en 1,5 à 2 s.

## Direction artistique

- Thème sombre fidèle à l'interface. Police Outfit pour les titres, JetBrains Mono pour les labels, en majuscules et bien espacées.
- Les fenêtres d'interface sont recréées en HTML dans le style des captures, pas collées en image.
- Habillage de bande démo à l'écran, pendant toute la vidéo :
  - des équerres dans les coins ;
  - « maintenant ■ motion reel » en haut à gauche ;
  - « ● REC » et un compteur de temps en haut à droite ;
  - le nom de la scène en bas à gauche et une barre de progression en bas à droite.
- Grain léger, vignettage, grille de points en fond qui ondule sur les temps forts. Secousses de caméra et flashs discrets sur les impacts. Aberration chromatique sur les gros titres qui claquent.

## Déroulé

Chaque ligne indique le temps de début, ce qu'on voit, puis la voix off.

1. **0 s, le signal** : un point vert bat comme un cœur, trace une ligne d'électrocardiogramme qui se transforme en « M » du logo, puis le nom apparaît lettre par lettre avec « INFRASTRUCTURE MONITORING IN ONE CONTAINER ». La caméra plonge dans le point.
   *« Your infrastructure has a pulse. »*
2. **5 s, le bruit** : douze fenêtres d'outils s'empilent, avec des erreurs qui clignotent (Prometheus, Grafana, Alertmanager, node-exporter, cAdvisor, blackbox-exporter, ssl_exporter, Loki, Promtail, Trivy, Diun, Pushgateway). Un compteur géant affiche le nombre d'outils. Tout est aspiré au centre, puis « ONE CONTAINER. » claque, sous-titré « replaces all of them. ».
   *« Watching it usually means a dozen tools to install, wire, and keep alive. » puis « Or… one container. »*
3. **12,5 s, le conteneur** : un conteneur en 3D isométrique tombe, provoque une onde de choc sur une grille au sol, puis dix services apparaissent autour de lui, reliés par des lignes où circulent des paquets. Texte : « Drop a container. Your stack is monitored. »
   *« Drop it in. maintenant finds every service on its own. »*
4. **18 s, huit écrans de fonctionnalités** : à gauche un grand numéro en contour, la catégorie et un titre sur deux lignes ; à droite une fenêtre d'interface animée.
   - Conteneurs : liste des conteneurs avec une boucle de redémarrage, logs en direct.
     *« Every container, its health, restarts, and live logs. »*
   - Endpoints : temps de réponse, disponibilité sur 90 jours, labels Docker tapés au clavier.
     *« Every endpoint, its uptime and response time. »*
   - Certificats : chaîne de validation, compte à rebours 30, 14, 7, 3, 1 jours, badge OCSP en Personal.
     *« Certificates, long before they expire. »*
   - Cron et heartbeats : la commande `curl` tapée, les pings reçus, une échéance manquée.
     *« Cron jobs that suddenly go quiet. »*
   - Ressources : jauges CPU, mémoire, disque et réseau, alerte de seuil par conteneur, historique 7 jours, 30 jours (Personal) et 90 jours (Pro).
     *« CPU, memory, disk, per container and per host. »*
   - Mises à jour : empreintes d'images qui changent, commandes compose de mise à jour et de retour arrière, fin de support d'un OS hôte.
     *« Image updates, before you pull. »*
   - Sécurité : ports exposés, conteneurs privilégiés, score de posture et CVE en Personal.
     *« Ports that should never have been open. »*
   - Alertes : une alerte critique envoyée vers Discord, Webhook, Email et Telegram (Personal), Slack et Teams (Pro), puis une escalade (Pro).
     *« And one alert pipeline, to every channel you use. »*
5. **46 s, plusieurs hôtes** : un serveur central et huit agents (Docker, Swarm, Kubernetes, Linux) reliés par des flux. Texte : « One server. Every host. », avec les badges « Personal : up to 20 hosts » et « Pro : unlimited ».
   *« One server watches every host. »*
6. **50,5 s, l'empreinte** : quatre chiffres claquent sur le temps puis se rangent dans une grille : 1 binaire, <30 Mo de RAM, 0 dépendance, 4 environnements.
   *« One binary. Under thirty megabytes. Zero dependencies. »*
7. **56 s, tableau de bord et MCP** : le tableau de bord s'assemble en 3D. Une discussion avec l'assistant IA pose « What's burning right now? », appelle les outils MCP, puis répond.
   *« And when something breaks, just ask your AI what's burning. »*
8. **64 s, les éditions** : « Start free. Grow when you need to. » et trois cartes : Community (« free · Apache 2.0 »), Personal et Pro. Chacune liste quelques points et finit par « + and much more… ».
   *« Community is free, under Apache 2. Personal and Pro add hosts, channels, history, and much more. »*
9. **70,5 s, la fin** :
   - le logo se redessine, puis le slogan « Drop a container. Your stack is monitored. » ;
   - un gros bouton « Start free at **maintenant.dev** → » qui brille ;
   - en dessous, `docker compose up -d` et « open source · Apache 2.0 » ;
   - tout en bas, le logo France 2030 sur une pastille blanche et « Selected for Hyper Open X ».
   *« maintenant. Drop a container. Your stack is monitored. » puis « Start free at maintenant dot dev. »*

## Technique

- **Animation** : une page HTML autonome pilotée par une fonction `render(t)` qui dépend seulement du temps. Pas d'animation CSS ni de `requestAnimationFrame`, pour que chaque image soit reproductible.
- **Rendu des images** : Chromium sans interface via Playwright. Charge les polices en local, capture chaque image, répartis le travail sur plusieurs processus, puis assemble avec ffmpeg (H.264, `yuv420p`, `-tune animation`).
- **Voix off** : synthèse vocale locale avec Kokoro, voix `af_heart`, installée dans un environnement Python 3.12.
  - Impose la prononciation française de « maintenant » (`[maintenant](/mɛ̃tənɑ̃/)`).
  - Génère chaque phrase séparément, place-la au début de son écran, et décale la suivante si elles se chevauchent. Signale tout décalage de plus d'une seconde.
- **Musique** : synthétisée en Python, 120 BPM, nappes, basse et arpèges légers.
- **Mixage** :
  - voix à -16 LUFS, musique 16 dB en dessous pendant la parole ;
  - la musique baisse 0,35 s avant chaque phrase et remonte entre les phrases ;
  - volume final à -14 LUFS par un gain fixe plus un limiteur sur les pics. N'utilise pas `loudnorm` de ffmpeg : dès que les pics l'empêchent de rester linéaire, il passe en mode dynamique et abîme les premières secondes.

## Vérifications avant de me livrer

- Des planches d'images extraites de la vidéo à des moments clés : contrôle les chevauchements, les textes coupés et les éléments qui sortent du cadre.
- Une mesure du volume (volume global, pics, écart entre la voix et la musique).
- Une transcription automatique du début de la voix off (faster-whisper), pour s'assurer que la première phrase est intelligible.
- Dis-moi clairement ce que tu as vérifié et ce que tu n'as pas pu vérifier (par exemple : tu n'as pas écouté le résultat).
