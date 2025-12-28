package heartbeat

import "fmt"

// GenerateSnippets returns integration code snippets for a heartbeat monitor.
func GenerateSnippets(baseURL, uuid string) map[string]string {
	pingURL := baseURL + "/ping/" + uuid
	startURL := pingURL + "/start"

	return map[string]string{
		"curl": fmt.Sprintf(`# Simple ping
curl -fsS -m 10 --retry 5 %s

# With start/finish tracking
curl -fsS -m 10 --retry 5 %s
# ... your job here ...
curl -fsS -m 10 --retry 5 %s`, pingURL, startURL, pingURL),

		"wget": fmt.Sprintf(`wget -q --spider %s`, pingURL),

		"python": fmt.Sprintf(`import urllib.request
urllib.request.urlopen('%s')`, pingURL),

		"go": fmt.Sprintf(`http.Get("%s")`, pingURL),

		"bash": fmt.Sprintf(`#!/bin/bash
set -e

# Signal start
curl -fsS -m 10 --retry 5 %s

# Your job here
/path/to/your/job.sh

# Signal success
curl -fsS -m 10 --retry 5 %s`, startURL, pingURL),

		"docker_healthcheck": fmt.Sprintf(`HEALTHCHECK CMD curl -f %s || exit 1`, pingURL),
	}
}
